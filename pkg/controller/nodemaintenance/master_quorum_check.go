package nodemaintenance

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policy "k8s.io/api/policy/v1beta1"
	kubernetes "k8s.io/client-go/kubernetes"
)

const OpenshiftMachineConfigNamespace = "openshift-machine-config-operator"

func checkValidQuorum(client kubernetes.Interface, node *corev1.Node) (bool, error) {
	if isMasterNode(client, node) {
		validQuorum, err := checkValidQuorumPdb(client)
		if err != nil {
			return false, err
		}
		log.Infof("node %s is a master. quorum status: %t", node.Name, validQuorum)
		return validQuorum, nil
	} else {
		log.Infof("node %s is not a master", node.Name)
	}
	return true, nil
}

func findMcoPDBForEtcd(client kubernetes.Interface) (*policy.PodDisruptionBudget,error) {
	pdbList, err := client.Policy().PodDisruptionBudgets(OpenshiftMachineConfigNamespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("can't get mco pdb: %v", err)
	}
	if pdbList.Size() == 0  || pdbList.Items == nil || len(pdbList.Items) == 0 {
		return nil, fmt.Errorf("mco pdb is empty")
	}
	return &pdbList.Items[0], nil
}

func isMasterNode(client kubernetes.Interface, node *corev1.Node) bool {
	if node.ObjectMeta.Labels != nil {
		_, isMaster := node.ObjectMeta.Labels[MasterNodeLabel]
		return isMaster
	}
	return false
}

func checkValidQuorumPdb(client kubernetes.Interface) (bool, error) {

	pdb, err := findMcoPDBForEtcd(client)
	if err != nil {
		return false, err
	}

	minAvailable := int32(pdb.Spec.MinAvailable.IntValue())
	currentHealthy := pdb.Status.CurrentHealthy
	disruptedPodLen := int32(len(pdb.Status.DisruptedPods))
	isValidQuorum := minAvailable <= currentHealthy-disruptedPodLen-1

	log.Infof("master quorum isvalid: %t machine configuration pdb: spec.minAvailable: %d current healthy: %d disruptedPods %d", isValidQuorum, minAvailable, currentHealthy, disruptedPodLen)

	return isValidQuorum, nil
}

