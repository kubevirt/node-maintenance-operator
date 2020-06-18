package nodemaintenance

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/api/errors"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	nodemaintenanceapi "kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kubevirt.io/node-maintenance-operator/pkg/drain"
)

const (
	MaxAllowedErrorToUpdateOwnedLease = 3
	drainerTimeoutInMinutes = 2
	LeaseDurationInSeconds = 3600
	LeaseHolderIdentity    = "node-maintenance"
	LeaseNamespaceDefault  = "node-maintenance-operator"
	LeaseApiPackage        = "coordination.k8s.io/v1beta1"
)

var LeaseNamespace = LeaseNamespaceDefault

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
	r.drainer.Timeout = drainerTimeoutInMinutes * time.Minute

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

// ReconcileNodeMaintenance reconciles a NodeMaintenance object
type ReconcileNodeMaintenance struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client      client.Client
	scheme      *runtime.Scheme
	drainer     *drain.Helper
	isLeaseSupported bool
}

func (r *ReconcileNodeMaintenance) checkLeaseSupported() (error) {
	isLeaseSupported, err := checkLeaseSupportedInternal(r.drainer.Client);
	if err != nil {
		log.Errorf("Failed to check for lease support %v", err)
		return err
	}
	r.isLeaseSupported = isLeaseSupported
	return nil
}

// Reconcile reads that state of the cluster for a NodeMaintenance object and makes changes based on the state read
// and what is in the NodeMaintenance.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeMaintenance) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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

	// Add finalizer when object is created
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !ContainsString(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer)
			if err := r.client.Update(context.TODO(), instance); err != nil {
				return r.reconcileAndError(instance, err)
			}
		}
	} else {
		reqLogger.Infof("Deletion timestamp not zero")

		// The object is being deleted
		if ContainsString(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer) {
			// Stop node maintenance - uncordon and remove live migration taint from the node.
			if err := r.stopNodeMaintenance(instance.Spec.NodeName); err != nil {
				reqLogger.Infof("error stopping node maintenance: %v", err)
				if errors.IsNotFound(err) == false {
					return r.reconcileAndError(instance, err)
				}
			}

			// Remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = RemoveString(instance.ObjectMeta.Finalizers, nodemaintenanceapi.NodeMaintenanceFinalizer)
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

	updateOwnedLeaseFailed, err := r.obtainLease(node)
	if err != nil && updateOwnedLeaseFailed {
		instance.Status.ErrorOnLeaseCount += 1
		if instance.Status.ErrorOnLeaseCount > MaxAllowedErrorToUpdateOwnedLease {
			log.Info("can't extend owned lease. uncordon for now")

			// Uncordon the node
			err = r.stopNodeMaintenanceImp(node)
			if err != nil {
				return r.reconcileAndError(instance,fmt.Errorf("Failed to uncordon upon failure to obtain owned lease : %v ", err))
			}
			instance.Status.Phase = nodemaintenanceapi.MaintenanceFailed
		}
		return r.reconcileAndError(instance,fmt.Errorf("Failed to extend lease owned by us : %v errorOnLeaseCount %d", err, instance.Status.ErrorOnLeaseCount))
	}
	if err != nil {
		instance.Status.ErrorOnLeaseCount = 0
		return r.reconcileAndError(instance, err)
	}  else {
		if instance.Status.Phase != nodemaintenanceapi.MaintenanceRunning || instance.Status.ErrorOnLeaseCount != 0 {
			instance.Status.Phase = nodemaintenanceapi.MaintenanceRunning
			instance.Status.ErrorOnLeaseCount = 0
		}
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

	instance.Status.Phase = nodemaintenanceapi.MaintenanceSucceeded
	instance.Status.PendingPods = nil
	err = r.client.Status().Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Errorf("Failed to update NodeMaintenance with \"Succeeded\" status. Error: %v", err)
		return r.reconcileAndError(instance, err)
	}
	reqLogger.Infof("Reconcile completed for Node: %s", nodeName)


	return reconcile.Result{}, nil
}

func (r *ReconcileNodeMaintenance) obtainLease(node *corev1.Node) (bool, error) {
	if !r.isLeaseSupported {
		return false, nil
	}

	log.Info("Lease object supported, obtaining lease")
	lease, needUpdate, err := createOrGetExistingLease(r.client, node, LeaseDurationInSeconds)

	if err != nil {
		log.Errorf("failed to create or get existing lease error=%v", err)
		return false, err
	}

	if needUpdate {

		log.Info("update lease")

		if _, err, updateOwnedLeaseFailed := updateLease(r.client, node, lease, time.Now(), LeaseDurationInSeconds); err != nil {
				return updateOwnedLeaseFailed, err
		}
	}

	return false, nil
}
func (r *ReconcileNodeMaintenance) stopNodeMaintenanceImp(node *corev1.Node ) error {
	// Uncordon the node
	err := AddOrRemoveTaint(r.drainer.Client, node, false)
	if err != nil {
		return err
	}

	if err = drain.RunCordonOrUncordon(r.drainer, node, false); err != nil {
		return err
	}

	if r.isLeaseSupported {
		if err := invalidateLease(r.client, node.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileNodeMaintenance) stopNodeMaintenance(nodeName string) error {
	node, err := r.fetchNode(nodeName)
	if err != nil {
		return err
	}
	return r.stopNodeMaintenanceImp(node)
}

func (r *ReconcileNodeMaintenance) fetchNode(nodeName string) (*corev1.Node, error) {
	node, err := r.drainer.Client.CoreV1().Nodes().Get( nodeName, metav1.GetOptions{})
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

func (r *ReconcileNodeMaintenance) reconcileAndError(nm *nodemaintenanceapi.NodeMaintenance, err error) (reconcile.Result, error) {
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
