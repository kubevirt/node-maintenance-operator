package nodemaintenance

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kubernetes "k8s.io/client-go/kubernetes"
)

func makeInt32(val int32) (*int32) {
	tmpVal := val
	return &tmpVal
}

func makeTimeNow(time time.Time) (*metav1.MicroTime) {

	timeNow := metav1.MicroTime{Time: time}
	return &timeNow
}

func checkLeaseSupportedInternal(cs kubernetes.Interface) (bool, error) {

	groupList, err := cs.Discovery().ServerGroups()
	if err != nil {
		return false, err
	}
	if groupList != nil {
		apiVersions := metav1.ExtractGroupVersions(groupList)
		for _, v := range apiVersions {
			if v == LeaseApiPackage {
				return true, nil
			}
		}
	}
	return false, nil
}

func makeExpectedOwnerOfLease(node *corev1.Node) (*metav1.OwnerReference) {
	return &metav1.OwnerReference{
		APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
		Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
		Name:       node.ObjectMeta.Name,
		UID:        node.ObjectMeta.UID,
	}
}

func createOrGetExistingLease(client  client.Client, node *corev1.Node, durationInSeconds int32) (*coordv1beta1.Lease, bool, error) {
	holderIdentity := LeaseHolderIdentity
	owner := makeExpectedOwnerOfLease(node)
	microTimeNow :=  makeTimeNow(time.Now())

	lease := &coordv1beta1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:            node.ObjectMeta.Name,
			Namespace:       LeaseNamespace,
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: coordv1beta1.LeaseSpec{
			HolderIdentity:       &holderIdentity,
			LeaseDurationSeconds: makeInt32(durationInSeconds),
			AcquireTime:          microTimeNow,
			RenewTime:            microTimeNow,
			LeaseTransitions:     makeInt32(0),
		},
	}

	if err := client.Create(context.TODO(), lease); err != nil {
		if errors.IsAlreadyExists(err) {

				nodeName := node.ObjectMeta.Name
				key := apitypes.NamespacedName{Namespace: LeaseNamespace, Name: nodeName}

				if err := client.Get(context.TODO(), key, lease); err != nil {
					return  nil, false, err
				}
				return lease, true, nil
		}
		return nil, false, err
	}
	return lease, false,  nil
}

func leaseDueTime(lease *coordv1beta1.Lease, durationInSeconds time.Duration) (time.Time) {
	dueTime := lease.Spec.RenewTime.Time
	return dueTime.Add(durationInSeconds * time.Second)
}

func needUpdateOwnedLease(lease *coordv1beta1.Lease, currentTime time.Time) (bool,bool) {

	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		log.Info("empty renew time or duration in sec")
		return true, true
	}
	duration := time.Duration(*lease.Spec.LeaseDurationSeconds)
	dueTime := leaseDueTime(lease, duration)

	deadline := currentTime

	// if lease expired right now, then both update the lease and the acquire time (second rvalue)
	// if the acquire time has been previously nil
	if dueTime.Before(deadline) {
		return true, lease.Spec.AcquireTime == nil
	}

	deadline = deadline.Add(2 * drainerTimeoutInSeconds * time.Second)

	// about to expire, update the lease but no the acquire time (second rvalue)
	return dueTime.Before(deadline), false
}

func isValidLease(lease *coordv1beta1.Lease, currentTime time.Time) bool {

	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return false
	}

	duration := time.Duration(*lease.Spec.LeaseDurationSeconds)
	renewTime := (*lease.Spec.RenewTime).Time
	dueTime := leaseDueTime(lease, duration)

	// valid lease if: due time not in the past and renew time not in the future
	return !dueTime.Before(currentTime) && !renewTime.After(currentTime)
}

func updateLease(client  client.Client, node *corev1.Node, lease *coordv1beta1.Lease, currentTime time.Time, durationInSeconds int32) (*coordv1beta1.Lease, error, bool) {

	holderIdentity := LeaseHolderIdentity

	needUpdateLease := false
	setAcquireAndLeaseTransitions := false
	updateAlreadyOwnedLease := false

	if  lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == holderIdentity {
		needUpdateLease, setAcquireAndLeaseTransitions = needUpdateOwnedLease(lease,currentTime)
		if  needUpdateLease {
			updateAlreadyOwnedLease = true

			log.Infof("renew lease owned by nmo setAcquireTime=%t", setAcquireAndLeaseTransitions)

		}
	}  else {
		// can't update the lease if it is currently valid.
		if isValidLease(lease, currentTime) {
			return nil, fmt.Errorf("Can't update valid lease held by different owner"), false
		}
		needUpdateLease = true

		log.Info("taking over foreign lease")
		setAcquireAndLeaseTransitions = true
	}

	if needUpdateLease {
		if setAcquireAndLeaseTransitions {
			lease.Spec.AcquireTime = makeTimeNow(currentTime)
			if lease.Spec.LeaseTransitions != nil {
				*lease.Spec.LeaseTransitions += int32(1)
			} else {
				lease.Spec.LeaseTransitions = makeInt32(1)
			}
		}
		owner := makeExpectedOwnerOfLease(node)
		lease.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*owner}
		lease.Spec.HolderIdentity = &holderIdentity
		lease.Spec.LeaseDurationSeconds = makeInt32(durationInSeconds)
		lease.Spec.RenewTime = makeTimeNow(currentTime)
		if err := client.Update(context.TODO(), lease); err != nil {
			log.Errorf("Failed to update the lease. node %s error: %v", node.Name, err)
			return lease, err, updateAlreadyOwnedLease
		}
	}

	return lease, nil, false
}

func invalidateLease(client  client.Client, nodeName string) error {
	log.Info("Lease object supported, invalidating lease")

	nName := apitypes.NamespacedName{Namespace: LeaseNamespace, Name: nodeName}
	lease := &coordv1beta1.Lease{}

	if err := client.Get(context.TODO(), nName, lease); err != nil {

		if  errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	lease.Spec.AcquireTime = nil
	lease.Spec.LeaseDurationSeconds = nil
	lease.Spec.RenewTime = nil
	lease.Spec.LeaseTransitions = nil

	if err := client.Update(context.TODO(), lease); err != nil {
		return err
	}
	return nil
}

