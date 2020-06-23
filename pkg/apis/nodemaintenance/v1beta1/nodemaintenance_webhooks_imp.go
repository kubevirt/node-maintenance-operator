package v1beta1

import(
	"context"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var HookClient	client.Client

func SetHookClient(client client.Client) {
	HookClient = client
}

func isValidNodeName(nodeName string) (bool, error) {
	if HookClient == nil {
		return false, fmt.Errorf("client-go has not been initialized")
	}

	node :=  &corev1.Node{}
	err := HookClient.Get(context.TODO(), types.NamespacedName{ Name: nodeName }, node)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isNMOObjectWorkingOnNode(nodeName string) (bool, error) {
	nmos := &NodeMaintenanceList{}
	err := HookClient.List(context.TODO(),nmos)
	if  err != nil && !errors.IsNotFound(err) {
		return false, err
	}
	for i :=0 ; i < len(nmos.Items); i += 1 {
		nmo := nmos.Items[i]
		if nmo.Spec.NodeName == nodeName {
			return true, nil
		}
	}
	return false, nil
}
