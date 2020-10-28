package lease

import (
	"context"
	"fmt"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type ErrorForeignHolder struct{ error }
type ErrorNoHolder struct{ error }
type ErrorExpired struct{ error }

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

type Handler struct {
	Name    string
	Lease   *resourcelock.LeaderElectionRecord
	Status  Status
	handler resourcelock.Interface
}

// Create a lease handler
//
// namespace: The namespace for the underlying lease object
// name: The name for the underlying lease object
// identity: The identity of the caller
// config: The k8s config for creating needed clients
func New(namespace, name, identity string, client kubernetes.Interface) (*Handler, error) {
	handler, err := resourcelock.New(resourcelock.LeasesResourceLock, namespace, name, client.CoreV1(), client.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity:      identity,
			EventRecorder: nil,
		},
	)
	if err != nil {
		return nil, err
	}

	return &Handler{
		Name:    name,
		handler: handler,
	}, nil
}

// Get a node lease
//
// leaseDuration: The duration the lease will be valid without being renewed
// renewPeriod: Renew the lease if it expires within this duration
func (h *Handler) Get(leaseDuration time.Duration, renewPeriod time.Duration) error {

	// check if we have a lease already
	ler, _, err := h.handler.Get(context.TODO())
	if !k8serrors.IsNotFound(err) {
		// something went wrong
		return err
	}
	if ler != nil {
		// check existing lease
		h.Lease = ler
		h.Status = Existed

		if err := h.isValid(*ler); err != nil {
			switch err.(type) {
			case ErrorNoHolder, ErrorExpired:
				// no holder or expired, let's take it
				now := metav1.Now()
				ler.HolderIdentity = h.handler.Identity()
				ler.LeaseDurationSeconds = int(leaseDuration.Seconds())
				ler.AcquireTime = now
				ler.RenewTime = now
				ler.LeaderTransitions += 1
				err = h.handler.Update(context.TODO(), *ler)
				if err != nil {
					h.Status = TakingFailed
					return err
				}
				h.Status = Taken
				return nil
			case ErrorForeignHolder:
				// valid but not ours, nothing we can do
				h.Status = ForeignOwner
				return err
			default:
				return fmt.Errorf("unhandled lease validation error! %v", err)
			}
		}

		// Renew if needed
		// TODO this only works Get() is called quick enough (< renewPeriod)
		// alternatives:
		// - let the user call Renew if needed?
		// - start a async renew loop for renewals until Release is called?
		if ler.RenewTime.Add(time.Duration(ler.LeaseDurationSeconds) * time.Second).After(time.Now().Add(renewPeriod)) {
			err = h.Renew()
		}

		// it's ours and valid :)
		return err
	}

	// no lease yet, let's create a new one
	now := metav1.Now()
	ler = &resourcelock.LeaderElectionRecord{
		HolderIdentity:       h.handler.Identity(),
		LeaseDurationSeconds: int(leaseDuration.Seconds()),
		AcquireTime:          now,
		RenewTime:            now,
		LeaderTransitions:    0,
	}

	h.Lease = ler
	h.Status = Created
	err = h.handler.Create(context.TODO(), *ler)
	if err != nil {
		h.Status = CreationFailed
	}

	return err
}

func (h *Handler) Renew() error {
	return h.update(true)
}

func (h *Handler) Release() error {
	return h.update(false)
}

func (h *Handler) update(renew bool) error {
	ler, _, err := h.handler.Get(context.TODO())
	if err != nil {
		return err
	}
	h.Lease = ler

	if renew {
		ler.RenewTime = metav1.Now()
		h.Status = Renewed
	} else {
		// oops.. we can' t set time fields to nil... how to release a lease now?
		// maybe this is enough
		ler.HolderIdentity = ""
		h.Status = Released
	}

	err = h.handler.Update(context.TODO(), *ler)
	if err != nil {
		if renew {
			h.Status = RenewFailed
		} else {
			h.Status = ReleaseFailed
		}
	}
	return err
}

func (h *Handler) IsValid() error {
	ler, _, err := h.handler.Get(context.TODO())
	if err != nil {
		return err
	}
	h.Lease = ler
	return h.isValid(*ler)
}

func (h *Handler) isValid(ler resourcelock.LeaderElectionRecord) error {
	// Order matters!
	// Returning ErrorForeignHolder also means that it's NOT expired
	if ler.HolderIdentity == "" {
		return ErrorNoHolder{}
	} else if ler.RenewTime.Add(time.Duration(ler.LeaseDurationSeconds) * time.Second).Before(time.Now()) {
		return ErrorExpired{}
	} else if ler.HolderIdentity != h.handler.Identity() {
		return ErrorForeignHolder{}
	}
	return nil
}
