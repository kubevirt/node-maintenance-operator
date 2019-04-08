package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NodeMaintenanceFinalizer string = "foregroundDeleteNodeMaintenance"
)

// NodeMaintenanceSpec defines the desired state of NodeMaintenance
// +k8s:openapi-gen=true
type NodeMaintenanceSpec struct {
	// Node name to apply maintanance on/off
	NodeName string `json:"nodeName"`
	// Reason for maintanance
	Reason string `json:"reason,omitempty"`
}

// NodeMaintenanceStatus defines the observed state of NodeMaintenance
// +k8s:openapi-gen=true
type NodeMaintenanceStatus struct {
	Phase string `json:"phase,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeMaintenance is the Schema for the nodemaintenances API
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

func init() {
	SchemeBuilder.Register(&NodeMaintenance{}, &NodeMaintenanceList{})
}
