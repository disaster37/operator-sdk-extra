package ssa

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// DryRunApply performs an SSA apply with DryRunAll to get the predicted object
// from the API server without persisting changes. The input obj is deep-copied
// to avoid mutation.
func DryRunApply(ctx context.Context, c client.Client, obj client.Object, fieldManager string) (*unstructured.Unstructured, error) {
	copiedObj, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return nil, errors.New("DeepCopyObject did not return a client.Object")
	}
	copied := copiedObj
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(copied)
	if err != nil {
		return nil, err
	}
	predicted := &unstructured.Unstructured{Object: u}
	predicted.SetManagedFields(nil)
	predicted.SetResourceVersion("")

	if err := c.Patch(ctx, predicted, client.Apply, client.DryRunAll, client.FieldOwner(fieldManager), client.ForceOwnership); err != nil {
		return nil, err
	}

	return predicted, nil
}

// normalizeMap strips API-server-generated fields and the last-applied-configuration
// annotation from a raw unstructured map, allowing two objects to be meaningfully
// compared with cmp.Diff. It mutates the map in place.
func normalizeMap(obj map[string]interface{}) {
	unstructured.RemoveNestedField(obj, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(obj, "metadata", "generation")
	unstructured.RemoveNestedField(obj, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj, "metadata", "uid")
	unstructured.RemoveNestedField(obj, "status")

	// Strip last-applied-configuration annotation. GetAnnotations returns a
	// copy, so we work directly on the nested field.
	if annRaw, found, _ := unstructured.NestedFieldNoCopy(obj, "metadata", "annotations"); found {
		if ann, ok := annRaw.(map[string]interface{}); ok {
			delete(ann, lastAppliedConfigAnnotation)
			if len(ann) == 0 {
				unstructured.RemoveNestedField(obj, "metadata", "annotations")
			}
		}
	}
}

// Normalize applies normalizeMap to an unstructured object.
func Normalize(u *unstructured.Unstructured) {
	normalizeMap(u.Object)
}

// IsObjectDiff compares current and predicted objects after normalization.
// The predicted parameter should come from DryRunApply.
func IsObjectDiff(current client.Object, predicted *unstructured.Unstructured) (changed bool, human string, err error) {
	cu, err := runtime.DefaultUnstructuredConverter.ToUnstructured(current)
	if err != nil {
		return false, "", err
	}
	normalizeMap(cu)
	normalizeMap(predicted.Object)

	diff := cmp.Diff(cu, predicted.Object)
	if diff == "" {
		return false, "", nil
	}
	return true, diff, nil
}
