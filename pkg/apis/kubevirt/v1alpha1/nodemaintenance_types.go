package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NodeMaintenanceFinalizer is a finalizer for a NodeMaintenance CR deletion
	NodeMaintenanceFinalizer string = "foregroundDeleteNodeMaintenance"
)

// MaintenancePhase contains the phase of maintenance
type MaintenancePhase string

const (
	// MaintenanceRunning - maintenance has started its proccessing
	MaintenanceRunning MaintenancePhase = "Running"
	// MaintenanceSucceeded - maintenance has finished succesfuly, cordoned the node and evicted all pods (that could be evicted)
	MaintenanceSucceeded MaintenancePhase = "Succeeded"
	// MaintenanceFailed - maintenance has failed the last reconciliation cycle and will retry
	MaintenanceFailed MaintenancePhase = "Failed"
)

// NodeMaintenanceSpec defines the desired state of NodeMaintenance
type NodeMaintenanceSpec struct {
	// Node name to apply maintanance on/off
	NodeName string `json:"nodeName"`
	// Reason for maintanance
	Reason string `json:"reason,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeMaintenance is the Schema for the nodemaintenances API
// kubebuilder:subresource:status
// +k8s:openapi-gen=true
type NodeMaintenance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeMaintenanceSpec   `json:"spec,omitempty"`
	Status NodeMaintenanceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeMaintenanceList contains a list of NodeMaintenance
type NodeMaintenanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeMaintenance `json:"items"`
}

// NodeMaintenanceStatus defines the observed state of NodeMaintenance
type NodeMaintenanceStatus struct {
	// Phase is the represtation of a maintenanace progress (Running,Succeeded,Failed)
	Phase MaintenancePhase `json:"phase,omitempty"`
	// LastError represents the latest reason for failed=true
	LastError string `json:"lastError,omitempty"`
	// PendingPods are pods that failed to be evicted in the latest reconciliation
	PendingPods []corev1.Pod `json:"pendingPods,omitempty"`
}

func init() {
	SchemeBuilder.Register(&NodeMaintenance{}, &NodeMaintenanceList{})
}
