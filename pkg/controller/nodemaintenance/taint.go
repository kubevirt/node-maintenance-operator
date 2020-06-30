package nodemaintenance

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	kubernetes "k8s.io/client-go/kubernetes"
)

var KubevirtDrainTaint = &corev1.Taint{
	Key:    "kubevirt.io/drain",
	Effect: corev1.TaintEffectNoSchedule,
}

var NodeUnschedulableTaint = &corev1.Taint{
	Key:    "node.kubernetes.io/unschedulable",
	Effect: corev1.TaintEffectNoSchedule,
}
var MaintenanceTaints = []corev1.Taint{*NodeUnschedulableTaint, *KubevirtDrainTaint}

func AddOrRemoveTaint(clientset kubernetes.Interface, node *corev1.Node, add bool) error {

	taintStr := ""
	patch := ""
	client := clientset.CoreV1().Nodes()

	if add {
		newTaints := append([]corev1.Taint{}, MaintenanceTaints...)
		if !addTaints(node.Spec.Taints, &newTaints) {
			return nil
		}
		addTaints, err := json.Marshal(newTaints)
		if err != nil {
			return err
		}
		taintStr = "add"
		log.Infof("Maintenance taints will be added to node %s", node.Name)
		patch = fmt.Sprintf(`{ "op": "add", "path": "/spec/taints", "value": %s }`, string(addTaints))
	} else {
		newTaints := append([]corev1.Taint{}, node.Spec.Taints...)
		if !deleteTaints(MaintenanceTaints, &newTaints) {
			return nil
		}
		removeTaints, err := json.Marshal(newTaints)
		if err != nil {
			return err
		}
		taintStr = "remove"
		log.Infof("Maintenance taints  will be removed from node %s", node.Name)
		patch = fmt.Sprintf(`{ "op": "replace", "path": "/spec/taints", "value": %s }`, string(removeTaints))
	}

	oldTaints, err := json.Marshal(node.Spec.Taints)
	if err != nil {
		return err
	}

	log.Infof("Applying %s taint %s on Node: %s", KubevirtDrainTaint.Key, taintStr, node.Name)

	test := fmt.Sprintf(`{ "op": "test", "path": "/spec/taints", "value": %s }`, string(oldTaints))
	log.Infof("Patching taints on Node: %s", node.Name)
	_, err = client.Patch(node.Name, types.JSONPatchType, []byte(fmt.Sprintf("[ %s, %s ]", test, patch)))
	if err != nil {
		return fmt.Errorf("patching node taints failed: %v", err)
	}

	return nil
}

// addTaints adds the newTaints list to existing ones and updates the newTaints List.
func addTaints(oldTaints []corev1.Taint, newTaints *[]corev1.Taint) bool {
	oldTaintLen := len(oldTaints)
	for _, oldTaint := range oldTaints {
		existsInNew := false
		for _, taint := range *newTaints {
			if taint.MatchTaint(&oldTaint) {
				existsInNew = true
				break
			}
		}
		if !existsInNew {
			*newTaints = append(*newTaints, oldTaint)
		}
	}
	return len(*newTaints) != oldTaintLen
}

// deleteTaint removes all the taints that have the same key and effect to given taintToDelete.
func deleteTaint(taints []corev1.Taint, taintToDelete *corev1.Taint) []corev1.Taint {
	newTaints := []corev1.Taint{}
	for i := range taints {
		if taintToDelete.MatchTaint(&taints[i]) {
			continue
		}
		newTaints = append(newTaints, taints[i])
	}
	return newTaints
}

// deleteTaints deletes the given taints from the node's taintlist.
func deleteTaints(taintsToRemove []corev1.Taint, newTaints *[]corev1.Taint) bool {
	oldTaintLen := len(*newTaints)
	for _, taintToRemove := range taintsToRemove {
		*newTaints = deleteTaint(*newTaints, &taintToRemove)
	}
	return len(*newTaints) != oldTaintLen
}
