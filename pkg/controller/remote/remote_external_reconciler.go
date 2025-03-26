package remote

import (
	"reflect"

	"emperror.dev/errors"
	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/object"
	jsonIterator "github.com/json-iterator/go"
)

// RemoteExternalReconciler is the interface to call the remote API to handler resource
type RemoteExternalReconciler[k8sObject object.RemoteObject, apiObject comparable, apiClient any] interface {
	Build(k8sO k8sObject) (object apiObject, err error)
	Get(k8sO k8sObject) (object apiObject, err error)
	Create(apiO apiObject, k8sO k8sObject) (err error)
	Update(apiO apiObject, k8sO k8sObject) (err error)
	Delete(k8sO k8sObject) (err error)
	Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, k8sO k8sObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error)
	Client() apiClient
}

// DefaultRemoteExternalReconciler is the default implementation of RemoteExternalReconciler
// It only implement the Diff method, because of is generic with 3-way merge patch
type DefaultRemoteExternalReconciler[k8sObject object.RemoteObject, apiObject comparable, apiClient any] struct {
	client apiClient
}

func NewRemoteExternalReconciler[k8sObject object.RemoteObject, apiObject comparable, apiClient any](handler apiClient) RemoteExternalReconciler[k8sObject, apiObject, apiClient] {
	return &DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]{
		client: handler,
	}
}

func (h *DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Diff(currentOject apiObject, expectedObject apiObject, originalObject apiObject, o k8sObject, ignoresDiff ...patch.CalculateOption) (patchResult *patch.PatchResult, err error) {
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

func (h *DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Client() apiClient {
	return h.client
}

func (h *DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Build(k8sO k8sObject) (object apiObject, err error) {
	panic("You need to implement it")
}

func (h *DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Get(k8sO k8sObject) (object apiObject, err error) {
	panic("You need to implement it")
}

func (h *DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Create(apiO apiObject, k8sO k8sObject) (err error) {
	panic("You need to implement it")
}

func (h *DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Update(apiO apiObject, k8sO k8sObject) (err error) {
	panic("You need to implement it")
}

func (h *DefaultRemoteExternalReconciler[k8sObject, apiObject, apiClient]) Delete(k8sO k8sObject) (err error) {
	panic("You need to implement it")
}
