package nodemaintenance

import (
	log "github.com/sirupsen/logrus"
	kubevirtv1alpha1 "kubevirt.io/node-maintenance-operator/pkg/apis/kubevirt/v1alpha1"
	"kubevirt.io/node-maintenance-operator/pkg/webhooks"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"fmt"
)

const (
	NMONamespace = "node-maintenance-operator"
	NMOPluralName="nodemaintenances"
)

var signalHandler <-chan struct{}

func SetSignalHandler(handler <-chan struct{}) {
	signalHandler = handler
}

func (r *ReconcileNodeMaintenance) validateCRDAdmission(crd runtime.Object) error {

	switch crdObj := crd.(type) {
		case *kubevirtv1alpha1.NodeMaintenance:
			nodeName := crdObj.Spec.NodeName
			log.Infof("Webhook: request to create nmo object on node %s", nodeName)
			node, err := r.drainer.Client.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
			if node == nil || err != nil {
				return fmt.Errorf("Can't create NMO object for node %s. The node does not exist", nodeName)
			}

		default:
			return fmt.Errorf("unexpected type %T", crdObj)
	}
	return nil
}


func (r *ReconcileNodeMaintenance) StartNMOWebhook(mgr manager.Manager) error {

		var callback = func(obj runtime.Object) error {
			return r.validateCRDAdmission(obj)
		}
		return webhooks.StartAdmissionWebhook(mgr.GetConfig(), mgr.GetScheme(), signalHandler, kubevirtv1alpha1.SchemeGroupVersion, NMOPluralName, NMONamespace, callback)
}


