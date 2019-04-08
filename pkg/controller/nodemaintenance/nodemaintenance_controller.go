package nodemaintenance

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/kubectl/drain"

	kubevirtv1alpha1 "kubevirt.io/node-maintenance-operator/pkg/apis/kubevirt/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_nodemaintenance")
var taintRetries = 3

// Add creates a new NodeMaintenance Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	r := &ReconcileNodeMaintenance{client: mgr.GetClient(), scheme: mgr.GetScheme()}
	err := initDrainer(r, mgr.GetConfig())
	return r, err
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodemaintenance-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to NodeMaintenance CR
			return false
		},
	}

	// Create a source for watching noe maintenance events.
	src := &source.Kind{Type: &kubevirtv1alpha1.NodeMaintenance{}}

	// Watch for changes to primary resource NodeMaintenance
	err = c.Watch(src, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}
	return nil
}

func initDrainer(r *ReconcileNodeMaintenance, config *rest.Config) error {

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
	r.drainer.Timeout = time.Minute

	// TODO - consider pod selectors (only for VMIs + others ?)
	//Label selector to filter pods on the node
	//r.drainer.PodSelector = "kubevirt.io=virt-launcher"

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	r.drainer.Client = cs
	r.drainer.DryRun = false

	return nil
}

var _ reconcile.Reconciler = &ReconcileNodeMaintenance{}

// ReconcileNodeMaintenance reconciles a NodeMaintenance object
type ReconcileNodeMaintenance struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client      client.Client
	scheme      *runtime.Scheme
	drainer     *drain.Helper
	podInformer cache.SharedInformer
}

// Reconcile reads that state of the cluster for a NodeMaintenance object and makes changes based on the state read
// and what is in the NodeMaintenance.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeMaintenance) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling NodeMaintenance")
	maintanenceMode := true

	// Fetch the NodeMaintenance instance
	instance := &kubevirtv1alpha1.NodeMaintenance{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info(fmt.Sprintf("NodeMaintenance Object: %s Deleted ", request.NamespacedName))
		} else {
			// Error reading the object - requeue the request.
			reqLogger.Info("Error reading the request object, requeuing.")
			return reconcile.Result{}, err
		}
	}

	//Add finalizer when object is created
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer)
			if err := r.client.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer) {
			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = removeString(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
			maintanenceMode = false
		} else {
			return reconcile.Result{}, nil
		}
	}

	nodeName := instance.Spec.NodeName

	if maintanenceMode {
		reqLogger.Info(fmt.Sprintf("Applying Maintenance mode on Node: %s with Reason: %s", nodeName, instance.Spec.Reason))
	}
	node, err := r.fetchNode(nodeName)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := runCordonOrUncordon(r, node, maintanenceMode); err != nil {
		return reconcile.Result{}, err
	}

	if maintanenceMode {

		taintRetries = 3
		err = r.taintNodeForLiveMigration(nodeName, true)
		if err != nil {
			return reconcile.Result{}, err
		}

		stop := make(chan struct{})
		defer close(stop)

		reqLogger.Info(fmt.Sprintf("Evict all Pods from Node: %s", nodeName))
		if err = drainPods(r, node, stop); err != nil {
			return reconcile.Result{}, err
		}

	} else {
		taintRetries = 3
		err = r.taintNodeForLiveMigration(nodeName, false)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNodeMaintenance) fetchNode(nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node)
	if err != nil && errors.IsNotFound(err) {
		log.Error(err, fmt.Sprintf("Node: %s cannot be found", nodeName))
		return nil, err
	} else if err != nil {
		log.Error(err, fmt.Sprintf("Failed to get Node %s: %v\n", nodeName, err))
		return nil, err
	}
	return node, nil
}

func (r *ReconcileNodeMaintenance) startPodInformer(node *corev1.Node, stop <-chan struct{}) error {
	fieldSelector := fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Name})

	lw := cache.NewListWatchFromClient(
		r.drainer.Client.CoreV1().RESTClient(),
		"pods",
		corev1.NamespaceAll,
		fieldSelector)

	r.podInformer = cache.NewSharedInformer(lw, &corev1.Pod{}, 30*time.Minute)

	go r.podInformer.Run(stop)
	if !cache.WaitForCacheSync(stop, r.podInformer.HasSynced) {
		return fmt.Errorf("Timed out waiting for caches to sync")
	}

	return nil
}

func (r *ReconcileNodeMaintenance) taintNodeForLiveMigration(nodeName string, taintNode bool) error {

	drainNode, err := r.fetchNode(nodeName)
	if err != nil {
		return err
	}
	taintStr := ""
	updated := false
	if taintNode {
		taintStr = "add"
		updated, err = addTaint(r.client, drainNode)
	} else {
		taintStr = "remove"
		updated, err = removeTaint(r.client, drainNode)
	}

	if !updated {
		log.Error(err, fmt.Sprintf("kubevirt.io/drain taint %s was not applied on Node: %s", taintStr, nodeName))
		if taintRetries > 0 {
			log.Info(fmt.Sprintf("Retry taint %s on node: %s . %d retries left.", taintStr, nodeName, taintRetries))
			taintRetries--
			time.Sleep(20 * time.Second)
			return r.taintNodeForLiveMigration(nodeName, taintNode)
		} else if err != nil {
			return err
		}
	}
	return nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
