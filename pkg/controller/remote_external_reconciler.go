package controller

import (
	"reflect"

	"emperror.dev/errors"
	"github.com/disaster37/generic-objectmatcher/patch"
	jsonIterator "github.com/json-iterator/go"
)

// RemoteExternalReconciler is the interface to call the remote API to handler resource
type RemoteExternalReconciler[k8sObject comparable, apiObject comparable, apiClient any] interface {
	Build(k8sO k8sObject) (object apiObject, err error)
	Get(k8sO k8sObject) (object apiObject, err error)
	Create(apiO apiObject, k8sO k8sObject) (err error)
	Update(apiO apiObject, k8sO k8sObject) (err error)
	Delete(k8sO k8sObject) (err error)
	Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error)
	Client() apiClient
}

// BasicRemoteExternalReconciler is the basic implementation of RemoteExternalReconciler
// It only implement the Diff method, because of is generic with 3-way merge patch
type BasicRemoteExternalReconciler[k8sObject comparable, apiObject comparable, apiClient any] struct {
	client apiClient
}

func NewBasicRemoteExternalReconciler[k8sObject comparable, apiObject comparable, apiClient any](handler apiClient) *BasicRemoteExternalReconciler[k8sObject, apiObject, apiClient] {
	return &BasicRemoteExternalReconciler[k8sObject, apiObject, apiClient]{
		client: handler,
	}
}

func (h *BasicRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error) {
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

func (h *BasicRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Client() apiClient {
	return h.client
}
