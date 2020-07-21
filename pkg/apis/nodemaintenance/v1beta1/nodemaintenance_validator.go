package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ webhook.Validator = &NodeMaintenance{}
var log = logger.Log.WithName("validator")

func (nm *NodeMaintenance) ValidateCreate() error {
	log.Info("Validating NodeMaintenance creation", "name", nm.Name)
	return nil
}

func (nm *NodeMaintenance) ValidateUpdate(old runtime.Object) error {
	log.Info("Validating NodeMaintenance update", "name", nm.Name)
	return nil
}

func (nm *NodeMaintenance) ValidateDelete() error {
	log.Info("Validating NodeMaintenance deletion", "name", nm.Name)
	return nil
}
