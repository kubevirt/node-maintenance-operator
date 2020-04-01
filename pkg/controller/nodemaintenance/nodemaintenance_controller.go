//go:generate mockgen -source $GOFILE -package=$GOPACKAGE -destination=generated_mock_$GOFILE

package nodemaintenance

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/drain"
	kubevirtv1alpha1 "kubevirt.io/node-maintenance-operator/pkg/apis/kubevirt/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	r := &ReconcileNodeMaintenance{client: mgr.GetClient(), scheme: mgr.GetScheme()}
	err := initDrainer(r, mgr.GetConfig())
	Handler = r
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
			newObj := e.ObjectNew.(*kubevirtv1alpha1.NodeMaintenance)
			return !newObj.DeletionTimestamp.IsZero()
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

	r.drainer.Out = writer{klog.Info}
	r.drainer.ErrOut = writer{klog.Error}
	r.drainer.OnPodDeletedOrEvicted = onPodDeletedOrEvicted
	return nil
}

var _ reconcile.Reconciler = &ReconcileNodeMaintenance{}

type ReconcileHandler interface {
	StartPodInformer(node *corev1.Node, stop <-chan struct{}) error
}

// ReconcileNodeMaintenance reconciles a NodeMaintenance object
type ReconcileNodeMaintenance struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client      client.Client
	scheme      *runtime.Scheme
	drainer     *drain.Helper
	podInformer cache.SharedInformer
}

var Handler ReconcileHandler

// Reconcile reads that state of the cluster for a NodeMaintenance object and makes changes based on the state read
// and what is in the NodeMaintenance.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeMaintenance) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithFields(log.Fields{"Request.Namespace": request.Namespace, "Request.Name": request.Name})
	reqLogger.Info("Reconciling NodeMaintenance")

	// Fetch the NodeMaintenance instance
	instance := &kubevirtv1alpha1.NodeMaintenance{}
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

	// Add finalizer when object is created
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !ContainsString(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer)
			if err := r.client.Update(context.TODO(), instance); err != nil {
				return r.reconcileAndError(instance, err)
			}
		}
	} else {
		// The object is being deleted
		if ContainsString(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer) {
			// Stop node maintenance - uncordon and remove live migration taint from the node.
			if err := r.stopNodeMaintenance(instance.Spec.NodeName); err != nil {
				if errors.IsNotFound(err) == false {
					return r.reconcileAndError(instance, err)
				}
			}
			// Remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = RemoveString(instance.ObjectMeta.Finalizers, kubevirtv1alpha1.NodeMaintenanceFinalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				return r.reconcileAndError(instance, err)
			}
		}
		return reconcile.Result{}, nil
	}

	err = r.initMaintenanceStatus(instance)
	if err != nil {
		reqLogger.Errorf("Failed to update NodeMaintenance with \"Running\" status. Error: %v", err)
		return r.reconcileAndError(instance, err)
	}

	nodeName := instance.Spec.NodeName

	reqLogger.Infof("Applying Maintenance mode on Node: %s with Reason: %s", nodeName, instance.Spec.Reason)
	node, err := r.fetchNode(nodeName)
	if err != nil {
		return r.reconcileAndError(instance, err)
	}

	// Cordon node
	err = AddOrRemoveTaint(r.drainer.Client, node, true)
	if err != nil {
		return r.reconcileAndError(instance, err)
	}

	if err = drain.RunCordonOrUncordon(r.drainer, node, true); err != nil {
		return r.reconcileAndError(instance, err)
	}

	reqLogger.Infof("Evict all Pods from Node: %s", nodeName)

	if err = drain.RunNodeDrain(r.drainer, nodeName); err != nil {
		return r.reconcileAndError(instance, err)
	}

	instance.Status.Phase = kubevirtv1alpha1.MaintenanceSucceeded
	instance.Status.PendingPods = nil
	err = r.client.Status().Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Errorf("Failed to update NodeMaintenance with \"Succeeded\" status. Error: %v", err)
		return r.reconcileAndError(instance, err)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNodeMaintenance) stopNodeMaintenance(nodeName string) error {
	node, err := r.fetchNode(nodeName)
	if err != nil {
		return err
	}

	// Uncordon the node
	err = AddOrRemoveTaint(r.drainer.Client, node, false)
	if err != nil {
		return err
	}

	if err = drain.RunCordonOrUncordon(r.drainer, node, false); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileNodeMaintenance) fetchNode(nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node)
	if err != nil && errors.IsNotFound(err) {
		log.Errorf("Node: %s cannot be found. Error: %v", nodeName, err)
		return nil, err
	} else if err != nil {
		log.Errorf("Failed to get Node %s: %v\n", nodeName, err)
		return nil, err
	}
	return node, nil
}

func (r *ReconcileNodeMaintenance) StartPodInformer(node *corev1.Node, stop <-chan struct{}) error {
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

func (r *ReconcileNodeMaintenance) initMaintenanceStatus(nm *kubevirtv1alpha1.NodeMaintenance) error {
	if nm.Status.Phase == "" {
		nm.Status.Phase = kubevirtv1alpha1.MaintenanceRunning
		pendingList, errlist := r.drainer.GetPodsForDeletion(nm.Spec.NodeName)
		if errlist != nil {
			return fmt.Errorf("Failed to get pods for eviction while initializing status")
		}
		if pendingList != nil {
			nm.Status.PendingPods = GetPodNameList(pendingList.Pods())
		}
		nm.Status.EvictionPods = len(nm.Status.PendingPods)

		podlist, err := r.drainer.Client.CoreV1().Pods(metav1.NamespaceAll).List(metav1.ListOptions{
			FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nm.Spec.NodeName}).String()})
		if err != nil {
			return err
		}
		nm.Status.TotalPods = len(podlist.Items)
		err = r.client.Status().Update(context.TODO(), nm)
		return err
	}
	return nil
}

func (r *ReconcileNodeMaintenance) reconcileAndError(nm *kubevirtv1alpha1.NodeMaintenance, err error) (reconcile.Result, error) {
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
	return reconcile.Result{}, err
}
