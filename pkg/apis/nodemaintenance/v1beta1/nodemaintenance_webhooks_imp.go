package v1beta1

import(
	"fmt"
	kubernetes "k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

var HookClient kubernetes.Interface

func SetHookClient(client kubernetes.Interface) {
	HookClient = client
}

func isValidNodeName(nodeName string) (bool, error) {
	if HookClient == nil {
		return false, fmt.Errorf("client-go has not been initialized")
	}
	_, err := HookClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
