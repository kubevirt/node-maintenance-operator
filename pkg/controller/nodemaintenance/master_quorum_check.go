package nodemaintenance

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policy "k8s.io/api/policy/v1beta1"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/api/errors"
)

const OpenshiftMachineConfigNamespace = "openshift-machine-config-operator"

func checkValidQuorum(client kubernetes.Interface, node *corev1.Node) (bool, bool, error) {
	if isMasterNode(client, node) {
		validQuorum, fallbackPDB, err := checkValidQuorumPdb(client)
		if err != nil {
			return false, false, err
		}
		log.Infof("node %s is a master. quorum status: %t", node.Name, validQuorum)
		return validQuorum, fallbackPDB, nil
	} else {
		log.Infof("node %s is not a master", node.Name)
	}
	return true, false, nil
}

func findMcoPDBForEtcd(client kubernetes.Interface) (*policy.PodDisruptionBudget,bool,error) {
	pdbList, err := client.Policy().PodDisruptionBudgets(OpenshiftMachineConfigNamespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("can't get mco pdb: %v", err)
	}
	if pdbList.Size() == 0  || pdbList.Items == nil || len(pdbList.Items) == 0 {
		return nil, true, fmt.Errorf("mco pdb is empty")
	}
	return &pdbList.Items[0], false, nil
}

func isMasterNode(client kubernetes.Interface, node *corev1.Node) bool {
	if node.ObjectMeta.Labels != nil {
		_, isMaster := node.ObjectMeta.Labels[MasterNodeLabel]
		return isMaster
	}
	return false
}

func createFallbackPDB(client kubernetes.Interface) (*policy.PodDisruptionBudget,error) {

	objectName  := "etcpdb"
	minAvailable := intstr.FromInt(1)
	object := &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: NodeMaintenanceOperatorNamespace,
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

	_, err := client.PolicyV1beta1().PodDisruptionBudgets(NodeMaintenanceOperatorNamespace).Create(object)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			object, err = client.PolicyV1beta1().PodDisruptionBudgets(NodeMaintenanceOperatorNamespace).Get(objectName,metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
		}
	}

	return object, nil
}

func checkValidQuorumPdb(client kubernetes.Interface) (bool, bool, error) {

	fallbackPDB := false
	pdb, pdbNotFound, err := findMcoPDBForEtcd(client)
	if pdbNotFound {
		fallbackPDB = true
		pdb, err = createFallbackPDB(client)
	}
	if err != nil {
		return false, false, err
	}

	minAvailable := int32(pdb.Spec.MinAvailable.IntValue())
	currentHealthy := pdb.Status.CurrentHealthy
	disruptedPodLen := int32(len(pdb.Status.DisruptedPods))
	isValidQuorum := minAvailable <= currentHealthy-disruptedPodLen-1

	log.Infof("master quorum isvalid: %t machine configuration pdb: spec.minAvailable: %d current healthy: %d disruptedPods %d fallbackPDB %t", isValidQuorum, minAvailable, currentHealthy, disruptedPodLen, fallbackPDB)

	return isValidQuorum, fallbackPDB, nil
}


