package lease

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type ErrorForeignHolder struct{}

func (e *ErrorForeignHolder) Error() string { return "ForeignHolder" }

type ErrorNoHolder struct{}

func (e *ErrorNoHolder) Error() string { return "NoHolder" }

type ErrorExpired struct{}

func (e *ErrorExpired) Error() string { return "Expired" }

type Status string

const (
	Existed        Status = "LeaseExisted"
	Created        Status = "LeaseCreated"
	CreationFailed Status = "LeaseCreationFailed"
	Taken          Status = "LeaseTaken"
	TakingFailed   Status = "LeaseTakingFailed"
	Renewed        Status = "LeaseRenewed"
	RenewFailed    Status = "LeaseRenewFailed"
	Released       Status = "LeaseReleased"
	ReleaseFailed  Status = "LeaseReleaseFailed"
	ForeignOwner   Status = "LeaseHasForeignOwner"
)

type CallbackReason string

const (
	Lost CallbackReason = "LeaseLost"
)

type Callback func(reason CallbackReason)

var handlers = make(map[string]*Handler)
var handlersLock = sync.Mutex{}

type Handler struct {
	ID               string
	Lease            *resourcelock.LeaderElectionRecord
	Status           Status
	ourLeaseDuration time.Duration
	renewAfter       time.Duration
	lock             resourcelock.Interface
	renewTicker      clock.Ticker
	callback         Callback
	logger           *log.Entry
}

// Create a lease lock
//
// namespace: The namespace for the underlying lease object
// name: The name for the underlying lease object
// identity: The identity of the caller
// config: The k8s config for creating needed clients
func New(namespace, name, identity string, client kubernetes.Interface, leaseDuration time.Duration) (*Handler, error) {

	logger := log.WithFields(log.Fields{"Lease.HolderIdentity": identity, "Lease.Namespace": namespace, "Lease.Name": name})

	handlersLock.Lock()
	defer handlersLock.Unlock()

	id := getID(identity, namespace, name)

	handler := handlers[id]
	if handler != nil {
		return handler, nil
	}

	lock, err := resourcelock.New(resourcelock.LeasesResourceLock, namespace, name, client.CoreV1(), client.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity:      identity,
			EventRecorder: nil,
		},
	)
	if err != nil {
		return nil, err
	}

	handler = &Handler{
		ID:               id,
		lock:             lock,
		ourLeaseDuration: leaseDuration,
		renewAfter:       leaseDuration / 3,
		logger:           logger,
	}
	handlers[id] = handler
	return handler, nil
}

func getID(parts ...string) string {
	id := ""
	for _, part := range parts {
		id += part
	}
	return id
}

// Get a node lease
//
// callBack: The callback func to call for notifying about async important events, e.g. a lost lease
func (h *Handler) Get(callback Callback) error {

	h.callback = callback

	// check if we have a lease already
	err := h.getLatest()
	if err != nil && !k8serrors.IsNotFound(err) {
		// something went wrong
		h.logger.WithError(err).Error("error getting lease")
		return err
	}

	// there is a lease already, validate it
	if h.Lease != nil {
		h.Status = Existed
		return h.validate(false)
	}

	// no lease yet, let's create a new one
	now := metav1.Now()
	h.Lease = &resourcelock.LeaderElectionRecord{
		HolderIdentity:       h.lock.Identity(),
		LeaseDurationSeconds: int(h.ourLeaseDuration.Seconds()),
		AcquireTime:          now,
		RenewTime:            now,
		LeaderTransitions:    0,
	}
	h.Status = Created
	err = h.lock.Create(context.TODO(), *h.Lease)
	if err != nil {
		h.logger.WithError(err).Error("error creating lease")
		h.Status = CreationFailed
	} else {
		h.startRenewals()
	}
	h.logger.Info("lease created")

	return err
}

func (h *Handler) getLatest() error {
	ler, _, err := h.lock.Get(context.TODO())
	if err != nil {
		return err
	}
	h.Lease = ler
	return nil
}

func (h *Handler) validate(getLatest bool) error {
	if getLatest {
		if err := h.getLatest(); err != nil {
			h.logger.WithError(err).Error("error getting lease")
			return err
		}
	}

	if err := h.isValid(); err != nil {
		switch err.(type) {
		case *ErrorNoHolder, *ErrorExpired:
			// no holder or expired, let's take it
			return h.takeLease()
		case *ErrorForeignHolder:
			// valid but not ours, nothing we can do
			h.logger.Errorf("lease owned by other component: %v", h.Lease.HolderIdentity)
			h.Status = ForeignOwner
			return err
		default:
			return fmt.Errorf("unhandled lease validation error! %v", err)
		}
	}

	// it's our valid lease
	// but renew now if needed
	if time.Now().After(h.Lease.RenewTime.Add(h.renewAfter)) {
		h.Lease.RenewTime = metav1.Now()
		h.Status = Renewed
		err := h.lock.Update(context.TODO(), *h.Lease)
		if err != nil {
			h.logger.WithError(err).Error("error renewing lease")
			h.Status = RenewFailed
			return err
		}
		h.logger.Info("lease renewed")
	} else {
		h.logger.Info("lease valid")
	}

	return nil
}

func (h *Handler) IsValid() error {
	h.getLatest()
	return h.isValid()
}

func (h *Handler) isValid() error {
	// Order matters!
	// Returning ErrorForeignHolder also means that it's NOT expired
	if h.Lease.HolderIdentity == "" {
		return &ErrorNoHolder{}
	} else if h.Lease.RenewTime.Add(time.Duration(h.Lease.LeaseDurationSeconds) * time.Second).Before(time.Now()) {
		return &ErrorExpired{}
	} else if h.Lease.HolderIdentity != h.lock.Identity() {
		return &ErrorForeignHolder{}
	}
	return nil
}

func (h *Handler) takeLease() error {
	now := metav1.Now()
	h.Lease.HolderIdentity = h.lock.Identity()
	h.Lease.LeaseDurationSeconds = int(h.ourLeaseDuration.Seconds())
	h.Lease.AcquireTime = now
	h.Lease.RenewTime = now
	h.Lease.LeaderTransitions += 1
	err := h.lock.Update(context.TODO(), *h.Lease)
	if err != nil {
		h.logger.WithError(err).Error("error taking lease")
		h.Status = TakingFailed
		return err
	}
	h.logger.Info("lease taken")
	h.Status = Taken
	h.startRenewals()
	return nil
}

func (h *Handler) startRenewals() {
	if h.renewTicker != nil {
		// already done
		return
	}
	ticker := clock.RealClock{}.NewTicker(h.renewAfter)
	h.renewTicker = ticker
	go func() {
		for {
			select {
			case <-ticker.C():
				if err := h.validate(true); err != nil {
					if h.Status == RenewFailed {
						// TODO retry faster than we do with the normal ticker?
					} else if _, ok := err.(*ErrorForeignHolder); ok {
						// we lost the lease!
						// notify user
						h.callback(Lost)
						// cleanup by calling release
						h.Release()
					}
				}
			case <-time.After(h.renewAfter):
				// just for being able to exit the loop in case the lease was released
			}
			if h.renewTicker == nil {
				break
			}
		}
	}()
}

func (h *Handler) Release() error {
	if h.lock == nil {
		// already done
		return nil
	}

	handlersLock.Lock()
	defer handlersLock.Unlock()

	// stop renewals
	if h.renewTicker != nil {
		h.renewTicker.Stop()
		h.renewTicker = nil
	}

	// check if it's still ours...
	h.getLatest()
	if err := h.isValid(); err != nil {
		switch err.(type) {
		case *ErrorNoHolder, *ErrorForeignHolder:
			// we are not holder anymore
			h.cleanup()
			return nil
		case *ErrorExpired:
			// let's still remove our identity
			break
		default:
			return fmt.Errorf("unhandled lease validation error! %v", err)
		}
	}

	// release the lease by removing our identity
	h.Lease.HolderIdentity = ""
	err := h.lock.Update(context.TODO(), *h.Lease)
	if err != nil {
		h.logger.WithError(err).Error("error releasing lease")
		h.Status = ReleaseFailed
		return err
	}
	h.logger.Info("lease released")
	h.cleanup()
	return nil
}

func (h *Handler) cleanup() {
	// the handlersLock should be acquired already when calling this!
	h.Status = Released
	h.lock = nil
	h.Lease = nil
	handlers[h.ID] = nil
	h.logger.Info("cleanup done")
}
