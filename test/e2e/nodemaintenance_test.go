package e2e

import (
	goctx "context"
	"fmt"
	"reflect"
	"testing"
	"time"
	apis "kubevirt.io/node-maintenance-operator/pkg/apis"
	operator "kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
	nmo "kubevirt.io/node-maintenance-operator/pkg/controller/nodemaintenance"
	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	"bytes"
	"io"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"k8s.io/apimachinery/pkg/api/errors"
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
	retryInterval        = time.Second * 5
	timeout              = time.Second * 120
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 5
	testDeployment       = "testdeployment"
	podLabel             = map[string]string{"test": "drain"}
	testIterations		 = 4
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

func showDeploymentStatus(t *testing.T, f *framework.Framework, callerError error) {

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
		t.Errorf("showDeployment: can't get log stream error=%v", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		t.Errorf("showDeployment: can't copy log stream error=%v", err)
	}
	str := buf.String()

	if callerError != nil {
		t.Fatalf("error: %v operator logs: %s", callerError, str)
	} else {
		t.Logf("operator logs %s", str)
	}
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

func  checkValidLease(t *testing.T, f *framework.Framework, nodeName string) error {
	nName := types.NamespacedName{Namespace: nmo.LeaseNamespace, Name: nodeName}
	lease := &coordv1beta1.Lease{}
	err := f.Client.Get(goctx.TODO(), nName, lease)
	if err != nil {
		return fmt.Errorf("can't get lease node %s : %v", nodeName, err)
	}

	if lease.Spec.LeaseDurationSeconds == nil || *lease.Spec.LeaseDurationSeconds != nmo.LeaseDurationInSeconds {
		return fmt.Errorf("checkValidLease wrong LeaseDurationInSeconds")
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != nmo.LeaseHolderIdentity {
		return fmt.Errorf("checkValidLease wrong Spec.HolderIdentity")
	}

	if lease.Spec.RenewTime == nil  {
		return fmt.Errorf("checkValidLease nil RenewTime")
	}

	timeNow := time.Now()
	if  (*lease.Spec.RenewTime).Time.After(timeNow)  {
		return fmt.Errorf("checkValidLease RenewTime in the future current time %s renew time %s", timeNow.Format(time.UnixDate), (*lease.Spec.RenewTime).Format(time.UnixDate))

	}
	tm := (*lease.Spec.RenewTime).Time
	tm = tm.Add( time.Duration(int64(*lease.Spec.LeaseDurationSeconds) * int64(time.Second))  )
	if  tm.Before(timeNow) {
		return fmt.Errorf("checkValidLease expiration time in the past time Now: %s expiration time %s :: leaseDuration %d renew time %s", timeNow.Format(time.UnixDate), tm.Format(time.UnixDate), *lease.Spec.LeaseDurationSeconds, (*lease.Spec.RenewTime).Time.Format(time.UnixDate))
	}

	return nil
}

func  checkInvalidLease(t *testing.T, f *framework.Framework, nodeName string) error {
	nName := types.NamespacedName{Namespace: nmo.LeaseNamespace, Name: nodeName}
	lease := &coordv1beta1.Lease{}
	err := f.Client.Get(goctx.TODO(), nName, lease)
	if err != nil {
		return fmt.Errorf("can't get lease node %s : %v", nodeName, err)
	}

	if lease.Spec.AcquireTime != nil  {
		return fmt.Errorf("AcquireTime not nil %s", nodeName)
	}
	if lease.Spec.LeaseDurationSeconds  != nil {
		return fmt.Errorf("LeaseDurationSeconds not nil %s", nodeName)
	}
	if lease.Spec.RenewTime != nil {
		return fmt.Errorf("RenewTime not nil %s", nodeName)
	}
	if lease.Spec.LeaseTransitions != nil {
		return fmt.Errorf("LeaseTransitions not nil %s", nodeName)
	}

	return nil
}

func  enterAndExitMaintenanceMode(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
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
	err = f.Client.Create(goctx.TODO(), nodeMaintenance, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatalf("Can't create CRD: %v", err)
	}

	// Get Running phase first
	if err := wait.PollImmediate(1*time.Second, 20*time.Second, func() (bool, error) {
		nm := &operator.NodeMaintenance{}
		err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
		if err != nil {
			t.Logf("Failed to get %q nodeMaintenance: %v", nm.Name, err)
			return false, nil
		}

		//t.Logf("%s: operator running state: %s", time.Now().Format("2006-01-02 15:04:05.000000"), nm.Status.Phase)
		if nm.Status.Phase != operator.MaintenanceRunning {
			t.Logf("%s: Running", time.Now().Format("2006-01-02 15:04:05.000000"))
			return false, nil
		}
		return true, nil
	}); err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("%s: Failed to verify running phase: %v", time.Now().Format("2006-01-02 15:04:05.000000"), err))
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
		showDeploymentStatus(t, f, fmt.Errorf("Failed to successfuly complete maintanance operation after defined test timeout (120s)"))
	}

	node := &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("Failed to get CRD after entering main. mode : %v", err))
	}

	if node.Spec.Unschedulable == false {
		checkFailureStatus(t, f)
		showDeploymentStatus(t, f, fmt.Errorf("Node %s should have been unschedulable ", nodeName))
	}

	if !kubevirtTaintExist(node) {
		checkFailureStatus(t, f)
		showDeploymentStatus(t, f, fmt.Errorf("Node %s should have been tainted with kubevirt.io/drain:NoSchedule", nodeName))
	}

	nodesList := &corev1.NodeList{}
	err = f.Client.List(goctx.TODO(), &client.ListOptions{}, nodesList)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("Failed to list nodes %v", err))
	}

	computeNodesNumber := 0

	for _, node := range nodesList.Items {
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; !exists {
			computeNodesNumber++
		}
	}

	err = checkValidLease(t, f, nodeName)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("no valid lease after nmo completion: %v", err))
	}

	if computeNodesNumber > 2 {
		// Check that the deployment has 1 replica running after maintenance
		err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, 1, retryInterval, timeout)
		if err != nil {
			showDeploymentStatus(t, f, fmt.Errorf("failed to wait for deployment. error %v", err))
		}

		newNodeName, err := getCurrentDeploymentHostName(t, f)
		if err != nil {
			showDeploymentStatus(t, f, err)
		}

		if newNodeName == nodeName {
			showDeploymentStatus(t, f, fmt.Errorf("Deployment was done on node %s that should be under maintanence", nodeName))
		}
	}

	t.Logf("Setting node %s out of maintanance", nodeName)

	nodeMaintenanceDelete := &operator.NodeMaintenance{}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: nodeMaintenance.Namespace, Name: nodeMaintenance.Name}, nodeMaintenanceDelete)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("Failed to get CRD. error %v", err))
	}

	// Delete the node maintenance custom resource
	err = f.Client.Delete(goctx.TODO(), nodeMaintenanceDelete)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("Could not delete node maintenance CR : %v", err))
	}

	time.Sleep(60 * time.Second)

	node = &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("can't get CRD. error %v", err))
	}

	if node.Spec.Unschedulable == true {
		showDeploymentStatus(t, f, fmt.Errorf("Node %s should have been schedulable", nodeName))
	}

	if kubevirtTaintExist(node) {
		showDeploymentStatus(t, f, fmt.Errorf("Node %s kubevirt.io/drain:NoSchedule taint should have been removed", nodeName) )
	}

	err = checkInvalidLease(t, f, nodeName)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("valid lease after nmo completion %v", err))
	}

	// Check that the deployment has 1 replica running after maintenance is removed.
	t.Logf("%s: wait for deployment.", time.Now().Format("2006-01-02 15:04:05.000000"))
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testDeployment, 1, retryInterval, timeout)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("%s: failed to wait for deployment. error %v.", err, time.Now().Format("2006-01-02 15:04:05.000000")))
	}

	err = deleteSimpleDeployment(t, f, ctx, namespace)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("test deployment deleted")

	return nil
}

func nodeMaintenanceTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {

	for i:=0; i < testIterations; i+=1 {
		if err := enterAndExitMaintenanceMode(t, f, ctx); err != nil {
			t.Fatalf("failed to enter maintenance mode. error %v", err);
		}
	}

	return nil
}

func deleteSimpleDeployment(t *testing.T, f *framework.Framework, ctx *framework.TestCtx, namespace string) error {

	deploymentToDelete := &appsv1.Deployment{}

	namespaceName := types.NamespacedName{Namespace: namespace, Name: testDeployment}
	err := f.Client.Get(goctx.TODO(), namespaceName, deploymentToDelete)
	if err != nil {
		t.Logf("Failed to get deploymten %v error %v", namespaceName, err)
		return err
	}

	err = f.Client.Delete(goctx.TODO(), deploymentToDelete)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete  %v: %v", namespaceName, err)
	}

	return wait.PollImmediate(1*time.Second, 20*time.Second, func() (bool, error) {

				err = f.Client.Get(goctx.TODO(), namespaceName, deploymentToDelete)
				if err != nil {
					if errors.IsNotFound(err) {
						return true, nil
					}
					return false, fmt.Errorf("error encountered during deletion of deployment: %v", err)
				}
				return false, nil
	})
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
		return nil, fmt.Errorf("Can't get test pods %v", err)
	}

	if pods.Size() == 0 {
		return nil, fmt.Errorf("There are no test pods deployed in cluster")
	}

	return pods, nil
}

func getCurrentDeploymentHostName(t *testing.T, f *framework.Framework) (string, error) {
	pods, err := getCurrentDeploymentPods(t, f)
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no test pods running pods for test deployment")
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
		return
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
			t.Logf("Failed to get deployment pods %v", err)
		} else {
			if len(pods.Items) != 0 && pods.Items[0].Name != nm.Status.PendingPods[0] {
				t.Logf("Status.PendingPods on %s nodeMaintenance does not contain pod %s", nm.Name, pods.Items[0].Name)
			} else  {
				t.Logf("no deployment pods found")
			}
		}
	}
}
