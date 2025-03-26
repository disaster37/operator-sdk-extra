package helper

import (
	"testing"

	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestDefaultControllerRateLimiter(t *testing.T) {
	limiter := controller.DefaultControllerRateLimiter[reconcile.Request]()
	assert.NotNil(t, limiter)
}
