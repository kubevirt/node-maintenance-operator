package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"
	operator "kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
	nmo "kubevirt.io/node-maintenance-operator/pkg/controller/nodemaintenance"
	nmooperator "kubevirt.io/node-maintenance-operator/pkg/controller/nodemaintenance"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"k8s.io/apimachinery/pkg/api/errors"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policy "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func createEtcPDB(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) (*policy.PodDisruptionBudget, *corev1.Namespace, error) {
	nsobject := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nmooperator.OpenshiftMachineConfigNamespace,
			Labels: map[string]string{
				"name":	nmooperator.OpenshiftMachineConfigNamespace,
			},
		},
	}
	err := f.Client.Create(goctx.TODO(), nsobject, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, nil, fmt.Errorf("can't create namespace : %v", err)
		}
		nsobject = nil
	}

	objectName  := "etcpdb"
	minAvailable := intstr.FromInt(1)
	pdb := &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: nmooperator.OpenshiftMachineConfigNamespace,
		},
		Spec: policy.PodDisruptionBudgetSpec{
			Selector:     &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k8s-app": "etcd-quorum-guard",
				},
			},
			MinAvailable: &minAvailable,
		},
		Status: policy.PodDisruptionBudgetStatus{},
	}

	err = f.Client.Create(goctx.TODO(), pdb, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			if nsobject != nil {
				f.Client.Delete(goctx.TODO(), nsobject)
			}
			return nil, nil, err
		}
		pdb  = nil
	}
	return pdb, nsobject, nil
}

func checkQuorumSizeViolation(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {

	pdb, nsobject, err:= createEtcPDB(t , f , ctx )
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("can't create test pdb :  %v", err))
	}
	t.Logf("test etc PDB created")

	namespace, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}

	err = createSimpleDeployment(t, f, ctx, namespace, true)
	if err != nil {
		t.Fatal(err)
	}

	nodeName, err := getCurrentDeploymentHostName(t, f)
	if err != nil {
		t.Fatal(err)
	}

	node := &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("Failed to get node %s : %v", nodeName, err))
	}

	_, isMaster := node.ObjectMeta.Labels[nmo.MasterNodeLabel]
	if !isMaster {
		showDeploymentStatus(t, f, fmt.Errorf("node %s is not a master node",nodeName))
	}

	t.Logf("Putting master node %s into maintanance", nodeName)

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

		if nm.Status.LastError != nmo.QuorumViolationErrorMsg {
			t.Logf("%s: Running, with wrong error: %s", time.Now().Format("2006-01-02 15:04:05.000000"), nm.Status.LastError)
			return false, nil
		}
		return true, nil
	}); err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("%s: Failed to verify running phase: %v", time.Now().Format("2006-01-02 15:04:05.000000"), err))
	}

	t.Logf("correct error has been observed")
	t.Logf("delete nmo for node  %s", nodeName)

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

	if err := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		nm := &operator.NodeMaintenance{}
		err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
		if err == nil {
			return false, nil
		}
		if !errors.IsNotFound(err) {
			return false, nil
		}
		return true, nil
	}); err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("%s: Failed to verify that nmo has been deleted %v", time.Now().Format("2006-01-02 15:04:05.000000"), err))
	}

	node = &corev1.Node{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		showDeploymentStatus(t, f, fmt.Errorf("can't get node. error %v", err))
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
	t.Logf("test checkQuorumSizeViolation completed successfully")

	if pdb != nil {
		err = f.Client.Delete(goctx.TODO(), pdb)
		if err != nil {
			showDeploymentStatus(t, f, fmt.Errorf("Could not delete pdb : %v", err))
		}
	}
	if nsobject != nil {
		err = f.Client.Delete(goctx.TODO(), nsobject)
		if err != nil {
			showDeploymentStatus(t, f, fmt.Errorf("Could not delete namespace : %v", err))
		}
	}
	return nil
}

