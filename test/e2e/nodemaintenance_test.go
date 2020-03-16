package e2e

import (
	goctx "context"
	"fmt"
	"k8s.io/apimachinery/pkg/selection"
	"reflect"
	"testing"
	"time"

	apis "kubevirt.io/node-maintenance-operator/pkg/apis"
	operator "kubevirt.io/node-maintenance-operator/pkg/apis/kubevirt/v1alpha1"

	"bytes"
	"io"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	retryInterval                = time.Second * 5
	timeout                      = time.Second * 120
	cleanupRetryInterval         = time.Second * 1
	cleanupTimeout               = time.Second * 5
	testDeployment               = "testdeployment"
	testDeploymentReplicas int32 = 2
	podLabel                     = map[string]string{"test": "drain"}
	operatorLabel                = map[string]string{"name": "node-maintenance-operator"}
	masterLabelKey               = "node-role.kubernetes.io/master"
)

func getCurrentOperatorPods(KubeClient kubernetes.Interface) (*corev1.Pod, error) {

	pods, err := KubeClient.CoreV1().Pods("node-maintenance-operator").List(metav1.ListOptions{LabelSelector: "name=node-maintenance-operator"})
	if err != nil {
		return nil, err
	}

	if pods.Size() == 0 {
		return nil, fmt.Errorf("There are no pods deployed in cluster to run the operator")
	}

	return &pods.Items[0], nil
}

func showDeploymentStatus(t *testing.T, f *framework.Framework) {

	pod, err := getCurrentOperatorPods(f.KubeClient)
	if err != nil {
		t.Fatalf("showDeployment: can't get operator deployment error=%v", err)
		return
	}
	podName := pod.ObjectMeta.Name
	podLogOpts := corev1.PodLogOptions{}

	req := f.KubeClient.CoreV1().Pods("node-maintenance-operator").GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream()
	if err != nil {
		t.Fatalf("showDeployment: can't get log stream error=%v", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		t.Fatalf("showDeployment: can't copy log stream error=%v", err)
		return
	}
	str := buf.String()
	t.Fatalf("operator logs: %s", str)
}

func TestNodeMainenance(t *testing.T) {
	nodeMainenanceList := &operator.NodeMaintenanceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "nodemaintenance.kubevirt.io/v1beta1",
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

	// Count worker nodes to spread the test deployment on all of them
	computeNodesNumber := countWorkerNodes(t, f)

	if computeNodesNumber < 2 {
		t.Fatal("not enough worker nodes in the cluster, expecting at least 2")
	}

	testDeploymentReplicas = int32(computeNodesNumber)

	// Create a test deployment with pod anti affinity to have a pod on each node
	err = createSimpleDeployment(t, f, ctx, namespace, testDeploymentReplicas)
	if err != nil {
		t.Fatal(err)
	}

	// Drain the node the operator is deployed on, in order to test
	// if it will be recovered and complete the drain operation from a different node
	// note: this node will have a test deployment pod because of its replica count
	// and anti pod affinity
	nodeName := getOperatorHostname(t, f)

	t.Logf("Putting node %s into maintanance", nodeName)

	// Create node maintenance custom resource
	nodeMaintenance := &operator.NodeMaintenance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "nodemaintenance.kubevirt.io/v1beta1",
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
	err = f.Client.Create(goctx.TODO(), nodeMaintenance, &framework.CleanupOptions{
		TestContext: ctx,
		Timeout: cleanupTimeout,
		RetryInterval: cleanupRetryInterval,
	})

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
		showDeploymentStatus(t, f)
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
		showDeploymentStatus(t, f)
		t.Fatal(fmt.Errorf("Failed to successfuly complete maintanance operation after defined test timeout (120s)"))
	}

	node := &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		t.Fatal(err)
	}

	if node.Spec.Unschedulable == false {
		checkFailureStatus(t, f)
		showDeploymentStatus(t, f)
		t.Fatal(fmt.Errorf("Node %s should have been unschedulable ", nodeName))
	}

	if !kubevirtTaintExist(node) {
		checkFailureStatus(t, f)
		showDeploymentStatus(t, f)
		t.Fatal(fmt.Errorf("Node %s should have been tainted with kubevirt.io/drain:NoSchedule", nodeName))
	}

	// Check that the deployment has full replicas running after maintenance
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, int(testDeploymentReplicas), retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the operator deployment has full recplicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "node-maintenance-operator", 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	operatorNewNodeName := getOperatorHostname(t, f)

	if testDeploymentExistsOnNode(t, f, nodeName) || operatorNewNodeName == nodeName {
		t.Fatal(fmt.Errorf("Deployment was done on node %s that should be under maintanence", nodeName))
	}

	t.Logf("Setting node %s out of maintanance", nodeName)

	nodeMaintenanceDelete := &operator.NodeMaintenance{}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: nodeMaintenance.Namespace, Name: nodeMaintenance.Name}, nodeMaintenanceDelete)
	if err != nil {
		showDeploymentStatus(t, f)
		t.Fatal(err)
	}

	// Delete the node maintenance custom resource
	err = f.Client.Delete(goctx.TODO(), nodeMaintenanceDelete)
	if err != nil {
		showDeploymentStatus(t, f)
		t.Fatalf("Could not delete node maintenance CR : %v", err)
	}

	time.Sleep(60 * time.Second)

	node = &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		t.Fatal(err)
	}

	if node.Spec.Unschedulable == true {
		showDeploymentStatus(t, f)
		t.Fatal(fmt.Errorf("Node %s should have been schedulable", nodeName))
	}

	if kubevirtTaintExist(node) {
		showDeploymentStatus(t, f)
		t.Fatal(fmt.Errorf("Node %s kubevirt.io/drain:NoSchedule taint should have been removed", nodeName))
	}

	// Check that the deployment has full replica running after maintenance is removed.
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, int(testDeploymentReplicas), retryInterval, timeout)
	if err != nil {
		showDeploymentStatus(t, f)
		t.Fatal(err)
	}

	return nil
}

func createSimpleDeployment(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, namespace string, replicas int32) error {
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
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 1,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: metav1.SetAsLabelSelector(podLabel),
										TopologyKey:   "kubernetes.io/hostname",
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
	// wait for testPodDeployment to reach full replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, int(testDeploymentReplicas), retryInterval, timeout)
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
	if nm.Status.Phase != operator.MaintenanceRunning {
		t.Logf("Status.Phase on %s nodeMaintenance should have been %s", nm.Name, operator.MaintenanceRunning)
	}
	if len(nm.Status.LastError) == 0 {
		t.Logf("Status.LastError on %s nodeMaintenance should have a value", nm.Name)
	}
	if len(nm.Status.PendingPods) > 0 {
		pods, err := getCurrentDeploymentPods(t, f)
		if err != nil {
			t.Logf("Failed to get deployment pods")
		}
		if pods.Items[0].Name != nm.Status.PendingPods[0] {
			t.Logf("Status.PendingPods on %s nodeMaintenance does not contain pod %s", nm.Name, pods.Items[0].Name)
		}
	}
}

func getOperatorHostname(t *testing.T, f *framework.Framework) string {
	pods := &corev1.PodList{}

	err := f.Client.List(goctx.TODO(), &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(operatorLabel),
	}, pods)

	if err != nil {
		t.Fatalf("failed listing operator pods: %v", err)
	}

	if len(pods.Items) < 1 {
		t.Fatal("there are no operator pods")
	}

	return pods.Items[0].Spec.NodeName
}

func countWorkerNodes(t *testing.T, f *framework.Framework) int {
	nodes := corev1.NodeList{}
	labelSelector := labels.NewSelector()

	requirement, err := labels.NewRequirement(masterLabelKey, selection.DoesNotExist, []string{})
	if err != nil {
		t.Fatalf("failed creating selector requirement: %v", err)
	}

	labelSelector = labelSelector.Add(*requirement)
	err = f.Client.List(goctx.TODO(), &client.ListOptions{
		LabelSelector: labelSelector,
	}, &nodes)

	if err != nil {
		t.Fatalf("failed listing worker nodes: %v", err)
	}

	return len(nodes.Items)
}

func testDeploymentExistsOnNode(t *testing.T, f *framework.Framework, nodeName string) bool {
	return sliceContainsString(getTestDeploymentHostnames(t, f), nodeName)
}

func getTestDeploymentHostnames(t *testing.T, f *framework.Framework) []string {
	pods, err := getCurrentDeploymentPods(t, f)

	if err != nil {
		t.Fatalf("could not list test deployment pods: %v", err)
	}

	hostnames := []string{}

	for _, pod := range pods.Items {
		if !sliceContainsString(hostnames, pod.Spec.NodeName) {
			hostnames = append(hostnames, pod.Spec.NodeName)
		}
	}

	return hostnames
}

func sliceContainsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}

	return false
}
