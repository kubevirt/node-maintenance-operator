/*


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
	"fmt"
	"time"
	"github.com/go-logr/logr"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/drain"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	nodemaintenancev1beta1 "kubevirt.io/node-maintenance-operator/api/v1beta1"
)

const (
	MaxAllowedErrorToUpdateOwnedLease = 3
	DrainerTimeout                    = 30 * time.Second
	WaitDurationOnDrainError          = 5 * time.Second
	FixedDurationReconcileLog         = "Reconciling with fixed duration"
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

func initDrainer(r *NodeMaintenanceReconciler, config *rest.Config) error {

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

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	r.drainer.Client = cs
	r.drainer.DryRunStrategy = util.DryRunNone

	r.drainer.Out = writer{klog.Info}
	r.drainer.ErrOut = writer{klog.Error}
	r.drainer.OnPodDeletedOrEvicted = onPodDeletedOrEvicted
	return nil
}

// NodeMaintenanceReconciler reconciles a NodeMaintenance object
type NodeMaintenanceReconciler struct {
	client.Client
	Log              logr.Logger
	Scheme           *runtime.Scheme
	drainer          *drain.Helper
	isLeaseSupported bool
}

func (r *NodeMaintenanceReconciler) checkLeaseSupported() error {
	isLeaseSupported, err := checkLeaseSupportedInternal(r.drainer.Client)
	if err != nil {
		log.Errorf("Failed to check for lease support %v", err)
		return err
	}
	r.isLeaseSupported = isLeaseSupported
	return nil
}

// +kubebuilder:rbac:groups=nodemaintenance.kubevirt.io,resources=nodemaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nodemaintenance.kubevirt.io,resources=nodemaintenances/status,verbs=get;update;patch
//todo nir probably need to add rbac for leases
func (r *NodeMaintenanceReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithFields(log.Fields{"Request.Namespace": request.Namespace, "Request.Name": request.Name})
	reqLogger.Info("Reconciling NodeMaintenance")

	// Fetch the NodeMaintenance instance
	instance := &nodemaintenancev1beta1.NodeMaintenance{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Infof("NodeMaintenance Object: %s Deleted ", request.NamespacedName)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Info("Error reading the request object, requeuing.")
		return ctrl.Result{}, err
	}

	// Add finalizer when object is created
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !ContainsString(instance.ObjectMeta.Finalizers, nodemaintenancev1beta1.NodeMaintenanceFinalizer) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, nodemaintenancev1beta1.NodeMaintenanceFinalizer)
			if err := r.Client.Update(context.TODO(), instance); err != nil {
				return r.onReconcileError(instance, err)
			}
		}
	} else {
		reqLogger.Infof("Deletion timestamp not zero")

		// The object is being deleted
		if ContainsString(instance.ObjectMeta.Finalizers, nodemaintenancev1beta1.NodeMaintenanceFinalizer) || ContainsString(instance.ObjectMeta.Finalizers, metav1.FinalizerOrphanDependents) {
			// Stop node maintenance - uncordon and remove live migration taint from the node.
			if err := r.stopNodeMaintenanceOnDeletion(instance.Spec.NodeName); err != nil {
				reqLogger.Infof("error stopping node maintenance: %v", err)
				if errors.IsNotFound(err) == false {
					return r.onReconcileError(instance, err)
				}
			}

			// Remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = RemoveString(instance.ObjectMeta.Finalizers, nodemaintenancev1beta1.NodeMaintenanceFinalizer)
			if err := r.Client.Update(context.Background(), instance); err != nil {
				return r.onReconcileError(instance, err)
			}
		}
		return ctrl.Result{}, nil
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

	updateOwnedLeaseFailed, err := r.obtainLease(node)
	if err != nil && updateOwnedLeaseFailed {
		instance.Status.ErrorOnLeaseCount += 1
		if instance.Status.ErrorOnLeaseCount > MaxAllowedErrorToUpdateOwnedLease {
			log.Info("can't extend owned lease. uncordon for now")

			// Uncordon the node
			err = r.stopNodeMaintenanceImp(node)
			if err != nil {
				return r.onReconcileError(instance, fmt.Errorf("Failed to uncordon upon failure to obtain owned lease : %v ", err))
			}
			instance.Status.Phase = nodemaintenancev1beta1.MaintenanceFailed
		}
		return r.onReconcileError(instance, fmt.Errorf("Failed to extend lease owned by us : %v errorOnLeaseCount %d", err, instance.Status.ErrorOnLeaseCount))
	}
	if err != nil {
		instance.Status.ErrorOnLeaseCount = 0
		return r.onReconcileError(instance, err)
	} else {
		if instance.Status.Phase != nodemaintenancev1beta1.MaintenanceRunning || instance.Status.ErrorOnLeaseCount != 0 {
			instance.Status.Phase = nodemaintenancev1beta1.MaintenanceRunning
			instance.Status.ErrorOnLeaseCount = 0
		}
	}

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

	instance.Status.Phase = nodemaintenancev1beta1.MaintenanceSucceeded
	instance.Status.PendingPods = nil
	err = r.Client.Status().Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Errorf("Failed to update NodeMaintenance with \"Succeeded\" status. Error: %v", err)
		return r.onReconcileError(instance, err)
	}
	reqLogger.Infof("Reconcile completed for Node: %s", nodeName)

	return ctrl.Result{}, nil
}

func makeBoolRef(val bool) *bool {
	return &val
}

func setOwnerRefToNode(instance *nodemaintenancev1beta1.NodeMaintenance, node *corev1.Node) {

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

func (r *NodeMaintenanceReconciler) obtainLease(node *corev1.Node) (bool, error) {
	if !r.isLeaseSupported {
		return false, nil
	}

	log.Info("Lease object supported, obtaining lease")
	lease, needUpdate, err := createOrGetExistingLease(r.Client, node, LeaseDuration)

	if err != nil {
		log.Errorf("failed to create or get existing lease error=%v", err)
		return false, err
	}

	if needUpdate {

		log.Info("update lease")

		now := metav1.NowMicro()
		if err, updateOwnedLeaseFailed := updateLease(r.Client, node, lease, &now, LeaseDuration); err != nil {
			return updateOwnedLeaseFailed, err
		}
	}

	return false, nil
}
func (r *NodeMaintenanceReconciler) stopNodeMaintenanceImp(node *corev1.Node) error {
	// Uncordon the node
	err := AddOrRemoveTaint(r.drainer.Client, node, false)
	if err != nil {
		return err
	}

	if err = drain.RunCordonOrUncordon(r.drainer, node, false); err != nil {
		return err
	}

	if r.isLeaseSupported {
		if err := invalidateLease(r.Client, node.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *NodeMaintenanceReconciler) stopNodeMaintenanceOnDeletion(nodeName string) error {
	node, err := r.fetchNode(nodeName)
	if err != nil {
		// if CR is gathered as result of garbage collection: the node may have been deleted, but the CR has not yet been deleted, still we must clean up the lease!
		if errors.IsNotFound(err) {
			if r.isLeaseSupported {
				if err := invalidateLease(r.Client, nodeName); err != nil {
					return err
				}
			}
			return nil
		}
		return err
	}
	return r.stopNodeMaintenanceImp(node)
}

func (r *NodeMaintenanceReconciler) fetchNode(nodeName string) (*corev1.Node, error) {
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

func (r *NodeMaintenanceReconciler) initMaintenanceStatus(nm *nodemaintenancev1beta1.NodeMaintenance) error {
	if nm.Status.Phase == "" {
		nm.Status.Phase = nodemaintenancev1beta1.MaintenanceRunning
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
		err = r.Client.Status().Update(context.TODO(), nm)
		return err
	}
	return nil
}

func (r *NodeMaintenanceReconciler) onReconcileErrorWithRequeue(nm *nodemaintenancev1beta1.NodeMaintenance, err error, duration *time.Duration) (ctrl.Result, error) {
	nm.Status.LastError = err.Error()

	if nm.Spec.NodeName != "" {
		pendingList, _ := r.drainer.GetPodsForDeletion(nm.Spec.NodeName)
		if pendingList != nil {
			nm.Status.PendingPods = GetPodNameList(pendingList.Pods())
		}
	}

	updateErr := r.Client.Status().Update(context.TODO(), nm)
	if updateErr != nil {
		log.Errorf("Failed to update NodeMaintenance with \"Failed\" status. Error: %v", updateErr)
	}
	if duration != nil {
		log.Infof(FixedDurationReconcileLog)
		return ctrl.Result{RequeueAfter: *duration}, nil
	}
	log.Infof("Reconciling with exponential duration")
	return ctrl.Result{}, err
}

func (r *NodeMaintenanceReconciler) onReconcileError(nm *nodemaintenancev1beta1.NodeMaintenance, err error) (ctrl.Result, error) {
	return r.onReconcileErrorWithRequeue(nm, err, nil)

}
func (r *NodeMaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newObj := e.ObjectNew.(*nodemaintenancev1beta1.NodeMaintenance)
			return !newObj.DeletionTimestamp.IsZero()
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&nodemaintenancev1beta1.NodeMaintenance{}).
		WithEventFilter(pred).
		Complete(r)
}
