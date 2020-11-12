package lease

import (
	"context"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"time"
)

type ObjectLeaseLock struct {
	ObjectLeaseLockCfg
	leaseLock *resourcelock.LeaseLock
}

type ObjectLeaseLockCfg struct {
	HolderId             string
	Clientset            kubernetes.Interface
	Namespace            string
	Name                 string
	DesiredLeaseDuration time.Duration
	GraceRenewDuration   time.Duration
}

// obtainLease acquires a lease
// returns a boolean indicating if successfully acquired the lease
// and an expiration time for the desired lease (either owned by us or not)
// the expiration time is reduced by some margin given in the lockCfg if the lease is ours to allow extension
func (oll *ObjectLeaseLock) ObtainLease() (bool, *time.Duration, error) {
	oll.ensureLeaseLockExists()

	leaseRecord, _, err := oll.leaseLock.Get(context.Background())

	if err != nil {
		if errors.IsNotFound(err) {
			return oll.createLock()
		}
		return false, nil, err
	}

	now := time.Now()
	leaseExpireTime := getLeaseExpirationTime(leaseRecord)
	leaseExpireDuration := now.Sub(leaseExpireTime)

	if leaseRecord.HolderIdentity == oll.HolderId {
		//lease is ours. renew if needed
		if leaseExpireDuration <= oll.GraceRenewDuration {
			leaseRecord.RenewTime = metav1.Now()
			if err := oll.leaseLock.Update(context.Background(), *leaseRecord); err != nil {
				return false, &leaseExpireDuration, err
			}
		}
		durationToRenew := leaseExpireDuration-oll.GraceRenewDuration
		return true, &durationToRenew, nil
	}

	//lease is not ours
	if isLeaseExpired(leaseRecord){
		//lease expired - take it
		if err := oll.acquire(leaseRecord); err != nil {
			return false, nil, err
		}
		return true, &oll.DesiredLeaseDuration, nil
	}

	//lease is not ours and yet to be expired
	return false, &leaseExpireDuration, nil
}

func (oll *ObjectLeaseLock) Release() error {
	oll.ensureLeaseLockExists()
	leaseRecord, _, err := oll.leaseLock.Get(context.Background())

	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if leaseRecord.HolderIdentity != oll.HolderId {
		return nil
	}

	if isLeaseExpired(leaseRecord){
		return nil
	}

	leaseRecord.RenewTime = metav1.Now()
	leaseRecord.AcquireTime = metav1.Now()
	leaseRecord.LeaseDurationSeconds = 1

	return oll.leaseLock.Update(context.Background(), *leaseRecord)
}

func getLeaseExpirationTime(lease *resourcelock.LeaderElectionRecord) time.Time {
	leaseDuration := time.Duration(lease.LeaseDurationSeconds) * time.Second
	return lease.RenewTime.Add(leaseDuration)
}

func isLeaseExpired(lease *resourcelock.LeaderElectionRecord) bool {
	now := time.Now()
	leaseExpireTime := getLeaseExpirationTime(lease)
	return now.After(leaseExpireTime)
}

func (oll *ObjectLeaseLock) ensureLeaseLockExists() {
	if oll.leaseLock != nil {
		return
	}

	oll.leaseLock = &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      oll.Name,
			Namespace: oll.Namespace,
		},
		Client: oll.Clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      oll.HolderId,
			EventRecorder: nil,
		},
	}
}

func (oll *ObjectLeaseLock) acquire(leaseRecord *resourcelock.LeaderElectionRecord) error {
	leaseRecord.LeaderTransitions += 1
	leaseRecord.HolderIdentity = oll.HolderId
	leaseRecord.RenewTime = metav1.Now()
	leaseRecord.AcquireTime = metav1.Now()
	leaseRecord.LeaseDurationSeconds = int(oll.DesiredLeaseDuration.Seconds())

	return oll.leaseLock.Update(context.Background(), *leaseRecord)
}

func (oll *ObjectLeaseLock) createLock() (bool, *time.Duration, error) {
	ler := resourcelock.LeaderElectionRecord{
		HolderIdentity:       oll.HolderId,
		LeaseDurationSeconds: int(oll.DesiredLeaseDuration.Seconds()),
		AcquireTime:          metav1.Now(),
		RenewTime:            metav1.Now(),
		LeaderTransitions:    0,
	}

	if err := oll.leaseLock.Create(context.Background(), ler); err != nil {
		return false, nil, err
	}

	return true, &oll.DesiredLeaseDuration, nil
}
