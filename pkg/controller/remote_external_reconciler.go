package controller

import (
	"reflect"

	"emperror.dev/errors"
	"github.com/disaster37/generic-objectmatcher/patch"
	jsonIterator "github.com/json-iterator/go"
)

// RemoteExternalReconciler is the interface to call the remote API to handler resource
type RemoteExternalReconciler[k8sObject comparable, apiObject comparable] interface {
	Build(k8sO k8sObject) (object apiObject, err error)
	Get(k8sO k8sObject) (object apiObject, err error)
	Create(apiO apiObject, k8sO k8sObject) (err error)
	Update(apiO apiObject, k8sO k8sObject) (err error)
	Delete(k8sO k8sObject) (err error)
	Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error)
	Custom(f func(handler any) error) (err error)
}

// BasicRemoteExternalReconciler is the basic implementation of RemoteExternalReconciler
// It only implement the Diff method, because of is generic with 3-way merge patch
type BasicRemoteExternalReconciler[k8sObject comparable, apiObject comparable, externalHandler any] struct {
	handler externalHandler
}

func NewBasicRemoteExternalReconciler[k8sObject comparable, apiObject comparable, externalHandler any](handler externalHandler) *BasicRemoteExternalReconciler[k8sObject, apiObject, externalHandler] {
	return &BasicRemoteExternalReconciler[k8sObject, apiObject, externalHandler]{
		handler: handler,
	}
}

func (h *BasicRemoteExternalReconciler[k8sObject, apiObject, externalHandler]) Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error) {
	if reflect.ValueOf(currentOject).IsNil() {
		expected, err := jsonIterator.ConfigCompatibleWithStandardLibrary.Marshal(expectedObject)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to convert expected object to byte sequence")
		}

		return &patch.PatchResult{
			Patch:    expected,
			Current:  expected,
			Modified: expected,
			Original: nil,
			Patched:  expectedObject,
		}, nil
	}

	return patch.DefaultPatchMaker.Calculate(currentOject, expectedObject, originalObject, ignoresDiff...)
}

func (h *BasicRemoteExternalReconciler[k8sObject, apiObject, externalHandler]) Custom(f func(handler any) error) (err error) {
	return f(h.handler)
}
