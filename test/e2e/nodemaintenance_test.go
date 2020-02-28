package e2e

import (
	goctx "context"
	"fmt"
	"os"
	"os/exec"
    "io/ioutil"
	"reflect"
	"strings"
	"testing"
	"time"

	apis "kubevirt.io/node-maintenance-operator/pkg/apis"
	operator "kubevirt.io/node-maintenance-operator/pkg/apis/kubevirt/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	appsv1 "k8s.io/api/apps/v1"
	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

const (
	NodeMaintenanceSpecAnnotation   = "lifecycle.openshift.io/maintenance"
	NodeMaintenanceStatusAnnotation = "lifecycle.openshift.io/maintenance-status"
	LeaseHolderIdentity             = "node-maintenance"
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

func setSpecAnnotation(node *corev1.Node, newValue string) error {

	if newValue != "" {
		node.ObjectMeta.Annotations[NodeMaintenanceSpecAnnotation] = newValue
	} else {
		delete(node.ObjectMeta.Annotations, NodeMaintenanceSpecAnnotation)
	}

	f := framework.Global
	client := f.KubeClient.Core().Nodes()

	_, err := client.Update(node)
	if err != nil {
		return fmt.Errorf("setAnnotation: update node failed. error: %v", err)
	}
	return nil
}

func isLeasePeriodSpecified(lease *coordv1beta1.Lease) bool {
	return lease.Spec.AcquireTime != nil && lease.Spec.LeaseDurationSeconds != nil
}

// check if lease is not expired (precondition that both AcquireTime and LeaseDuration are not nil)
func isLeaseValid(lease *coordv1beta1.Lease) bool {

	if lease.Spec.AcquireTime == nil {
		// can this happen?
		return false
	}

	leasePeriodEnd := (*lease.Spec.AcquireTime).Time
	if lease.Spec.LeaseDurationSeconds != nil {
		leasePeriodEnd = leasePeriodEnd.Add(time.Duration(int64(*lease.Spec.LeaseDurationSeconds) * int64(time.Second)))
	}

	timeNow := time.Now()

	return !timeNow.After(leasePeriodEnd)
}

func checkHasLease(t *testing.T, nodeName string, durationValid bool) {

	lease := &coordv1beta1.Lease{}
	nName := types.NamespacedName{Namespace: corev1.NamespaceNodeLease, Name: nodeName}

	f := framework.Global

	if err := f.Client.Get(goctx.TODO(), nName, lease); err != nil {
		if errors.IsNotFound(err) {
			t.Fatal(fmt.Errorf("Lease object not found name=%s ns=%s", nName.Name, nName.Namespace))
			return
		}

		t.Fatal(fmt.Errorf("failed to get lease object. name=%s ns=%s error=%v", nName.Name, nName.Namespace, err))
		return
	}

	heldByMe := lease.Spec.HolderIdentity != nil && LeaseHolderIdentity == *lease.Spec.HolderIdentity

	if durationValid {
		if !heldByMe {
			t.Fatal(fmt.Errorf("lease not held by me"))
			return
		}
		if !isLeasePeriodSpecified(lease) || !isLeaseValid(lease) {
			t.Fatal(fmt.Errorf("lease not valid."))
		}

	} else {

		if heldByMe && isLeasePeriodSpecified(lease) && isLeaseValid(lease) {
			t.Fatal(fmt.Errorf("lease still valid."))
		}
	}

}

const ( scriptGetStatus = `#!/bin/bash -x

OC="./cluster/kubectl.sh"

pwd
ls .

echo "*** dump operator status & logs ***"

$OC get pods -n node-maintenance-operator

POD_NAME=$($OC get pods -n node-maintenance-operator  | grep node-maintenance-operator | awk '{print $1}')

echo "pod name: ${POD_NAME}"

$OC describe pod -n node-maintenance-operator  $POD_NAME

$OC logs -n node-maintenance-operator ${POD_NAME} -c node-maintenance-operator
`)

func showDeploymtentStatus(t *testing.T) {
	mydir, _ := os.Getwd()
	t.Logf("directory: %s", mydir)

    scriptName := "./opstat.sh"
    ferr := ioutil.WriteFile( scriptName, []byte(scriptGetStatus),0755)
    if ferr != nil {
        t.Logf("can't write script. err=%v\n", ferr)
    }

	clicmd := exec.Command(scriptName)
	output, err := clicmd.CombinedOutput()
	if err != nil {
        cmd := string(output)
		t.Logf("can't get cluster status: %v comand: %s\n", err, cmd)
	} else {
		soutput := string(output)
		fmt.Printf("cluster status: %s\n", soutput)
		t.Logf("cluster status: %s", output)
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

	// get the node object.
	node, errn := f.KubeClient.Core().Nodes().Get(nodeName, metav1.GetOptions{})
	if errn != nil {
		t.Fatal(fmt.Errorf("Failed to get node. error: %v", errn))
	}

	errn = setSpecAnnotation(node, "3600")
	if errn != nil {
		t.Fatal(fmt.Errorf("Failed to set maint. mode annotation. error: %v", errn))
	}

	// Get Running phase first
	if err := wait.PollImmediate(1*time.Second, 10*time.Second, func() (bool, error) {

		node, err := f.KubeClient.Core().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			t.Logf("Failed to get node %s error: %v", nodeName, err)
			return false, nil
		}

		val, hasAnnotation := node.ObjectMeta.Annotations[NodeMaintenanceSpecAnnotation]
		if hasAnnotation {
			t.Logf("spec annotation %s", val)
		} else {
			t.Logf("no spec annotation yet")
		}

		val, hasAnnotation = node.ObjectMeta.Annotations[NodeMaintenanceStatusAnnotation]

		if hasAnnotation {
			t.Logf("status annotation %s", val)
			if strings.HasPrefix(val, "new") {
				return true, nil
			}
		} else {
			t.Logf("no status annotation yet")
		}
		return false, nil
	}); err != nil {
		showDeploymtentStatus(t)
		t.Fatal(fmt.Errorf("Failed to verify running phase: %v", err))
	}

	t.Logf("node %s transition to maintenance mode ongoing", nodeName)

	checkHasLease(t, nodeName, true)

	// Get Running phase first
	if err := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {

		node, err := f.KubeClient.Core().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			t.Logf("Failed to get node %s error: %v", nodeName, err)
			return false, nil
		}

		val, hasAnnotation := node.ObjectMeta.Annotations[NodeMaintenanceStatusAnnotation]

		if hasAnnotation && val == "active" {
			return true, nil
		}

		return false, nil
	}); err != nil {
		showDeploymtentStatus(t)
		t.Fatal(fmt.Errorf("Failed to verify running phase: %v", err))
	}

	t.Logf("node %s maintenance mode active", nodeName)

	checkHasLease(t, nodeName, true)

	node = &corev1.Node{}
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
		showDeploymtentStatus(t)
		t.Fatal(fmt.Errorf("Node %s should have been tainted with kubevirt.io/drain:NoSchedule", nodeName))
	}

	nodesList := &corev1.NodeList{}
	err = f.Client.List(goctx.TODO(), &client.ListOptions{}, nodesList)
	if err != nil {
		showDeploymtentStatus(t)
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

	// get the node object.
	node, errn = f.KubeClient.Core().Nodes().Get(nodeName, metav1.GetOptions{})
	if errn != nil {
		t.Fatal(fmt.Errorf("Failed to get node. error: %v", errn))
	}

	errn = setSpecAnnotation(node, "")
	if errn != nil {
		t.Fatal(fmt.Errorf("Failed to set maint. mode annotation. error: %v", errn))
	}

	if err := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {

		node, err := f.KubeClient.Core().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			t.Logf("Failed to get node %s error: %v", nodeName, err)
			return false, nil
		}

		val, hasAnnotation := node.ObjectMeta.Annotations[NodeMaintenanceStatusAnnotation]

		if hasAnnotation && val == "ended" {
			return true, nil
		}

		return false, nil
	}); err != nil {
		showDeploymtentStatus(t)
		t.Fatal(fmt.Errorf("Failed to verify running phase: %v", err))
	}

	if node.Spec.Unschedulable == false {
		t.Fatal(fmt.Errorf("Node %s should have been schedulable", nodeName))
	}

	if !kubevirtTaintExist(node) {
		t.Fatal(fmt.Errorf("Node %s kubevirt.io/drain:NoSchedule taint should have been removed", nodeName))
	}

	checkHasLease(t, nodeName, false)

	t.Logf("node %s is operational again", nodeName)

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
