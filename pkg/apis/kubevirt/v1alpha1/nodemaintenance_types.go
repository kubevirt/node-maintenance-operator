package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
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
	// StartTime contains the Date and time at which the maintenance CR processing has started
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Completed represents the state of maintenance. True if completed , false otherwise
	Completed bool `json:"completed,omitempty"`
	// Failed is true if the controller did not complete maintenance during the latest reconciliation,
	// false otherwise
	Failed bool `json:"failed,omitempty"`
	// ErrorMessage represents the latest reason for failed=true
	ErrorMessage string `json:"errorMessage,omitempty"`
	// PendingPods are pods that failed to be evicted in the latest reconciliation
	PendingPods []corev1.Pod `json:"pendingPods,omitempty"`
}

func init() {
	SchemeBuilder.Register(&NodeMaintenance{}, &NodeMaintenanceList{})
}
