package controller

import (
	"github.com/disaster37/generic-objectmatcher/patch"
)

type RemoteExternalReconciler[O comparable, T comparable] interface {
	Build(o O) (object T, err error)
	Get(name string) (object T, err error)
	Create(object T) (err error)
	Update(object T) (err error)
	Delete(name string) (err error)
	Diff(currentOject T, expectedObject T, originalObject T, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error)
}
