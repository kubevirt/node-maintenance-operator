package nodemaintenance

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	kubernetes "k8s.io/client-go/kubernetes"
	le "k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

)

const (
	LeaseDuration2         = 3600 * time.Second
	LeaseNamespaceDefault2 = "node-maintenance"
	LeaseApiPackage2      = "coordination.k8s.io/v1beta1"
)

// checkLeaseSupportedInternal check if lease object can be used in the cluster
func checkLeaseSupportedInternal2(cs kubernetes.Interface) (bool, error) {
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

// getNodeOwnerRef returns an owner reference of the given node
func getNodeOwnerRef2(node *corev1.Node) *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: node.APIVersion,
		Kind:       node.Kind,
		Name:       node.Name,
		UID:        node.UID,
	}
}

// createOrGetExistingLease creates a lease object for the given node with the given durarion
// it returns the lease object, a boolean to indicate if lease was already exists and an error
// if there was any
func createOrGetExistingLease2(client client.Client, node *corev1.Node, duration time.Duration, holderIdentity string) (*coordv1.Lease, bool, error) {
	rlc := resourcelock.ResourceLockConfig{
		Identity:      LeaseHolderIdentity,
	}

	resourcelock.LeaseLock{
		LeaseMeta:  metav1.ObjectMeta{
			Name:                       "",
			GenerateName:               "",
			Namespace:                  "",
			SelfLink:                   "",
			UID:                        "",
			ResourceVersion:            "",
			Generation:                 0,
			CreationTimestamp:          metav1.Time{},
			DeletionTimestamp:          nil,
			DeletionGracePeriodSeconds: nil,
			Labels:                     nil,
			Annotations:                nil,
			OwnerReferences:            nil,
			Finalizers:                 nil,
			ClusterName:                "",
			ManagedFields:              nil,
		},
		Client:     client,
		LockConfig: rlc,
	}
	leaseLock, _ := resourcelock.New(resourcelock.LeasesResourceLock, LeaseNamespace, node.Name, nil, client, rlc)

	leaderElectionConfig := le.LeaderElectionConfig{
		Lock:           leaseLock,
		LeaseDuration:   LeaseDuration,
		RenewDeadline:   0,
		RetryPeriod:     0,
		Callbacks:       le.LeaderCallbacks{},
		WatchDog:        nil,
		ReleaseOnCancel: true,
		Name:            node.Name,
	}

	leaderElector, _ := le.NewLeaderElector(leaderElectionConfig)
	ctx := context.Background()
	leaderElector.Run()
	leaderElector.Run(ctx)

	ctx.Done()







	// ********
	owner := getNodeOwnerRef(node)
	microTimeNow := metav1.NowMicro()

	lease := &coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:            node.Name,
			Namespace:       LeaseNamespace,
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: coordv1.LeaseSpec{
			HolderIdentity:       &holderIdentity,
			LeaseDurationSeconds: pointer.Int32Ptr(int32(duration.Seconds())),
			AcquireTime:          &microTimeNow,
			RenewTime:            &microTimeNow,
			LeaseTransitions:     pointer.Int32Ptr(0),
		},
	}

	if err := client.Create(context.TODO(), lease); err != nil {
		if errors.IsAlreadyExists(err) {
			nodeName := node.Name
			key := apitypes.NamespacedName{Namespace: LeaseNamespace, Name: nodeName}

			lease := &coordv1.Lease{}
			if err := client.Get(context.TODO(), key, lease); err != nil {
				return nil, false, err
			}
			return lease, true, nil
		}
		return nil, false, err
	}
	return lease, false, nil
}

func leaseDueTime2(lease *coordv1.Lease) time.Time {
	return lease.Spec.RenewTime.Time.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
}

func needUpdateOwnedLease2(lease *coordv1.Lease, currentTime metav1.MicroTime) (bool, bool) {
	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true, true
	}
	dueTime := leaseDueTime(lease)

	// if lease expired right now, then both update the lease and the acquire time (second rvalue)
	// if the acquire time has been previously nil
	if dueTime.Before(currentTime.Time) {
		return true, lease.Spec.AcquireTime == nil
	}

	deadline := currentTime.Add(2 * DrainerTimeout)

	// about to expire, update the lease but no the acquire time (second rvalue)
	return dueTime.Before(deadline), false
}

func isValidLease2(lease *coordv1.Lease, currentTime time.Time) bool {

	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return false
	}

	renewTime := (*lease.Spec.RenewTime).Time
	dueTime := leaseDueTime(lease)

	// valid lease if: due time not in the past and renew time not in the future
	return !dueTime.Before(currentTime) && !renewTime.After(currentTime)
}

func updateLease2(client client.Client, node *corev1.Node, lease *coordv1.Lease, currentTime *metav1.MicroTime,
					duration time.Duration, holderIdentity string) (error, bool) {

	needUpdateLease := false
	setAcquireAndLeaseTransitions := false
	updateAlreadyOwnedLease := false

	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == holderIdentity {
		needUpdateLease, setAcquireAndLeaseTransitions = needUpdateOwnedLease(lease, *currentTime)
		if needUpdateLease {
			updateAlreadyOwnedLease = true

			log.Infof("renew lease owned by nmo setAcquireTime=%t", setAcquireAndLeaseTransitions)

		}
	} else {
		// can't update the lease if it is currently valid.
		if isValidLease(lease, currentTime.Time) {
			return fmt.Errorf("can't update valid lease held by different owner"), false
		}
		needUpdateLease = true

		log.Info("taking over foreign lease")
		setAcquireAndLeaseTransitions = true
	}

	if needUpdateLease {
		if setAcquireAndLeaseTransitions {
			lease.Spec.AcquireTime = currentTime
			if lease.Spec.LeaseTransitions != nil {
				*lease.Spec.LeaseTransitions += int32(1)
			} else {
				lease.Spec.LeaseTransitions = pointer.Int32Ptr(1)
			}
		}
		owner := getNodeOwnerRef(node)
		lease.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*owner}
		lease.Spec.HolderIdentity = &holderIdentity
		lease.Spec.LeaseDurationSeconds = pointer.Int32Ptr(int32(duration.Seconds()))
		lease.Spec.RenewTime = currentTime
		if err := client.Update(context.TODO(), lease); err != nil {
			//formattedErr := fmt.Errorf("failed to update the lease. node %s error: %v", node.Name, err)
			log.Errorf("Failed to update the lease. node %s error: %v", node.Name, err)
			return err, updateAlreadyOwnedLease
		}
	}

	return nil, false
}

func invalidateLease2(client client.Client, nodeName string) error {
	nName := apitypes.NamespacedName{Namespace: LeaseNamespace, Name: nodeName}
	lease := &coordv1.Lease{}

	if err := client.Get(context.TODO(), nName, lease); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	lease.Spec.AcquireTime = nil
	lease.Spec.LeaseDurationSeconds = nil
	lease.Spec.RenewTime = nil
	lease.Spec.LeaseTransitions = nil

	return client.Update(context.TODO(), lease)
}
