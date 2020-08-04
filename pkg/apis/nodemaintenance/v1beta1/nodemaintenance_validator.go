package v1beta1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	ErrorNodeNotExists           = "invalid nodeName, no node with name %s found"
	ErrorNodeMaintenanceExists   = "invalid nodeName, a NodeMaintenance for node %s already exists"
	ErrorNodeNameUpdateForbidden = "updating spec.NodeName isn't allowed"
	ErrorMasterQuorumViolation   = "can not put master node into maintenance at this moment, it would violate the master quorum"
)

const (
	EtcdQuorumPDBName      = "etcd-quorum-guard"
	EtcdQuorumPDBNamespace = "openshift-etcd"
	LabelNameRoleMaster    = "node-role.kubernetes.io/master"
)

var log = logger.Log.WithName("validator")

// introduce a NodeMaintenanceValidator, which gets a k8s client injected
// +k8s:deepcopy-gen=false
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
		log.Info("validation failed", "error", err)
		return err
	}

	// Validate that no NodeMaintenance for given node exists yet
	if err := v.validateNoNodeMaintenanceExists(nm.Spec.NodeName); err != nil {
		log.Info("validation failed", "error", err)
		return err
	}

	// Validate that NodeMaintenance for master nodes don't violate quorum
	if err := v.validateMasterQuorum(nm.Spec.NodeName); err != nil {
		log.Info("validation failed", "error", err)
		return err
	}

	return nil
}

func (v *NodeMaintenanceValidator) ValidateUpdate(new, old *NodeMaintenance) error {
	// Validate that node name didn't change
	if new.Spec.NodeName != old.Spec.NodeName {
		log.Info("validation failed", "error", ErrorNodeNameUpdateForbidden)
		return fmt.Errorf(ErrorNodeNameUpdateForbidden)
	}
	return nil
}

func (v *NodeMaintenanceValidator) validateNodeExists(nodeName string) error {
	if node, err := getNode(nodeName, v.client); err != nil {
		return fmt.Errorf("could not get node for validating spec.NodeName, please try again: %v", err)
	} else if node == nil {
		return fmt.Errorf(ErrorNodeNotExists, nodeName)
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
			return fmt.Errorf(ErrorNodeMaintenanceExists, nodeName)
		}
	}
	return nil
}

func (v *NodeMaintenanceValidator) validateMasterQuorum(nodeName string) error {
	// check if the node is a master node
	if node, err := getNode(nodeName, v.client); err != nil {
		return fmt.Errorf("could not get node for master quorum validation, please try again: %v", err)
	} else if node == nil {
		// this should have been catched already, but just in case
		return fmt.Errorf(ErrorNodeNotExists, nodeName)
	} else if !isMasterNode(node) {
		// not a master node, nothing to do
		return nil
	}

	// check the etcd-quorum-guard PodDisruptionBudget if we can drain a master node
	var pdb v1beta1.PodDisruptionBudget
	key := types.NamespacedName{
		Namespace: EtcdQuorumPDBNamespace,
		Name:      EtcdQuorumPDBName,
	}
	if err := v.client.Get(context.TODO(), key, &pdb); err != nil {
		if apierrors.IsNotFound(err) {
			// TODO do we need a fallback for k8s clusters?
			log.Info("etcd-quorum-guard PDB not found. Skipping master quorum validation.")
			return nil
		}
		return fmt.Errorf("could not get etcd-quorum-guard PDB for master quorum validation, please try again: %v", err)
	}
	if pdb.Status.DisruptionsAllowed == 0 {
		return fmt.Errorf(ErrorMasterQuorumViolation)
	}
	return nil
}

// if the returned node is nil, it wasn't found
func getNode(nodeName string, client client.Client) (*v1.Node, error) {
	var node v1.Node
	key := types.NamespacedName{
		Name: nodeName,
	}
	if err := client.Get(context.TODO(), key, &node); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not get node: %v", err)
	}
	return &node, nil
}

func isMasterNode(node *v1.Node) bool {
	if _, ok := node.Labels[LabelNameRoleMaster]; ok {
		return true
	}
	return false
}
