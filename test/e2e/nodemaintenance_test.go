package e2e

import (
	goctx "context"
	"fmt"
	"reflect"
	"testing"
	"time"

	apis "kubevirt.io/node-maintenance-operator/pkg/apis"
	operator "kubevirt.io/node-maintenance-operator/pkg/apis/kubevirt/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	retryInterval        = time.Second * 5
	timeout              = time.Second * 120
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 5
	testDeployment       = "testdeployment"
	podLabel             = map[string]string{"test": "drain"}
)

func TestNodeMainenance(t *testing.T) {
	nodeMainenanceList := &operator.NodeMaintenanceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "kubevirt.io/v1alpha1",
		},
	}
	err := framework.AddToFrameworkScheme(apis.AddToScheme, nodeMainenanceList)
	if err != nil {
		t.Fatalf("failed to add custom resource scheme to framework: %v", err)
	}
	// run subtests
	t.Run("NodeMaintenance-group", func(t *testing.T) {
		t.Run("Cluster", ClusterTest)
	})
}

func ClusterTest(t *testing.T) {
	//t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("Initialized cluster resources")
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}
	// get global framework variables
	f := framework.Global
	// wait for node- maintanence-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "node-maintenance-operator", 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	if err = nodeMaintenanceTest(t, f, ctx); err != nil {
		t.Fatal(err)
	}
}

func nodeMaintenanceTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	err = createSimpleDeployment(t, f, ctx, namespace)
	if err != nil {
		t.Fatal(err)
	}

	nodeName, err := getCurrentDeploymentHostName(t, f)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Putting node %s into maintanance", nodeName)

	// Create node maintenance custom resource
	nodeMaintenance := &operator.NodeMaintenance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "kubevirt.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "nodemaintenance-xyz",
		},
		Spec: operator.NodeMaintenanceSpec{
			NodeName: nodeName,
			Reason:   "Set maintenance on node for e2e testing",
		},
	}

	// Create node maintenance CR
	err = f.Client.Create(goctx.TODO(), nodeMaintenance, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatal(err)
	}

	// Get Running phase first
	if err := wait.PollImmediate(1*time.Second, 10*time.Second, func() (bool, error) {
		nm := &operator.NodeMaintenance{}
		err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
		if err != nil {
			t.Logf("Failed to get %q nodeMaintenance: %v", nm.Name, err)
			return false, nil
		}
		if nm.Status.Phase != operator.MaintenanceRunning {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatal(fmt.Errorf("Failed to verify running phase: %v", err))
	}

	// Wait for the maintanance operation to complete successfuly
	if err := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		nm := &operator.NodeMaintenance{}
		err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
		if err != nil {
			t.Logf("Failed to get %q nodeMaintenance: %v", nm.Name, err)
			return false, nil
		}
		if nm.Status.Phase != operator.MaintenanceSucceeded {
			return false, nil
		}
		return true, nil
	}); err != nil {
		checkFailureStatus(t, f)
		t.Fatal(fmt.Errorf("Failed to successfuly complete maintanance operation after defined test timeout (120s)"))
	}

	node := &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		t.Fatal(err)
	}

	if node.Spec.Unschedulable == false {
		checkFailureStatus(t, f)
		t.Fatal(fmt.Errorf("Node %s should have been unschedulable ", nodeName))
	}

	if !kubevirtTaintExist(node) {
		checkFailureStatus(t, f)
		t.Fatal(fmt.Errorf("Node %s should have been tainted with kubevirt.io/drain:NoSchedule", nodeName))
	}

	nodesList := &corev1.NodeList{}
	err = f.Client.List(goctx.TODO(), &client.ListOptions{}, nodesList)
	if err != nil {
		t.Fatal(err)
	}

	computeNodesNumber := 0

	for _, node := range nodesList.Items {
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; !exists {
			computeNodesNumber++
		}
	}

	if computeNodesNumber > 2 {
		// Check that the deployment has 1 replica running after maintenance
		err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, 1, retryInterval, timeout)
		if err != nil {
			t.Fatal(err)
		}

		newNodeName, err := getCurrentDeploymentHostName(t, f)
		if err != nil {
			t.Fatal(err)
		}

		if newNodeName == nodeName {
			t.Fatal(fmt.Errorf("Deployment was done on node %s that should be under maintanence", nodeName))
		}
	}

	t.Logf("Setting node %s out of maintanance", nodeName)

	nodeMaintenanceDelete := &operator.NodeMaintenance{}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: nodeMaintenance.Namespace, Name: nodeMaintenance.Name}, nodeMaintenanceDelete)
	if err != nil {
		t.Fatal(err)
	}

	// Delete the node maintenance custom resource
	err = f.Client.Delete(goctx.TODO(), nodeMaintenanceDelete)
	if err != nil {
		t.Fatalf("Could not delete node maintenance CR : %v", err)
	}

	time.Sleep(60 * time.Second)

	node = &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		t.Fatal(err)
	}

	if node.Spec.Unschedulable == true {
		t.Fatal(fmt.Errorf("Node %s should have been schedulable", nodeName))
	}

	if kubevirtTaintExist(node) {
		t.Fatal(fmt.Errorf("Node %s kubevirt.io/drain:NoSchedule taint should have been removed", nodeName))
	}

	// Check that the deployment has 1 replica running after maintenance is removed.
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	return nil
}

func createSimpleDeployment(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, namespace string) error {
	replicas := rune(1)
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDeployment,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabel,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Labels:    podLabel,
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "node-role.kubernetes.io/master",
												Operator: corev1.NodeSelectorOpDoesNotExist,
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{{
						Image:   "busybox",
						Name:    "testpodbusybox",
						Command: []string{"/bin/sh"},
						Args:    []string{"-c", "while true; do echo hello; sleep 10;done"},
					}},
				},
			},
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err := f.Client.Create(goctx.TODO(), dep, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		return err
	}
	// wait for testPodDeployment to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, 1, retryInterval, timeout)
	if err != nil {
		return err
	}
	return nil
}

func getCurrentDeploymentPods(t *testing.T, f *framework.Framework) (*corev1.PodList, error) {
	labelSelector := labels.SelectorFromSet(podLabel)
	pods := &corev1.PodList{}
	err := f.Client.List(goctx.TODO(), &client.ListOptions{LabelSelector: labelSelector}, pods)
	if err != nil {
		return pods, err
	}

	if pods.Size() == 0 {
		return pods, fmt.Errorf("There are no pods deployed in cluster to perform the test")
	}

	return pods, nil
}

func getCurrentDeploymentHostName(t *testing.T, f *framework.Framework) (string, error) {
	pods, err := getCurrentDeploymentPods(t, f)
	if err != nil {
		return "", nil
	}
	nodeName := pods.Items[0].Spec.NodeName
	return nodeName, nil
}

func kubevirtTaintExist(node *corev1.Node) bool {
	kubevirtDrainTaint := corev1.Taint{
		Key:    "kubevirt.io/drain",
		Effect: corev1.TaintEffectNoSchedule,
	}
	taints := node.Spec.Taints
	for _, taint := range taints {
		if reflect.DeepEqual(taint, kubevirtDrainTaint) {
			return true
		}
	}
	return false
}

func checkFailureStatus(t *testing.T, f *framework.Framework) {
	nm := &operator.NodeMaintenance{}
	err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
	if err != nil {
		t.Logf("Failed to get %s nodeMaintenance: %v", nm.Name, err)
	}
	if nm.Status.Phase != operator.MaintenanceFailed {
		t.Logf("Status.Phase on %s nodeMaintenance should have been %s", nm.Name, operator.MaintenanceFailed)
	}
	if len(nm.Status.LastError) == 0 {
		t.Logf("Status.LastError on %s nodeMaintenance should have a value", nm.Name)
	}
	if len(nm.Status.PendingPods) > 0 {
		pods, err := getCurrentDeploymentPods(t, f)
		if err != nil {
			t.Logf("Failed to get deployment pods")
		}
		if pods.Items[0].Name != nm.Status.PendingPods[0].Name {
			t.Logf("Status.PendingPods on %s nodeMaintenance does not contain pod %s", nm.Name, pods.Items[0].Name)
		}
	}
}
