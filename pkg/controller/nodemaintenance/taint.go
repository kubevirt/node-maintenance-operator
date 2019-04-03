package nodemaintenance

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	taintutils "k8s.io/kubernetes/pkg/util/taints"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var taint = &corev1.Taint{
	Key:    "kubevirt.io/drain",
	Effect: corev1.TaintEffectNoSchedule,
}

func addTaint(c client.Client, node *corev1.Node) (bool, error) {
	freshNode, updated, err := taintutils.AddOrUpdateTaint(node, taint)
	if err != nil {
		return false, err
	}
	if updated {
		err = c.Update(context.TODO(), freshNode)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func removeTaint(c client.Client, node *corev1.Node) (bool, error) {
	freshNode, updated, err := taintutils.RemoveTaint(node, taint)
	if err != nil {
		return false, err
	}
	if updated {
		err = c.Update(context.TODO(), freshNode)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
