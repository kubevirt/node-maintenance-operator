// +build !test

package nodemaintenance

import (
	nodemaintenanceapi "kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new NodeMaintenance Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}

	reconciler := r.(*ReconcileNodeMaintenance)
	leaseChannel, err := add(mgr, r)
	reconciler.leaseChannel = leaseChannel

	return err
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	r := &ReconcileNodeMaintenance{client: mgr.GetClient(), scheme: mgr.GetScheme()}

	err := initDrainer(r, mgr.GetConfig())
	if err == nil {
		err = r.checkLeaseSupported()
	}
	return r, err
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (chan<- event.GenericEvent, error) {
	// Create a new controller
	c, err := controller.New("nodemaintenance-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newObj := e.ObjectNew.(*nodemaintenanceapi.NodeMaintenance)
			return !newObj.DeletionTimestamp.IsZero()
		},
	}

	// Create a source for watching noe maintenance events.
	src := &source.Kind{Type: &nodemaintenanceapi.NodeMaintenance{}}

	// Watch for changes to primary resource NodeMaintenance
	err = c.Watch(src, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return nil, err
	}

	leaseChannel := make(chan event.GenericEvent)
	channelSource := &source.Channel{
		Source: leaseChannel,
	}

	err = c.Watch(channelSource, &handler.EnqueueRequestForObject{})
	if err != nil {
		return nil, err
	}

	return leaseChannel, nil
}
