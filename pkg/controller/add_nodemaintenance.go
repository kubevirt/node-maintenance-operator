package controller

import (
	"kubevirt.io/node-maintenance-operator/pkg/controller/nodemaintenance"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, nodemaintenance.Add)
}
