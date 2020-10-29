package nodemaintenance

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/drain"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nodemaintenanceapi "kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
	"kubevirt.io/node-maintenance-operator/pkg/controller/nodemaintenance/lease"
)

const (
	MaxAllowedErrorToUpdateOwnedLease = 3
	DrainerTimeout                    = 30 * time.Second
	WaitDurationOnDrainError          = 5 * time.Second
	FixedDurationReconcileLog         = "Reconciling with fixed duration"
	LeaseNamespaceDefault             = "node-maintenance"
	LeaseHolderIdentity               = "node-maintenance"
	// TODO revert this change, find a better way to e2e test renewals
	LeaseDuration = 15 * time.Second
)

var LeaseNamespace = LeaseNamespaceDefault

var reconcileLocks = make(map[string]*sync.Mutex)

// set readable timestamp format in logs, not reference time
func init() {
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "2006-01-02 15:04:05.000000", FullTimestamp: true})
}

// writer implements io.Writer interface as a pass-through for klog.
type writer struct {
	logFunc func(args ...interface{})
}

// Write passes string(p) into writer's logFunc and always returns len(p)
func (w writer) Write(p []byte) (n int, err error) {
	w.logFunc(string(p))
	return len(p), nil
}

func onPodDeletedOrEvicted(pod *corev1.Pod, usingEviction bool) {
	var verbString string
	if usingEviction {
		verbString = "Evicted"
	} else {
		verbString = "Deleted"
	}
	msg := fmt.Sprintf("pod: %s:%s %s from node: %s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, verbString, pod.Spec.NodeName)
	klog.Info(msg)
}

func SetLeaseNamespace(namespace string) {
	LeaseNamespace = namespace
}

func initDrainer(r *ReconcileNodeMaintenance, clientset kubernetes.Interface) error {

	r.drainer = &drain.Helper{}

	//Continue even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet.
	//This is required because VirtualMachineInstance pods are not owned by a ReplicaSet or DaemonSet controller.
	//This means that the drain operation can’t guarantee that the pods being terminated on the target node will get
	//re-scheduled replacements placed else where in the cluster after the pods are evicted.
	//KubeVirt has its own controllers which manage the underlying VirtualMachineInstance pods.
	//Each controller behaves differently to a VirtualMachineInstance being evicted.
	r.drainer.Force = true

	//Continue even if there are pods using emptyDir (local data that will be deleted when the node is drained).
	//This is necessary for removing any pod that utilizes an emptyDir volume.
	//The VirtualMachineInstance Pod does use emptryDir volumes,
	//however the data in those volumes are ephemeral which means it is safe to delete after termination.
	r.drainer.DeleteLocalData = true

	//Ignore DaemonSet-managed pods.
	//This is required because every node running a VirtualMachineInstance will also be running our helper DaemonSet called virt-handler.
	//This flag indicates that it is safe to proceed with the eviction and to just ignore DaemonSets.
	r.drainer.IgnoreAllDaemonSets = true

	//Period of time in seconds given to each pod to terminate gracefully. If negative, the default value specified in the pod will be used.
	r.drainer.GracePeriodSeconds = -1

	// TODO - add logical value or attach from the maintancene CR
	//The length of time to wait before giving up, zero means infinite
	r.drainer.Timeout = DrainerTimeout

	// TODO - consider pod selectors (only for VMIs + others ?)
	//Label selector to filter pods on the node
	//r.drainer.PodSelector = "kubevirt.io=virt-launcher"

	r.drainer.Client = clientset
	r.drainer.DryRunStrategy = util.DryRunNone

	r.drainer.Out = writer{klog.Info}
	r.drainer.ErrOut = writer{klog.Error}
	r.drainer.OnPodDeletedOrEvicted = onPodDeletedOrEvicted
	return nil
}

var _ reconcile.Reconciler = &ReconcileNodeMaintenance{}

// ReconcileNodeMaintenance reconciles a NodeMaintenance object
type ReconcileNodeMaintenance struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client          client.Client
	scheme          *runtime.Scheme
	clientset       kubernetes.Interface
	drainer         *drain.Helper
	leaseCallbackCh chan event.GenericEvent
}

// Reconcile reads that state of the cluster for a NodeMaintenance object and makes changes based on the state read
// and what is in the NodeMaintenance.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeMaintenance) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	// prevent reconciling the same node concurrently
	// can happen when a lease expired and triggers a reconcile by channel source
	var lock sync.Mutex
	if lock, ok := reconcileLocks[request.Name]; !ok {
		lock = &sync.Mutex{}
		reconcileLocks[request.Name] = lock
	}
	lock.Lock()
	defer lock.Unlock()

	reqLogger := log.WithFields(log.Fields{"Request.Namespace": request.Namespace, "Request.Name": request.Name})
	reqLogger.Info("Reconciling NodeMaintenance")

	// Fetch the NodeMaintenance instance
	instance := &nodemaintenanceapi.NodeMaintenance{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Infof("NodeMaintenance Object: %s Deleted ", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Info("Error reading the request object, requeuing.")
		return reconcile.Result{}, err
	}

	leaseHandler, err := lease.New(LeaseNamespace, instance.Spec.NodeName, LeaseHolderIdentity, r.clientset, LeaseDuration)
	if err != nil {
		reqLogger.Errorf("Failed to setup lease handler: %v", err)
		return reconcile.Result{}, err
	}

	// Add finalizer when object is created
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !ContainsString(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer)
			if err := r.client.Update(context.TODO(), instance); err != nil {
				return r.onReconcileError(instance, err)
			}
		}
	} else {
		reqLogger.Infof("Deletion timestamp not zero")

		// The object is being deleted
		if ContainsString(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer) || ContainsString(instance.ObjectMeta.Finalizers, metav1.FinalizerOrphanDependents) {
			// Stop node maintenance - uncordon and remove live migration taint from the node.
			if err := r.stopNodeMaintenanceOnDeletion(instance.Spec.NodeName, leaseHandler); err != nil {
				reqLogger.Infof("error stopping node maintenance: %v", err)
				if errors.IsNotFound(err) == false {
					return r.onReconcileError(instance, err)
				}
			}

			// Remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = RemoveString(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				return r.onReconcileError(instance, err)
			}
		}
		return reconcile.Result{}, nil
	}

	err = r.initMaintenanceStatus(instance)
	if err != nil {
		reqLogger.Errorf("Failed to update NodeMaintenance with \"Running\" status. Error: %v", err)
		return r.onReconcileError(instance, err)
	}

	nodeName := instance.Spec.NodeName

	reqLogger.Infof("Applying Maintenance mode on Node: %s with Reason: %s", nodeName, instance.Spec.Reason)
	node, err := r.fetchNode(nodeName)
	if err != nil {
		return r.onReconcileError(instance, err)
	}

	setOwnerRefToNode(instance, node)

	// try to get a node lease
	// use a callback to get notified when we lose the lease and trigger a new reconcile
	leaseCallback := func(reason lease.CallbackReason) {
		switch reason {
		case lease.Lost:
			r.leaseCallbackCh <- event.GenericEvent{
				Meta: instance.GetObjectMeta(),
			}
		}
	}
	if err := leaseHandler.Get(leaseCallback); err != nil {

		log.Errorf("failed to get node lease! status: %v, error: %v", leaseHandler.Status, err)

		// heads up:
		// comparing to old impl, we can't count renewal failures anymore, but I think that's ok
		// we can only react on the fact that maintenance is running and then losing the lease
		if leaseHandler.Status == lease.ForeignOwner &&
			(instance.Status.Phase == nodemaintenanceapi.MaintenanceRunning || instance.Status.Phase == nodemaintenanceapi.MaintenanceSucceeded) {
			log.Errorf("we lost the lease, stopping maintenance")
			// TODO is it actually ok to modify the node when we don't have the lease anymore?? We might conflict with the new owner...
			err = r.stopNodeMaintenanceImp(node, leaseHandler)
			if err != nil {
				return r.onReconcileError(instance, fmt.Errorf("failed to stop maintenance after losing lease : %v ", err))
			}
			instance.Status.Phase = nodemaintenanceapi.MaintenanceFailed
			return r.onReconcileError(instance, fmt.Errorf("lease lost"))
		}

		// for everything else, just retry later
		return r.onReconcileError(instance, err)
	}

	instance.Status.Phase = nodemaintenanceapi.MaintenanceRunning

	// Cordon node
	err = AddOrRemoveTaint(r.drainer.Client, node, true)
	if err != nil {
		return r.onReconcileError(instance, err)
	}

	if err = drain.RunCordonOrUncordon(r.drainer, node, true); err != nil {
		return r.onReconcileError(instance, err)
	}

	reqLogger.Infof("Evict all Pods from Node: %s", nodeName)

	if err = drain.RunNodeDrain(r.drainer, nodeName); err != nil {
		reqLogger.Infof("not all pods evicted: %s : %v", nodeName, err)
		waitOnReconcile := WaitDurationOnDrainError
		return r.onReconcileErrorWithRequeue(instance, err, &waitOnReconcile)
	}
	reqLogger.Infof("All pods evicted: %s", nodeName)

	instance.Status.Phase = nodemaintenanceapi.MaintenanceSucceeded
	instance.Status.PendingPods = nil
	err = r.client.Status().Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Errorf("Failed to update NodeMaintenance with \"Succeeded\" status. Error: %v", err)
		return r.onReconcileError(instance, err)
	}
	reqLogger.Infof("Reconcile completed for Node: %s", nodeName)

	return reconcile.Result{}, nil
}

func makeBoolRef(val bool) *bool {
	return &val
}

func setOwnerRefToNode(instance *nodemaintenanceapi.NodeMaintenance, node *corev1.Node) {

	for _, ref := range instance.ObjectMeta.GetOwnerReferences() {
		if ref.APIVersion == node.TypeMeta.APIVersion && ref.Kind == node.TypeMeta.Kind && ref.Name == node.ObjectMeta.GetName() && ref.UID == node.ObjectMeta.GetUID() {
			return
		}
	}

	log.Info("setting owner ref to node")

	nodeMeta := node.TypeMeta
	ref := metav1.OwnerReference{
		APIVersion:         nodeMeta.APIVersion,
		Kind:               nodeMeta.Kind,
		Name:               node.ObjectMeta.GetName(),
		UID:                node.ObjectMeta.GetUID(),
		BlockOwnerDeletion: makeBoolRef(false),
		Controller:         makeBoolRef(false),
	}

	instance.ObjectMeta.SetOwnerReferences(append(instance.ObjectMeta.GetOwnerReferences(), ref))
}

func (r *ReconcileNodeMaintenance) stopNodeMaintenanceImp(node *corev1.Node, leaseHandler *lease.Handler) error {
	// Uncordon the node
	err := AddOrRemoveTaint(r.drainer.Client, node, false)
	if err != nil {
		return err
	}

	if err = drain.RunCordonOrUncordon(r.drainer, node, false); err != nil {
		return err
	}

	if err := leaseHandler.Release(); err != nil {
		return err
	}
	return nil
}

func (r *ReconcileNodeMaintenance) stopNodeMaintenanceOnDeletion(nodeName string, leaseHandler *lease.Handler) error {
	node, err := r.fetchNode(nodeName)
	if err != nil {
		// if CR is gathered as result of garbage collection: the node may have been deleted, but the CR has not yet been deleted, still we must clean up the lease!
		if errors.IsNotFound(err) {
			if err := leaseHandler.Release(); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	return r.stopNodeMaintenanceImp(node, leaseHandler)
}

func (r *ReconcileNodeMaintenance) fetchNode(nodeName string) (*corev1.Node, error) {
	node, err := r.drainer.Client.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		log.Errorf("Node: %s cannot be found. Error: %v", nodeName, err)
		return nil, err
	} else if err != nil {
		log.Errorf("Failed to get Node %s: %v\n", nodeName, err)
		return nil, err
	}
	return node, nil
}

func (r *ReconcileNodeMaintenance) initMaintenanceStatus(nm *nodemaintenanceapi.NodeMaintenance) error {
	if nm.Status.Phase == "" {
		nm.Status.Phase = nodemaintenanceapi.MaintenanceRunning
		pendingList, errlist := r.drainer.GetPodsForDeletion(nm.Spec.NodeName)
		if errlist != nil {
			return fmt.Errorf("Failed to get pods for eviction while initializing status")
		}
		if pendingList != nil {
			nm.Status.PendingPods = GetPodNameList(pendingList.Pods())
		}
		nm.Status.EvictionPods = len(nm.Status.PendingPods)

		podlist, err := r.drainer.Client.CoreV1().Pods(metav1.NamespaceAll).List(
			context.Background(),
			metav1.ListOptions{
				FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nm.Spec.NodeName}).String(),
			})
		if err != nil {
			return err
		}
		nm.Status.TotalPods = len(podlist.Items)
		err = r.client.Status().Update(context.TODO(), nm)
		return err
	}
	return nil
}

func (r *ReconcileNodeMaintenance) onReconcileErrorWithRequeue(nm *nodemaintenanceapi.NodeMaintenance, err error, duration *time.Duration) (reconcile.Result, error) {
	nm.Status.LastError = err.Error()

	if nm.Spec.NodeName != "" {
		pendingList, _ := r.drainer.GetPodsForDeletion(nm.Spec.NodeName)
		if pendingList != nil {
			nm.Status.PendingPods = GetPodNameList(pendingList.Pods())
		}
	}

	updateErr := r.client.Status().Update(context.TODO(), nm)
	if updateErr != nil {
		log.Errorf("Failed to update NodeMaintenance with \"Failed\" status. Error: %v", updateErr)
	}
	if duration != nil {
		log.Infof(FixedDurationReconcileLog)
		return reconcile.Result{RequeueAfter: *duration}, nil
	}
	log.Infof("Reconciling with exponential duration")
	return reconcile.Result{}, err
}

func (r *ReconcileNodeMaintenance) onReconcileError(nm *nodemaintenanceapi.NodeMaintenance, err error) (reconcile.Result, error) {
	return r.onReconcileErrorWithRequeue(nm, err, nil)

}
