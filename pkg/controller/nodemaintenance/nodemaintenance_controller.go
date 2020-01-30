//go:generate mockgen -source $GOFILE -package=$GOPACKAGE -destination=generated_mock_$GOFILE

package nodemaintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	drain "k8s.io/kubectl/pkg/drain"
)

var _ reconcile.Reconciler = &ReconcileNodeMaintenance{}

const (
	NodeMaintenanceSpecAnnotation   = "lifecycle.openshift.io/maintenance"
	NodeMaintenanceStatusAnnotation = "lifecycle.openshift.io/maintenance-status"
	LeaseHolderIdentity             = "node-maintenance"
	LeasePaddingSeconds             = 30
	WaitForLeasePeriod              = 10 * time.Second
)

const (
	MinMaintenenceWindowSeconds = 300              // minimum maintenance window size. (must be bigger than LeasePaddingSeconds)
	EvictionTimeSlice           = 20 * time.Second // max. time in seconds that eviction may block.
	RequeuDrainingWaitTime      = 10 * time.Second // time to reqeueu draining if not completed.
)

type LeaseStatus int

const (
	LeaseStatusNotFound               LeaseStatus = -1 // currently not active lease
	LeaseStatusOwnedByMe              LeaseStatus = 1  // there is a lease active right now
	LeaseStatusOwnedByDifferentHolder LeaseStatus = 3
	LeaseStatusFail                   LeaseStatus = 6
)

type NodeMaintenanceStatusType string

const (
	NodeStateWaiting     NodeMaintenanceStatusType = "waiting"
	NodeStateNew         NodeMaintenanceStatusType = "new"
	NodeStateNewCreate   NodeMaintenanceStatusType = "new,create"
	NodeStateNewAcquired NodeMaintenanceStatusType = "new,acquired"
	NodeStateNewStale    NodeMaintenanceStatusType = "new,stale"
	NodeStateNewRecreate NodeMaintenanceStatusType = "new,recreate"
	NodeStateUpdated     NodeMaintenanceStatusType = "updated"
	NodeStateActive      NodeMaintenanceStatusType = "active"
	NodeStateEnded       NodeMaintenanceStatusType = "ended"

	NodeStateNone NodeMaintenanceStatusType = "" // placeholder
)

type TransitionAction int

const (
	TransitionSet      TransitionAction = 0
	TransitionInc      TransitionAction = 1
	TransitionNoChange TransitionAction = 2
)

// parse this out of NodeMaintenanceSpecAnnotation annotation
type ReconcileNodeMaintenanceSpecInfo int32

// parse this out of NodeMaintenanceStatusAnnotation annotation
type ReconcileNodeMaintenanceStatusInfo string

type ClientFactory interface {
	createClient(config *rest.Config) (kubernetes.Interface, error)
}

type ClientFactoryTest struct {
	client kubernetes.Interface
}

func (r *ClientFactoryTest) createClient(config *rest.Config) (kubernetes.Interface, error) {
	return r.client, nil
}

// ReconcileNodeMaintenance reconciles a node object
type ReconcileNodeMaintenance struct {
	clientFactory ClientFactory
	client        client.Client
	config        *rest.Config

	clientset kubernetes.Interface
	drainer   *drain.Helper

	// used only for the duration of one reconcile call. transient value.
	// That's ok as there is one go routine that handles reconciles (here)
	reqLogger      *log.Entry
	node           *corev1.Node
	specAnnoData   *ReconcileNodeMaintenanceSpecInfo
	statusAnnoData *ReconcileNodeMaintenanceStatusInfo
}

type DeadlineCheck struct {
	isSet    bool
	deadline time.Time
}

func (exp DeadlineCheck) isExpired() bool {
	return exp.isSet && exp.deadline.After(time.Now())
}
func (exp DeadlineCheck) DurationUntilExpiration() time.Duration {
	if exp.isSet {
		exp.deadline.Sub(time.Now())
	}
	return time.Duration(math.MaxInt64)
}

var _ reconcile.Reconciler = &ReconcileNodeMaintenance{}

// Add creates a new NodeMaintenance Controller and adds it to the Manager.
// The Manager will set fields on the Controller and start it when the Manager is started.
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {

	reconciler := &ReconcileNodeMaintenance{
		client: mgr.GetClient(),
		config: mgr.GetConfig(),
	}
	if err := reconciler.initDrainer(); err != nil {
		return nil, fmt.Errorf("failed to init reconciler %v", err)
	}

	return reconciler, nil
}

func (r *ReconcileNodeMaintenance) createClient(config *rest.Config) (kubernetes.Interface, error) {

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	/*
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
	*/

	return cs, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodemaintenance-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	return c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{})
}

// Reconcile monitors Nodes and creates the MachineRemediation object when the node has reboot annotation.
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeMaintenance) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	r.reqLogger = log.WithFields(log.Fields{"Request.Namespace": request.Namespace, "Request.Name": request.Name})
	r.reqLogger.Debug("Reconciling node ", request.Name)

	// Get node from request
	r.node = &corev1.Node{}

	if err := r.client.Get(context.TODO(), request.NamespacedName, r.node); err != nil {
		if errors.IsNotFound(err) {
			r.reqLogger.Info("node not found")
			return reconcile.Result{}, nil
		}
		r.reqLogger.Errorf("node not found. name %s error %v", request.Name, err)
		return reconcile.Result{}, err
	}

	// parse status & spec, create initial status object on first call
	err := r.parseAnnotations()
	if err != nil {
		r.reqLogger.Errorf("request parsing error. error %v", err)
		return reconcile.Result{}, err
	}

	if r.specAnnoData == nil && r.statusAnnoData == nil {
		// nothing to do here.
		return reconcile.Result{}, err
	}

	if r.specAnnoData != nil {
		return r.processMaintModeOn()
	}
	return r.processMaintModeOff()

	/*
		if isTimeoutError(err) {
			r.reqLogger.Infof("timeout error. name %s state %s error %v", request.Name, statusAnnoData.Status, err)
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		} else if err != nil {
			r.reqLogger.Errorf("error. name %s state %s error %v", request.Name, statusAnnoData.Status, err)
		}

		// nothin to do on the node, no relevent annotations have been set
		return reconcile.Result{}, err
	*/
}

func (r *ReconcileNodeMaintenance) parseAnnotations() error {

	r.specAnnoData = nil
	r.statusAnnoData = nil

	if r.node.ObjectMeta.Annotations == nil {
		// no annotations on node, nothing to do here.
		return nil
	}

	val, exists := r.node.ObjectMeta.Annotations[NodeMaintenanceSpecAnnotation]
	if exists {
		nval, err := strconv.ParseInt(val, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid integer value annotation: %s value: %s err: %v", NodeMaintenanceSpecAnnotation, val, err)
		}

		ret := ReconcileNodeMaintenanceSpecInfo(int32(nval))
		r.specAnnoData = &ret

		if ret < MinMaintenenceWindowSeconds {
			err := fmt.Errorf("maintenance window size too small annotation: %s currentValue: %d minValue: %d", NodeMaintenanceSpecAnnotation, ret, MinMaintenenceWindowSeconds)
			return err
		}
		return nil
	}

	val, exists = r.node.ObjectMeta.Annotations[NodeMaintenanceStatusAnnotation]
	if exists {
		ret := ReconcileNodeMaintenanceStatusInfo(val)
		r.statusAnnoData = &ret
	}

	return nil
}

func (r *ReconcileNodeMaintenance) processMaintModeOn() (reconcile.Result, error) {

	// handling of lease as a prerequisite to state transitions.
	leaseStatus, _, lease := r.doesLeaseExist()

	if leaseStatus == LeaseStatusFail || (leaseStatus == LeaseStatusOwnedByDifferentHolder && isLeaseValid(lease) && !isLeaseExpired(lease, false)) {
		r.reqLogger.Debugf("no lease available, wait")
		r.setAnnotations(NodeStateWaiting, false)
		return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
	}

	var err error

	if leaseStatus == LeaseStatusNotFound {
		r.reqLogger.Debugf("create new lease - not found")

		if lease, err = r.createOrUpdateLease(nil, TransitionSet, NodeStateNewCreate); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
	}

	if leaseStatus == LeaseStatusOwnedByDifferentHolder {
		r.reqLogger.Debugf("update lease - invalid/different owner ")

		if _, err = r.createOrUpdateLease(lease, TransitionInc, NodeStateNewAcquired); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

	}

	if !isLeaseValid(lease) || isLeaseDurationZero(lease) {
		r.reqLogger.Debugf("update lease - not valid or zero duration")

		if _, err = r.createOrUpdateLease(lease, TransitionInc, NodeStateNew); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

	}

	if r.isLeaseDurationChanged(lease) {
		r.reqLogger.Debugf("update lease - lease duration change requested")

		if _, err = r.createOrUpdateLease(lease, TransitionNoChange, NodeStateUpdated); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

	}

	if isLeaseExpired(lease, false) {
		r.reqLogger.Debugf("update lease - lease expired")

		if _, err = r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNewStale); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
	}

	if r.statusAnnoData != nil && *r.statusAnnoData == ReconcileNodeMaintenanceStatusInfo(NodeStateEnded) {
		r.reqLogger.Debugf("update lease - lease ended")

		if _, err = r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNewRecreate); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
	}

	dcheck := DeadlineCheck{}

	//if  leaseOverdue {
	if isLeaseExpired(lease, true) {
		r.reqLogger.Debugf("lease deadline + padding is expired. do actions")

		r.setAnnotations(NodeStateEnded, true)

		dcheck = DeadlineCheck{isSet: true, deadline: leaseDeadline(lease)}

		if err := r.runCordonOrUncordon(true, dcheck); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

		if err := r.cancelEviction(dcheck); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
		if !dcheck.isExpired() {
			r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNone) // set lease duration to 0
		}

	} else {
		r.reqLogger.Debugf("lease ok. do actions")

		if err := r.runCordonOrUncordon(true, dcheck); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

		if err := r.evictPods(dcheck); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

		r.setAnnotations(NodeStateActive, false)

	}
	return reconcile.Result{}, nil
}

func (r *ReconcileNodeMaintenance) processMaintModeOff() (reconcile.Result, error) {

	leaseStatus, _, lease := r.doesLeaseExist()
	if leaseStatus == LeaseStatusOwnedByMe {
		dcheck := DeadlineCheck{isSet: true, deadline: leaseDeadline(lease)}

		if !isLeaseExpired(lease, false) {

			r.setAnnotations(NodeStateEnded, false)

			if err := r.runCordonOrUncordon(false, dcheck); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
			}

			if err := r.cancelEviction(dcheck); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
			}

		} else if !isLeaseDurationZero(lease) {

			tmpDurationInSeconds := int32(LeasePaddingSeconds)
			lease.Spec.LeaseDurationSeconds = &tmpDurationInSeconds
			lease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}

			if err := r.client.Update(context.TODO(), lease); err != nil {
				r.reqLogger.Errorf("Failed to update the lease. error: %v", err)
				return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
			}

			if err := r.runCordonOrUncordon(false, dcheck); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
			}

		}
		if !dcheck.isExpired() {
			r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNone) // set lease duration to 0.
		}

	}
	return reconcile.Result{}, nil
}

func (r *ReconcileNodeMaintenance) isLeaseDurationChanged(lease *coordv1beta1.Lease) bool {
	if lease.Spec.LeaseDurationSeconds != nil {
		value := int32(*r.specAnnoData)
		return *lease.Spec.LeaseDurationSeconds != value+LeasePaddingSeconds
	}
	return false
}

func leaseDeadline(lease *coordv1beta1.Lease) time.Time {

	if lease.Spec.AcquireTime != nil {
		leaseDeadline := (*lease.Spec.AcquireTime).Time

		if lease.Spec.LeaseDurationSeconds != nil {
			leaseDeadline.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
		}
		return leaseDeadline
	}
	return time.Now()
}

func isLeaseDurationZero(lease *coordv1beta1.Lease) bool {

	return lease.Spec.LeaseDurationSeconds == nil || *lease.Spec.LeaseDurationSeconds == 0
}

func isLeaseValid(lease *coordv1beta1.Lease) bool {
	return lease.Spec.AcquireTime != nil && lease.Spec.LeaseDurationSeconds != nil
}

func isLeaseExpired(lease *coordv1beta1.Lease, addPaddingToCurrent bool) bool {

	if lease.Spec.AcquireTime == nil {
		// can this happen?
		return false
	}

	leaseDeadline := (*lease.Spec.AcquireTime).Time
	if lease.Spec.LeaseDurationSeconds != nil {
		leaseDeadline = leaseDeadline.Add(time.Duration(int64(*lease.Spec.LeaseDurationSeconds) * int64(time.Second)))
	}

	timeNow := time.Now()

	if addPaddingToCurrent {
		timeNow = timeNow.Add(time.Duration(int64(LeasePaddingSeconds) * int64(time.Second)))
	}
	return timeNow.After(leaseDeadline)
}

func (r *ReconcileNodeMaintenance) doesLeaseExist() (LeaseStatus, error, *coordv1beta1.Lease) {
	lease := &coordv1beta1.Lease{}

	nodeName := r.node.ObjectMeta.Name
	nName := apitypes.NamespacedName{Namespace: corev1.NamespaceNodeLease, Name: nodeName}

	if err := r.client.Get(context.TODO(), nName, lease); err != nil {
		if errors.IsNotFound(err) {
			return LeaseStatusNotFound, nil, nil
		}

		r.reqLogger.Errorf("failed to get lease object. node: %s error: %v", nodeName, err)
		return LeaseStatusFail, err, nil
	}

	r.reqLogger.Debugf("got lease leaseName: %s nodeName %s", lease.ObjectMeta.Name, nodeName)

	if lease.Spec.HolderIdentity != nil && LeaseHolderIdentity == *lease.Spec.HolderIdentity {
		return LeaseStatusOwnedByMe, nil, lease
	}
	return LeaseStatusOwnedByDifferentHolder, nil, lease
}

func (r *ReconcileNodeMaintenance) createOrUpdateLease(lease *coordv1beta1.Lease, transitions TransitionAction, nextState NodeMaintenanceStatusType) (*coordv1beta1.Lease, error) {

	nodeName := r.node.ObjectMeta.Name
	tmpDurationInSeconds := int32(0)
	if nextState != NodeStateNone {
		if r.specAnnoData == nil {
			panic("no spec")
		}
		tmpDurationInSeconds = int32(*r.specAnnoData) + LeasePaddingSeconds
	}

	holderIdentity := LeaseHolderIdentity

	owner := &metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       nodeName,
		UID:        r.node.ObjectMeta.UID,
	}
	if lease == nil {
		leaseTransitions := int32(1)
		microTimeNow := metav1.NewMicroTime(time.Now())

		lease = &coordv1beta1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:            nodeName,
				Namespace:       corev1.NamespaceNodeLease,
				OwnerReferences: []metav1.OwnerReference{*owner},
			},
			Spec: coordv1beta1.LeaseSpec{
				HolderIdentity:       &holderIdentity,
				LeaseDurationSeconds: &tmpDurationInSeconds,
				AcquireTime:          &microTimeNow,
				LeaseTransitions:     &leaseTransitions,
			},
		}

		if err := r.client.Create(context.TODO(), lease); err != nil {
			r.reqLogger.Errorf("Failed to create lease. node %s error: %v", nodeName, err)
			return lease, err
		}
		r.reqLogger.Debugf("lease created. duration: %d sec", tmpDurationInSeconds)
	} else {
		lease.ObjectMeta.Name = nodeName
		lease.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*owner}
		lease.Spec.HolderIdentity = &holderIdentity
		lease.Spec.LeaseDurationSeconds = &tmpDurationInSeconds
		lease.Spec.AcquireTime = &metav1.MicroTime{Time: time.Now()}

		timeNow := metav1.MicroTime{Time: time.Now()}
		lease.Spec.RenewTime = &timeNow

		if transitions == TransitionInc && lease.Spec.LeaseTransitions != nil {
			*lease.Spec.LeaseTransitions += int32(1)
		} else if transitions != TransitionNoChange {
			leaseTransitions := int32(1)
			lease.Spec.LeaseTransitions = &leaseTransitions
		}

		if err := r.client.Update(context.TODO(), lease); err != nil {
			r.reqLogger.Errorf("Failed to update the lease. node %s error: %v", nodeName, err)
			return lease, err
		}
		r.reqLogger.Debugf("lease updated: duration: %d sec", tmpDurationInSeconds)
	}

	if nextState != NodeStateNone {
		r.setAnnotations(nextState, false)
	}
	return lease, nil
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

func (obj *ReconcileNodeMaintenance) initDrainer() error {

	factory := obj.clientFactory
	if factory == nil {
		factory = obj
	}

	cs, err := factory.createClient(obj.config)
	if err != nil {
		return fmt.Errorf("failed to init clientset %v", err)
	}
	obj.clientset = cs

	obj.drainer = &drain.Helper{
		Client:              obj.clientset,
		Force:               true,
		IgnoreAllDaemonSets: true,
		DisableEviction:     false,
		GracePeriodSeconds:  -1,
		// If a pod is not evicted in 20 seconds, retry the eviction next time the
		// machine gets reconciled again (to allow other machines to be reconciled).
		Timeout: EvictionTimeSlice,
		OnPodDeletedOrEvicted: func(pod *corev1.Pod, usingEviction bool) {
			verbStr := "Deleted"
			if usingEviction {
				verbStr = "Evicted"
			}
			klog.Info(fmt.Sprintf("%s pod from Node", verbStr),
				"pod", fmt.Sprintf("%s/%s", pod.Name, pod.Namespace))
		},
		Out:    writer{klog.Info},
		ErrOut: writer{klog.Error},
		DryRun: false,
	}
	return nil
}

func (r *ReconcileNodeMaintenance) runCordonOrUncordon(cordonOn bool, dcheck DeadlineCheck) error {

	countTaint, numDesiredTaints := CountDesiredTaintOnNode(r.node)
        if (cordonOn && countTaint != numDesiredTaints) || (!cordonOn && countTaint != 0) {
                if !dcheck.isExpired() {
                        if err := AddOrRemoveTaint(r.drainer.Client, r.node, cordonOn); err != nil {
                                r.reqLogger.Errorf("failed to set tain (action-set: %t) error: %v", cordonOn, err)
                                return err
                        }
                        r.reqLogger.Debugf("completed add/remove taint set=%t", cordonOn)
                } else {
                        r.reqLogger.Info("skipped remove taint; time expired taintOn=", cordonOn)
                }

        } else {
                r.reqLogger.Debugf("no change of taint required. set=%t", cordonOn)
        }

        if r.node.Spec.Unschedulable != cordonOn {
                if !dcheck.isExpired() {
	
			/*
                        r.node.Spec.Unschedulable = cordonOn
                        if err := r.updateNode(r.node); err != nil {
                                r.reqLogger.Errorf("failed to cordon. action: %t %v", cordonOn, err)
                                return err
                        }
			*/
                        if err := drain.RunCordonOrUncordon(r.drainer, r.node, cordonOn); err != nil {
                                r.reqLogger.Errorf("failed to cordon. action: %t %v", cordonOn, err)
                                return err
                        }
                        r.reqLogger.Debugf("completed cordon/uncordon action-set=%t", cordonOn)
                } else {
                        r.reqLogger.Info("skipped cordon/uncordon; time expired cordonOn=", cordonOn)
                }
        } else {
                r.reqLogger.Debugf("no change of cordon required. set=%t", cordonOn)
        }

	return nil
}

func (r *ReconcileNodeMaintenance) evictPods(dcheck DeadlineCheck) error {

	if !dcheck.isExpired() {
		nodeName := r.node.ObjectMeta.Name
		list, errs := r.drainer.GetPodsForDeletion(nodeName)
		if errs != nil {
			err := utilerrors.NewAggregate(errs)
			r.reqLogger.Errorf("failed got get pod for eviction %v", err)
			return err
		}

		if !dcheck.isExpired() && len(list.Pods()) != 0 {

			r.drainer.Timeout = dcheck.DurationUntilExpiration()

			// indicate to the user that it is evicting pods.
			if err := r.drainer.DeleteOrEvictPods(list.Pods()); err != nil {
				r.reqLogger.Infof("Not all pods evicted %v", err)
				return err
			}
		}
		r.reqLogger.Debugf("completed evicting pods")

	}

	return nil
}
func (r *ReconcileNodeMaintenance) cancelEviction(dcheck DeadlineCheck) error {

	if dcheck.isExpired() {
		return nil
	}

	nodeName := r.node.ObjectMeta.Name
	list, errs := r.drainer.GetPodsForDeletion(nodeName)
	if errs != nil {
		err := utilerrors.NewAggregate(errs)
		r.reqLogger.Errorf("failed got get pod for cancelEviction %v", err)
		return err
	}

	pods := list.Pods()
	if len(pods) != 0 {

		// cancel the move
		for _, pod := range pods {
			if !dcheck.isExpired() {
				err := r.drainer.Client.PolicyV1beta1().Evictions(pod.Namespace).Evict(nil)
				if err != nil {
					r.reqLogger.Errorf("failed cancel eviction %v", err)
					return err
				}
			}
		}
	}
	return nil
}

func (r *ReconcileNodeMaintenance) setAnnotations(statusInfo NodeMaintenanceStatusType, deleteSpec bool) error {

	newNode := r.node.DeepCopy()

	newNode.ObjectMeta.Annotations[NodeMaintenanceStatusAnnotation] = string(statusInfo)
	if deleteSpec {
		delete(newNode.ObjectMeta.Annotations, NodeMaintenanceSpecAnnotation)
	}

	err := r.patchNodes(r.node, newNode, deleteSpec) // for whatever reasons: when deleting an annotation value - patch didn't work, have to update the whole thing.
	if err != nil {
		log.Error("Can't set status annotation of node", err)
	}
	r.node = newNode

	st := ReconcileNodeMaintenanceStatusInfo(string(statusInfo))
	r.statusAnnoData = &st
	if deleteSpec {
		r.specAnnoData = nil
	}

	return err
}

func (r *ReconcileNodeMaintenance) updateNode(node *corev1.Node) error {
    client := r.clientset.Core().Nodes()
    _, err := client.Update(node)
    if err != nil {
            r.reqLogger.Errorf("update (force) failed %v", err)
    }
    return err
}

func (r *ReconcileNodeMaintenance) patchNodes(oldNode *corev1.Node, newNode *corev1.Node, forceUpdate bool) error {

	if forceUpdate {
               return r.updateNode(newNode)
	}

        oldData, err := json.Marshal(oldNode)
        if err != nil {
                r.reqLogger.Errorf("failed to marshal oldNode  %v", err)
                return err
        }
	newData, err := json.Marshal(newNode)
	if err != nil {
		r.reqLogger.Errorf("failed to marshal newNode %v", err)
		return err
	}

	client := r.clientset.Core().Nodes()

	patchBytes, patchErr := strategicpatch.CreateTwoWayMergePatch(oldData, newData, oldNode)
	if patchErr == nil && !forceUpdate {
		_, err = client.Patch(oldNode.Name, types.StrategicMergePatchType, patchBytes)
	} else {
		r.reqLogger.Infof("failed to create patch. using update %v", err)
		_, err = client.Update(newNode)
	}
	if err != nil {
		r.reqLogger.Errorf("Failed to patch/update error=%v", err)
	}
	return err
}

type errorarray []error

// return error if any one of the errors is a timeout
func isTimeoutError(err error) bool {

	if aerr, ok := err.(utilerrors.Aggregate); ok {
		for _, err := range aerr.Errors() {
			if isTimeoutError(err) { // lets hope they are not doing loops in the graph ;-)
				return true
			}
		}
		return false
	}

	if err, ok := err.(net.Error); ok && err.Timeout() {
		return true
	}
	return false
}
