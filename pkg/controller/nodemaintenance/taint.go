package nodemaintenance

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	kubernetes "k8s.io/client-go/kubernetes"
)

var KubevirtDrainTaint = &corev1.Taint{
	Key:    "kubevirt.io/drain",
	Effect: corev1.TaintEffectNoSchedule,
}

func AddOrRemoveTaint(clientset kubernetes.Interface, node *corev1.Node, add bool) error {

	taintStr := ""
	patch := ""
	client := clientset.Core().Nodes()

	oldTaints, err := json.Marshal(node.Spec.Taints)
	if err != nil {
		return err
	}

	newTaints, err := json.Marshal([]corev1.Taint{*KubevirtDrainTaint})
	if err != nil {
		return err
	}

	if add {
		taintStr = "add"
		log.Info(fmt.Sprintf("Taints: %s will be added to node %s", string(newTaints), node.Name))
		patch = fmt.Sprintf(`{ "op": "add", "path": "/spec/taints", "value": %s }`, string(newTaints))
	} else {
		taintStr = "remove"
		log.Info(fmt.Sprintf("Taints: %s will be removed from node %s", string(newTaints), node.Name))
		patch = fmt.Sprintf(`{ "op": "remove", "path": "/spec/taints", "value": %s }`, string(newTaints))
	}

	log.Info(fmt.Sprintf("Applying %s taint %s on Node: %s", KubevirtDrainTaint.Key, taintStr, node.Name))

	test := fmt.Sprintf(`{ "op": "test", "path": "/spec/taints", "value": %s }`, string(oldTaints))
	log.Info(fmt.Sprintf("Patching taints on Node: %s", node.Name))
	_, err = client.Patch(node.Name, types.JSONPatchType, []byte(fmt.Sprintf("[ %s, %s ]", test, patch)))
	if err != nil {
		return fmt.Errorf("patching node taints failed: %v", err)
	}

	return nil
}
