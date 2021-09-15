/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nodemaintenancev1beta1 "kubevirt.io/node-maintenance-operator/api/v1beta1"
)

const (
	MaxAllowedErrorToUpdateOwnedLease = 3
	DrainerTimeout                    = 30 * time.Second
	WaitDurationOnDrainError          = 5 * time.Second
	FixedDurationReconcileLog         = "Reconciling with fixed duration"
)

// NodeMaintenanceReconciler reconciles a NodeMaintenance object
type NodeMaintenanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nodemaintenance.kubevirt.io,resources=nodemaintenances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nodemaintenance.kubevirt.io,resources=nodemaintenances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nodemaintenance.kubevirt.io,resources=nodemaintenances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeMaintenance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *NodeMaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeMaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nodemaintenancev1beta1.NodeMaintenance{}).
		Complete(r)
}
