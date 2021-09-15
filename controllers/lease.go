package controllers

import "time"

const (
	LeaseDuration         = 3600 * time.Second
	LeaseHolderIdentity   = "node-maintenance"
	LeaseNamespaceDefault = "node-maintenance"
	LeaseApiPackage       = "coordination.k8s.io/v1"
)
