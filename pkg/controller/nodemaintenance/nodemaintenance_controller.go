//go:generate mockgen -source $GOFILE -package=$GOPACKAGE -destination=generated_mock_$GOFILE

package nodemaintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
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

	"k8s.io/apimachinery/pkg/fields"

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
	EvictionTimeSlice           = 10 * time.Second // max. time in seconds that eviction may block.
	RequeuDrainingWaitTime      = 10 * time.Second // time to reqeueu draining if not completed.
)

type LeaseStatus int

const (
	LeaseStatusNotFound               LeaseStatus = -1 // currently not active lease
	LeaseStatusOwnedByMe              LeaseStatus = 1  // there is a lease active right now
	LeaseStatusOwnedByDifferentHolder LeaseStatus = 2
	LeaseStatusFail                   LeaseStatus = 3
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

func NewDeadlineInSeconds(seconds int64) DeadlineCheck {
	tm := time.Now()
	tm.Add(time.Duration(seconds) * time.Second)

	return DeadlineCheck{isSet: true, deadline: tm}
}

func NewDeadlineAt(atTime time.Time) DeadlineCheck {
	return DeadlineCheck{isSet: true, deadline: atTime}
}

func (exp DeadlineCheck) IsExpired() bool {
	return exp.isSet && exp.deadline.After(time.Now())
}
func (exp DeadlineCheck) DurationUntilExpiration() time.Duration {
	if exp.isSet {
		return exp.deadline.Sub(time.Now())
	}
	// better not be here, better check if the deadline has been set.
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

	//log.SetLevel(log.DebugLevel)

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
			r.reqLogger.Info("cannot retrieve nod. not found")
			return reconcile.Result{}, nil
		}
		r.reqLogger.Infof("cannot retrieve node. error: %v", err)
		return reconcile.Result{}, err
	}

	// parse status & spec, create initial status object on first call
	err := r.parseAnnotations()
	if err != nil {
		r.reqLogger.Errorf("request parsing error. error: %v", err)
		return reconcile.Result{}, err
	}

	if r.specAnnoData == nil && r.statusAnnoData == nil {
		return r.processNoAnnotations()
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

	if leaseStatus == LeaseStatusFail || (leaseStatus == LeaseStatusOwnedByDifferentHolder && isLeasePeriodSpecified(lease) && isLeaseValid(lease, false)) {
		r.reqLogger.Debugf("MaintOn: waiting for lease. leaseStatus=%d", int32(leaseStatus))
		r.setAnnotations(NodeStateWaiting, false)
		return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
	}

	var err error = nil

	if leaseStatus == LeaseStatusNotFound {
		r.reqLogger.Infof("MaintOn: create new lease - no existing lase found")

		if lease, err = r.createOrUpdateLease(nil, TransitionSet, NodeStateNewCreate); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
	}

	if leaseStatus == LeaseStatusOwnedByDifferentHolder {
		r.reqLogger.Infof("MaintOn: update lease -  different owner & invalid lease ")

		if _, err = r.createOrUpdateLease(lease, TransitionInc, NodeStateNewAcquired); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

	}

	if !isLeasePeriodSpecified(lease) || isLeaseDurationZero(lease) {
		r.reqLogger.Infof("MaintOn: update lease - null lease period or zero duration")

		if _, err = r.createOrUpdateLease(lease, TransitionInc, NodeStateNew); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

	}

	if r.isLeaseDurationChanged(lease) {
		r.reqLogger.Info("MaintOn: update lease - lease duration change requested")

		if _, err = r.createOrUpdateLease(lease, TransitionNoChange, NodeStateUpdated); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

	}

	if !isLeaseValid(lease, false) {
		r.reqLogger.Info("MaintOn: update lease - lease expired")

		if _, err = r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNewStale); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
	}

	if r.statusAnnoData != nil && *r.statusAnnoData == ReconcileNodeMaintenanceStatusInfo(NodeStateEnded) {
		r.reqLogger.Info("MaintOn: update lease - lease ended")

		if _, err = r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNewRecreate); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
	}

	return r.handleMaintModeTransition(lease)
}

func (r *ReconcileNodeMaintenance) handleMaintModeTransition(lease *coordv1beta1.Lease) (reconcile.Result, error) {

	dcheck := DeadlineCheck{}

	if !isLeaseValid(lease, true) {
		r.reqLogger.Info("MaintOn: lease to expire within padding.")

		r.setAnnotations(NodeStateEnded, true)

		dcheck = NewDeadlineAt(leasePeriodEnd(lease))

		for !dcheck.IsExpired() {
			if err := r.runCordonOrUncordon(false, dcheck); err != nil {
				log.Errorf("MaintOn: retry loop: Uncordon failed err=%v", err)
			} else {
				log.Infof("MaintOn: retry loop: Uncordon ok")
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		for !dcheck.IsExpired() {
			if err := r.cancelEviction(dcheck); err != nil {
				log.Errorf("MaintOn: retry loop: cancelEviction failed: err=%v", err)
			} else {
				log.Infof("MaintOn: retry loop: cancelEviction ok")
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		for !dcheck.IsExpired() {
			if _, err := r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNone); err != nil { // set lease duration to 0.a
				log.Errorf("MaintOn: retry loop: updateLease failed: err=%v", err)
			} else {
				log.Infof("MaintOn: retry loop: updateLease")
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

	} else {
		r.reqLogger.Debugf("MaintOn: lease ok. (regular case)")

		if err := r.runCordonOrUncordon(true, dcheck); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

		if err := r.evictPods(dcheck); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

		if err := r.setAnnotations(NodeStateActive, false); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

	}
	return reconcile.Result{}, nil
}

func (r *ReconcileNodeMaintenance) processNoAnnotations() (reconcile.Result, error) {

	dcheck := DeadlineCheck{}

	leaseStatus, _, lease := r.doesLeaseExist()
	if leaseStatus == LeaseStatusOwnedByMe && isLeaseValid(lease, false) {
		r.reqLogger.Info("NoAnnotation: valid lease exists. cleaning up")

		if err := r.runCordonOrUncordon(false, dcheck); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

		if err := r.cancelEviction(dcheck); err != nil {
			r.reqLogger.Errorf("NoAnnotation: cancelEviction failed %v", err)
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}

		if _, err := r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNone); err != nil { // set lease duration to 0.
			r.reqLogger.Errorf("NoAnnotation: update lease failed %v", err)
			return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
		}
	} else {
		r.reqLogger.Debug("NoAnnotation: no annotations and no lease.")
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileNodeMaintenance) processMaintModeOff() (reconcile.Result, error) {

	leaseStatus, _, lease := r.doesLeaseExist()
	if leaseStatus == LeaseStatusOwnedByMe {
		dcheck := DeadlineCheck{isSet: true, deadline: leasePeriodEnd(lease)}

		if isLeaseValid(lease, false) {
			r.reqLogger.Info("MaintOff: lease owned by me & valid")

			// ? why isn't that supposed to be in a 'retry loop?'
			if err := r.setAnnotations(NodeStateEnded, false); err != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
			}

			for !dcheck.IsExpired() {
				if err := r.runCordonOrUncordon(false, dcheck); err != nil {
					log.Infof("MaintOff: cleanup: retry loop: uncordon failed: err=%v", err)
				} else {
					log.Infof("MaintOff: cleanup: uncordon ok")
					break
				}
				time.Sleep(50 * time.Millisecond)
			}

			for !dcheck.IsExpired() {
				if err := r.cancelEviction(dcheck); err != nil {
					log.Infof("MaintOff: cleanup: cancelEviction failed. err=%v", err)
				} else {
					log.Infof("MaintOff: cleanup: cancelEviction ok")
					break
				}
				time.Sleep(50 * time.Millisecond)
			}

		} else if !isLeaseDurationZero(lease) {
			r.reqLogger.Info("MaintOff: lease owned by me & duration positive but lease expired (not valid)")

			tmpDurationInSeconds := int32(LeasePaddingSeconds)
			lease.Spec.LeaseDurationSeconds = &tmpDurationInSeconds
			lease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}

			// why isn't that a 'retry loop'?
			if err := r.client.Update(context.TODO(), lease); err != nil {
				r.reqLogger.Errorf("Failed to update the lease. error: %v", err)
				return reconcile.Result{Requeue: true, RequeueAfter: RequeuDrainingWaitTime}, nil
			}

			for !dcheck.IsExpired() {
				if err := r.runCordonOrUncordon(false, dcheck); err != nil {
					log.Errorf("MaintOff: retry loop: uncordon failed: err=%v", err)
				} else {
					log.Infof("MaintOff: retry loop: uncordon ok")
					break
				}
				time.Sleep(50 * time.Millisecond)
			}

		} else {
			return reconcile.Result{}, nil
		}

		for !dcheck.IsExpired() {
			// check if we are still the current owner. Otherwise it is not possible to set the lease duration to zero (the next step)
			leaseStatus, _, lease := r.doesLeaseExist()
			if leaseStatus != LeaseStatusOwnedByMe {
				log.Errorf("MaintOff: no longer the current owner, can't proceed to set lease duration to zero")
				break
			}
			if _, err := r.createOrUpdateLease(lease, TransitionNoChange, NodeStateNone); err != nil { // set lease duration to 0.
				log.Errorf("MaintOff: retry loop: update lease failed: err=%v", err)
			} else {
				log.Infof("MaintOff: retry loop: update lease ok")
				break
			}
			if r.statusAnnoData != nil && *r.statusAnnoData != ReconcileNodeMaintenanceStatusInfo(string(NodeStateEnded)) {
				r.setAnnotations(NodeStateEnded, false)
			}
			time.Sleep(50 * time.Millisecond)
		}
		log.Infof("MaintOff: eof")
	}
	return reconcile.Result{}, nil
}

// returns true if lease duration differs from node annotation value
func (r *ReconcileNodeMaintenance) isLeaseDurationChanged(lease *coordv1beta1.Lease) bool {
	if lease.Spec.LeaseDurationSeconds != nil {
		value := int32(*r.specAnnoData)
		return *lease.Spec.LeaseDurationSeconds != value+LeasePaddingSeconds
	}
	return false
}

func isLeaseDurationZero(lease *coordv1beta1.Lease) bool {

	return lease.Spec.LeaseDurationSeconds == nil || *lease.Spec.LeaseDurationSeconds == 0
}

func leasePeriodEnd(lease *coordv1beta1.Lease) time.Time {

	if lease.Spec.AcquireTime != nil {
		leasePeriodEnd := (*lease.Spec.AcquireTime).Time

		if lease.Spec.LeaseDurationSeconds != nil {
			leasePeriodEnd.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
		}
		return leasePeriodEnd
	}
	return time.Now()
}

func isLeasePeriodSpecified(lease *coordv1beta1.Lease) bool {
	return lease.Spec.AcquireTime != nil && lease.Spec.LeaseDurationSeconds != nil
}

// check if lease is not expired (precondition that both AcquireTime and LeaseDuration are not nil)
func isLeaseValid(lease *coordv1beta1.Lease, addPaddingToCurrent bool) bool {

	if lease.Spec.AcquireTime == nil {
		// can this happen?
		return false
	}

	leasePeriodEnd := (*lease.Spec.AcquireTime).Time
	if lease.Spec.LeaseDurationSeconds != nil {
		leasePeriodEnd = leasePeriodEnd.Add(time.Duration(int64(*lease.Spec.LeaseDurationSeconds) * int64(time.Second)))
	}

	timeNow := time.Now()

	if addPaddingToCurrent {
		timeNow = timeNow.Add(time.Duration(int64(LeasePaddingSeconds) * int64(time.Second)))
	}
	return !timeNow.After(leasePeriodEnd)
}

func (r *ReconcileNodeMaintenance) doesLeaseExist() (LeaseStatus, error, *coordv1beta1.Lease) {
	lease := &coordv1beta1.Lease{}

	nodeName := r.node.ObjectMeta.Name
	nName := apitypes.NamespacedName{Namespace: corev1.NamespaceNodeLease, Name: nodeName}

	if err := r.client.Get(context.TODO(), nName, lease); err != nil {
		if errors.IsNotFound(err) {
			r.reqLogger.Infof("Lease object not found name=%s ns=%s", nName.Name, nName.Namespace)
			return LeaseStatusNotFound, nil, nil
		}

		r.reqLogger.Errorf("failed to get lease object. name=%s ns=%s", nName.Name, nName.Namespace)
		return LeaseStatusFail, err, nil
	}

	heldByMe := lease.Spec.HolderIdentity != nil && LeaseHolderIdentity == *lease.Spec.HolderIdentity
	r.reqLogger.Debugf("got lease name=%s ns=%s heldByMe=%t", nName.Name, nName.Namespace, heldByMe)

	if heldByMe {
		return LeaseStatusOwnedByMe, nil, lease
	}
	return LeaseStatusOwnedByDifferentHolder, nil, lease
}

func (r *ReconcileNodeMaintenance) createOrUpdateLease(lease *coordv1beta1.Lease, transitions TransitionAction, nextState NodeMaintenanceStatusType) (*coordv1beta1.Lease, error) {

	nodeName := r.node.ObjectMeta.Name
	tmpDurationInSeconds := int32(0)
	if nextState != NodeStateNone {
		if r.specAnnoData == nil {
			return nil, fmt.Errorf("annotation is empty, unexpected state")
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

		var refDuration *int32 = nil
		if tmpDurationInSeconds != 0 {
			refDuration = &tmpDurationInSeconds
		}

		lease = &coordv1beta1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:            nodeName,
				Namespace:       corev1.NamespaceNodeLease,
				OwnerReferences: []metav1.OwnerReference{*owner},
			},
			Spec: coordv1beta1.LeaseSpec{
				HolderIdentity:       &holderIdentity,
				LeaseDurationSeconds: refDuration,
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
		lease.Spec.AcquireTime = &metav1.MicroTime{Time: time.Now()}

		timeNow := metav1.MicroTime{Time: time.Now()}
		lease.Spec.RenewTime = &timeNow

		if transitions == TransitionInc && lease.Spec.LeaseTransitions != nil {
			*lease.Spec.LeaseTransitions += int32(1)
		} else if transitions != TransitionNoChange {
			leaseTransitions := int32(1)
			lease.Spec.LeaseTransitions = &leaseTransitions
		}

		var refDuration *int32 = nil
		if tmpDurationInSeconds != 0 {
			refDuration = &tmpDurationInSeconds
		}
		lease.Spec.LeaseDurationSeconds = refDuration

		if err := r.client.Update(context.TODO(), lease); err != nil {
			r.reqLogger.Errorf("Failed to update the lease. node %s error: %v", nodeName, err)
			return lease, err
		}
		r.reqLogger.Debugf("lease updated: duration: %d sec", tmpDurationInSeconds)
	}

	var err error = nil
	if nextState != NodeStateNone {
		err = r.setAnnotations(nextState, false)
	}
	return lease, err
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
		// If a pod is not evicted in ``EvictionTimeSlice`` seconds, stop waiting and
		// allow it to (hopefully) complete while we process other nodes
		// Pending evictions will be checked and reattempted when the Reconcile()
		// loop gets called again
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
		if !dcheck.IsExpired() {
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
		if !dcheck.IsExpired() {
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

	if !dcheck.IsExpired() {
		nodeName := r.node.ObjectMeta.Name
		list, errs := r.drainer.GetPodsForDeletion(nodeName)
		if errs != nil {
			err := utilerrors.NewAggregate(errs)
			r.reqLogger.Errorf("failed got get pod for eviction %v", err)
			return err
		}

		if !dcheck.IsExpired() && len(list.Pods()) != 0 {

			if dcheck.isSet && EvictionTimeSlice > dcheck.DurationUntilExpiration() {
				r.drainer.Timeout = dcheck.DurationUntilExpiration()
			} else {
				r.drainer.Timeout = EvictionTimeSlice
			}

			r.reqLogger.Infof("start evicting pods, timeout: %d ", r.drainer.Timeout/time.Second)

			// indicate to the user that it is evicting pods.
			if err := r.drainer.DeleteOrEvictPods(list.Pods()); err != nil {
				hasAllTimeout, errorNoTimeout := checkEvictPodsErrorNonTimeoutErrors(err)

				if hasAllTimeout {
					r.reqLogger.Infof("all pod evictions errors were timeout errors")
				} else {
					r.reqLogger.Errorf("non timeout errors during eviction: %v", errorNoTimeout)
				}
				return err // return original error to indicate that the call has failed.
			}

			r.reqLogger.Infof("completed evicting pods")
		}
	}
	return nil
}

type ClientGoCallResult struct {
	err          error
	callerResult interface{}
}

func CallClientGoWithTimeout(client kubernetes.Interface, caller func(client kubernetes.Interface) (error, interface{}), dcheck DeadlineCheck) (interface{}, error) {

	returnCh := make(chan ClientGoCallResult, 1)

	go func() {
		callerResult, err := caller(client)

		if err != nil {
			retErr := fmt.Errorf("GetListOfEvictedPods: error while listing pods: %f", err)
			returnCh <- ClientGoCallResult{err: retErr}
		} else {
			returnCh <- ClientGoCallResult{nil, callerResult}
		}
	}()

	for !dcheck.IsExpired() {
		select {
		case res := <-returnCh:
			return res, res.err
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	return []corev1.Pod{}, fmt.Errorf("GetListOfEvictedPods: timed out")
}

func GetListOfEvictedPods(drainer *drain.Helper, nodeName string, dcheck DeadlineCheck) ([]corev1.Pod, error) {

	cgo := func(client kubernetes.Interface) (error, interface{}) {
		fieldSelector := fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName, "status.phase": "Failed"})
		podList, err := client.CoreV1().Pods(metav1.NamespaceAll).List(metav1.ListOptions{
			FieldSelector: fieldSelector.String()})
		return err, podList

	}
	val, err := CallClientGoWithTimeout(drainer.Client, cgo, dcheck)

	if err != nil {
		return nil, err
	}

	evictedPods := []corev1.Pod{}
	podList, _ := val.(corev1.PodList)

	if podList.Items != nil {
		for _, pod := range podList.Items {
			if pod.Status.Reason == "Evicted" {
				evictedPods = append(evictedPods, pod)
			}
		}
	}
	return evictedPods, nil
}

func (r *ReconcileNodeMaintenance) cancelEviction(dcheck DeadlineCheck) error {

	if dcheck.IsExpired() {
		return fmt.Errorf("cancelEviction timed out")
	}

	nodeName := r.node.ObjectMeta.Name
	podList, err := GetListOfEvictedPods(r.drainer, nodeName, dcheck)

	if err != nil {
		return fmt.Errorf("cancelEviction: failed to enumerate pods in evicted state err=%v", err)
	}

	if len(podList) == 0 {
		return nil
	}

	if dcheck.IsExpired() {
		return fmt.Errorf("cancelEviction timed out after enumerting pods")
	}

	r.drainer.Timeout = dcheck.DurationUntilExpiration()
	r.drainer.DisableEviction = true
	err = r.drainer.DeleteOrEvictPods(podList)
	r.drainer.DisableEviction = false

	r.reqLogger.Infof("start deleting evicted pods, timeout: %d sec podsToDelete: %d ", r.drainer.Timeout/time.Second, len(podList))

	if err != nil {
		return fmt.Errorf("cancelEviction: Failed to delete pods in evicted state err=%v", err)
	} else {
		r.reqLogger.Infof("finshed deleting evicted pods")
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
		log.Error("Can't modify annotation of node", err)
	} else {
		r.node = newNode

		st := ReconcileNodeMaintenanceStatusInfo(string(statusInfo))
		r.statusAnnoData = &st
		if deleteSpec {
			r.specAnnoData = nil
		}
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

func isEvictTimeoutError(err error) bool {
	if err, ok := err.(net.Error); ok && err.Timeout() {
		return true
	}

	// very ugly check: check if string conforms to pattern produced by eviction library.
	//error when evicting pod %q: global timeout reached: %v", pod.Name, globalTimeout)
	msg := err.Error()
	if strings.Index(msg, "error when evicting pod ") != 0 && strings.Index(msg, "global timeout reached: ") != 0 {
		return true
	}
	return false
}

// treatment of aggregate errors;
// if there were non timeout errors in the composite: return a new composite with only non timeout errors.
// return true if all errors were timeout errors
//
func checkEvictPodsErrorNonTimeoutErrors(err error) (bool, error) {
	var errors []error

	if aerr, ok := err.(utilerrors.Aggregate); ok {
		for _, err := range aerr.Errors() {
			if !isEvictTimeoutError(err) {
				errors = append(errors, err)
			}
		}
	} else {
		if !isEvictTimeoutError(err) {
			errors = append(errors, err)
		}
	}
	if len(errors) == 0 {
		return true, nil
	}
	return false, utilerrors.NewAggregate(errors)
}
