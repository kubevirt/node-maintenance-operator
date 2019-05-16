package nodemaintenance

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	kubernetes "k8s.io/client-go/kubernetes"
	taintutils "k8s.io/kubernetes/pkg/util/taints"
)

var taint = &corev1.Taint{
	Key:    "kubevirt.io/drain",
	Effect: corev1.TaintEffectNoSchedule,
}

func AddOrRemoveTaint(clientset kubernetes.Interface, node *corev1.Node, add bool) error {

	taintStr := ""
	freshNode := &corev1.Node{}
	updated := false

	client := clientset.Core().Nodes()
	oldData, err := json.Marshal(node)
	if err != nil {
		return err
	}

	if add {
		taintStr = "add"
		freshNode, updated, err = taintutils.AddOrUpdateTaint(node, taint)

	} else {
		taintStr = "remove"
		freshNode, updated, err = taintutils.RemoveTaint(node, taint)

	}

	log.Info(fmt.Sprintf("Applying %s taint %s on Node: %s", taint.Key, taintStr, node.Name))

	if err != nil {
		return err
	}

	if !updated {
		log.Info(fmt.Sprintf("%s taint %s was not applied on Node: %s", taint.Key, taintStr, node.Name))
	} else {
		newData, err := json.Marshal(freshNode)
		if err != nil {
			return err
		}

		patchBytes, patchErr := strategicpatch.CreateTwoWayMergePatch(oldData, newData, node)
		if patchErr == nil {
			_, err = client.Patch(node.Name, types.StrategicMergePatchType, patchBytes)
		} else {
			log.Error(patchErr, fmt.Sprintf("%s taint %s could not be patched on Node: %s , performing update", taint.Key, taintStr, node.Name))
			_, err = client.Update(freshNode)
		}

		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("%s taint %s applied on Node: %s", taint.Key, taintStr, node.Name))
	}

	return nil
}
