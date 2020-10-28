package objectlock

import (
	"context"
	"errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	le "k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sync"
	"time"
)

var existingLocks = make(map[string]*NodeLock)
var mutexLock = &sync.Mutex{}

type NodeLock struct {
	lockConfig    LockConfig
	leaderElector *le.LeaderElector
	//cancelFunc is used to terminate the leader election routine that is trying to renew the lease
	cancelFunc              func()
	leaderElectionCfg       le.LeaderElectionConfig
	hasActiveLeaderElection bool
}

type LockConfig struct {
	// name of the lease object that will be created, it should be the same across all users of these lock
	LockName string
	// eventsChannel is where a new event is written to once lock holder changes.
	// it is recommended to add a channel watch for this so it triggers reconcile() again
	EventsChannel chan<- event.GenericEvent
	// objMeta is the object meta of the object that will be written to the eventsChannel
	ObjMeta metav1.ObjectMeta
	// identity is the name of the lock holder
	Identity  string
	Clientset kubernetes.Interface
	// lockDuration represents the initial lock duration
	LockDuration time.Duration
	// Namespace is the namespace in which the lease lock will be created
	Namespace string
}

// GetOrCreate creates a new NodeLock with the given arguments. If a NodeLock was already created
// for the given node, it will return the previously created NodeLock
func GetOrCreate(lockConfig LockConfig) (*NodeLock, error) {
	mutexLock.Lock()
	defer mutexLock.Unlock()

	if lockConfig.Identity == "" {
		return nil, errors.New("identity can't be empty string")
	}

	if lockConfig.EventsChannel == nil {
		return nil, errors.New("events channel can't be nil")
	}

	if lockConfig.LockName == "" {
		return nil, errors.New("lock name can't be empty string")
	}

	if existingLock, exists := existingLocks[lockConfig.LockName]; exists {
		//todo what should we do if existingLock.lockConfig != lockConfig
		return existingLock, nil
	}

	lock := &NodeLock{
		lockConfig: lockConfig,
	}

	lock.createLeaderElectionCfg()
	existingLocks[lock.lockConfig.LockName] = lock
	return lock, nil
}

func (lock *NodeLock) Release() {
	if lock.hasActiveLeaderElection && lock.cancelFunc != nil {
		//start workaround until https://github.com/kubernetes/kubernetes/pull/80954 is in client-go
		now := metav1.Now()
		leaderElectionRecord := resourcelock.LeaderElectionRecord{
			LeaderTransitions:    1,
			LeaseDurationSeconds: 1,
			RenewTime:            now,
			AcquireTime:          now,
		}

		_ = lock.leaderElectionCfg.Lock.Update(context.Background(), leaderElectionRecord)
		//end workaround until https://github.com/kubernetes/kubernetes/pull/80954 is in client-go

		lock.cancelFunc()
	}

	lock.hasActiveLeaderElection = false
	delete(existingLocks, lock.leaderElectionCfg.Name)
}

func (lock *NodeLock) StartLockingLoop() error {
	if lock.hasActiveLeaderElection {
		//there's already existing go routine that is trying to acquire the lock
		return nil
	}

	if lock.leaderElector == nil {
		var err error
		if lock.leaderElector, err = le.NewLeaderElector(lock.leaderElectionCfg); err != nil {
			return err
		}
	}

	lock.hasActiveLeaderElection = true
	ctx, cancelFunc := context.WithCancel(context.Background())
	lock.cancelFunc = cancelFunc
	go lock.leaderElector.Run(ctx)

	return nil
}

func (lock *NodeLock) IsLockAcquired() bool {
	if lock.leaderElector == nil {
		return false
	}
	return lock.leaderElector.IsLeader()
}

func (lock *NodeLock) GetHolderId() string {
	if lock.leaderElector == nil {
		return ""
	}

	return lock.leaderElector.GetLeader()
}

func (lock *NodeLock) addNodeEventToReconcileQueue() {
	nodeEvent := event.GenericEvent{Meta: &lock.lockConfig.ObjMeta}
	//todo this could blocking if channel is full
	lock.lockConfig.EventsChannel <- nodeEvent
}

func (lock *NodeLock) createLeaderElectionCfg() {
	//todo add node owner ref to lease (pending upstream PR)
	leaseLock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      lock.lockConfig.LockName,
			Namespace: lock.lockConfig.Namespace,
		},
		Client: lock.lockConfig.Clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: lock.lockConfig.Identity,
		},
	}

	leaderElectionConfig := le.LeaderElectionConfig{
		Lock:          leaseLock,
		LeaseDuration: lock.lockConfig.LockDuration,
		RenewDeadline: 30 * time.Second,
		RetryPeriod:   10 * time.Second,
		Callbacks: le.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				lock.addNodeEventToReconcileQueue()
			},
			OnStoppedLeading: func() {
				lock.hasActiveLeaderElection = false
				lock.addNodeEventToReconcileQueue()
			},
			OnNewLeader: func(identity string) {
				//todo is this redundant?
				lock.addNodeEventToReconcileQueue()
			},
		},
		WatchDog:        nil,
		ReleaseOnCancel: true,
		Name:            lock.lockConfig.LockName,
	}

	lock.leaderElectionCfg = leaderElectionConfig
}
