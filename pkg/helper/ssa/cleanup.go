package ssa

import (
	"context"
	"encoding/json"

	"emperror.dev/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// lastAppliedConfigCleanupPatch is the JSON merge patch that nulls the legacy
// last-applied-configuration annotation, deleting the key regardless of which
// field manager owns it. json.Marshal cannot fail for this fixed map, so the
// error is discarded.
var lastAppliedConfigCleanupPatch, _ = json.Marshal(map[string]any{
	"metadata": map[string]any{
		"annotations": map[string]any{
			lastAppliedConfigAnnotation: nil,
		},
	},
})

// CleanupLastAppliedAnnotation removes the legacy
// kubectl.kubernetes.io/last-applied-configuration annotation from the live
// object represented by liveObj if present.
//
// It issues a JSON merge patch (MergePatchType) that sets the annotation key
// to null, which deletes the key regardless of which field manager owns it.
// This is necessary because SSA cannot delete a map key the applying field
// manager does not own, and the v3 field manager never owned this key.
//
// liveObj must reflect the current live server state: its annotations are
// inspected to decide whether a patch is needed. liveObj is not mutated.
//
// Idempotent: no-op (returns removed=false, nil) when the annotation is
// absent, when liveObj has a deletionTimestamp, or when liveObj has no
// annotations. Does not use SSA and does not require a fieldManager.
func CleanupLastAppliedAnnotation(
	ctx context.Context,
	c client.Client,
	liveObj client.Object,
	logger *logrus.Entry,
) (removed bool, err error) {
	if liveObj.GetDeletionTimestamp() != nil {
		return false, nil
	}
	if _, ok := liveObj.GetAnnotations()[lastAppliedConfigAnnotation]; !ok {
		return false, nil
	}

	if err = c.Patch(ctx, liveObj, client.RawPatch(types.MergePatchType, lastAppliedConfigCleanupPatch)); err != nil {
		return false, errors.Wrapf(err, "Error when removing last-applied-configuration annotation on object '%s'", liveObj.GetName())
	}

	logger.Debugf("Removed legacy last-applied-configuration annotation from object '%s'", liveObj.GetName())
	return true, nil
}
