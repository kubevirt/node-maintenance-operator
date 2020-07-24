package v1beta1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var log = logger.Log.WithName("validator")

// introduce a NodeMaintenanceValidator, which gets a k8s client injected
type NodeMaintenanceValidator struct {
	client client.Client
}
var validator *NodeMaintenanceValidator

// implement webhook.Validator on NodeMaintenance
var _ webhook.Validator = &NodeMaintenance{}

func (nm *NodeMaintenance) ValidateCreate() error {
	log.Info("Validating NodeMaintenance creation", "name", nm.Name)
	if validator == nil {
		return fmt.Errorf("nodemaintenance validator isn't initialized yet")
	}
	return validator.ValidateCreate(nm)
}

func (nm *NodeMaintenance) ValidateUpdate(old runtime.Object) error {
	log.Info("Validating NodeMaintenance update", "name", nm.Name)
	if validator == nil {
		return fmt.Errorf("nodemaintenance validator isn't initialized yet")
	}
	return validator.ValidateUpdate(nm, old.(*NodeMaintenance))
}

func (nm *NodeMaintenance) ValidateDelete() error {
	log.Info("Validating NodeMaintenance deletion", "name", nm.Name)
	if validator == nil {
		return fmt.Errorf("nodemaintenance validator isn't initialized yet")
	}
	return nil
}

// Initialize the NodeMaintenanceValidator
func InitValidator(client client.Client) {
	validator = &NodeMaintenanceValidator{
		client: client,
	}
}

func (v *NodeMaintenanceValidator) ValidateCreate(nm *NodeMaintenance) error {
	// Validate that node with given name exists
	if err := v.validateNodeExists(nm.Spec.NodeName); err != nil {
		return err
	}

	// Validate that no NodeMaintenance for given node exists yet
	if err := v.validateNoNodeMaintenanceExists(nm.Spec.NodeName); err != nil {
		return err
	}

	return nil
}

func (v *NodeMaintenanceValidator) ValidateUpdate(new, old *NodeMaintenance) error {
	// Validate that node name didn't change
	if new.Spec.NodeName != old.Spec.NodeName {
		return fmt.Errorf("updating spec.NodeName isn't allowed")
	}
	return nil
}

func (v *NodeMaintenanceValidator) validateNodeExists(nodeName string) error {
	var node v1.Node
	key := types.NamespacedName{
		Name: nodeName,
	}
	if err := v.client.Get(context.TODO(), key, &node); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("invalid nodeName, no node with name %s found", nodeName)
		}
		return fmt.Errorf("could not get node for validating spec.NodeName, please try again: %v", err)
	}
	return nil
}

func (v *NodeMaintenanceValidator) validateNoNodeMaintenanceExists(nodeName string) error {
	var nodeMaintenances NodeMaintenanceList
	if err := v.client.List(context.TODO(), &nodeMaintenances, &client.ListOptions{}); err != nil {
		return fmt.Errorf("could not list NodeMaintenances for validating spec.NodeName, please try again: %v", err)
	}

	for _, nm := range nodeMaintenances.Items {
		if nm.Spec.NodeName == nodeName {
			return fmt.Errorf("invalid nodeName, a NodeMaintenance for node %s already exists", nodeName)
		}
	}
	return nil
}