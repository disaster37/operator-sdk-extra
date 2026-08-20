package multiphase

import (
	"context"

	ssadiff "github.com/disaster37/operator-sdk-extra/v3/pkg/helper/ssa"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanupReadLastAppliedAnnotations removes the legacy
// kubectl.kubernetes.io/last-applied-configuration annotation from all current
// objects in read that have a matching expected object (managed children that
// are not orphans). Orphans are skipped because they are about to be deleted.
//
// Best-effort: per-object cleanup failures are logged at warn level and do not
// abort the loop or return an error. Returns the number of objects cleaned.
func CleanupReadLastAppliedAnnotations[T client.Object](
	ctx context.Context,
	c client.Client,
	read MultiPhaseRead[T],
	logger *logrus.Entry,
) (cleaned int) {
	expectedNames := make(map[string]struct{}, len(read.GetExpectedObjects()))
	for _, o := range read.GetExpectedObjects() {
		expectedNames[o.GetName()] = struct{}{}
	}

	for _, live := range read.GetCurrentObjects() {
		if _, ok := expectedNames[live.GetName()]; !ok {
			continue // orphan, will be deleted
		}
		removed, err := ssadiff.CleanupLastAppliedAnnotation(ctx, c, live, logger)
		if err != nil {
			logger.Warnf("Failed to clean last-applied-configuration annotation on object '%s': %s (continuing)", live.GetName(), err.Error())
			continue
		}
		if removed {
			cleaned++
		}
	}
	return cleaned
}
