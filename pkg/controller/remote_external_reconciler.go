package controller

import (
	"github.com/disaster37/k8s-objectmatcher/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RemoteExternalReconciler[T comparable] interface {
	Build(o client.Object) (object T, err error)
	Get(name string) (object T, err error)
	Create(object T) (err error)
	Update(object T) (err error)
	Delete(name string) (err error)
	Diff(currentOject T, expectedObject T, originalObject T, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error)
}
