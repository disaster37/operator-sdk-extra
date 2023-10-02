package controller

import (
	"github.com/disaster37/generic-objectmatcher/patch"
)

// RemoteExternalReconciler is the interface to call the remote API to handler resource
type RemoteExternalReconciler[k8sObject comparable, apiObject comparable] interface {
	Build(o k8sObject) (object apiObject, err error)
	Get(name string) (object apiObject, err error)
	Create(object apiObject) (err error)
	Update(object apiObject) (err error)
	Delete(name string) (err error)
	Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error)
}
