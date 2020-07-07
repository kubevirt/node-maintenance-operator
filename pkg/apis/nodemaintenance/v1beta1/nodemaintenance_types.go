package v1beta1

import (
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
	// MaintenanceFailed - maintenance has failed
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
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nodemaintenances,scope=Cluster
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
	// Phase is the represtation of the maintenanace progress (Running,Succeeded,Failed)
	Phase MaintenancePhase `json:"phase,omitempty"`
	// LastError represents the latest error if any in the latest reconciliation
	LastError string `json:"lastError,omitempty"`
	// PendingPods is a list of pending pods for eviction
	PendingPods []string `json:"pendingPods,omitempty"`
	// TotalPods is the total number of all pods on the node from the start
	TotalPods int `json:"totalpods,omitempty"`
	// EvictionPods is the total number of pods up for eviction from the start
	EvictionPods int `json:"evictionPods,omitempty"`
	// Consecutive number of errors upon obtaining a lease
	ErrorOnLeaseCount int `json:"errorOnLeaseCount,omitempty"`
}

func init() {
	SchemeBuilder.Register(&NodeMaintenance{}, &NodeMaintenanceList{})
}
