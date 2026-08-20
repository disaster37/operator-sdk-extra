package helper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/util/workqueue"
)

func TestDefaultControllerRateLimiterUnit(t *testing.T) {
	t.Run("returns a non-nil rate limiter", func(t *testing.T) {
		limiter := DefaultControllerRateLimiter[string]()
		assert.NotNil(t, limiter)
	})

	t.Run("returns correct type", func(t *testing.T) {
		limiter := DefaultControllerRateLimiter[string]()
		assert.IsType(t, &workqueue.TypedMaxOfRateLimiter[string]{}, limiter)
	})

	t.Run("rate limiter works correctly", func(t *testing.T) {
		limiter := DefaultControllerRateLimiter[string]()

		item := "test-item"

		delay := limiter.When(item)
		assert.NotNil(t, delay)

		limiter.Forget(item)
		assert.Equal(t, 0, limiter.NumRequeues(item))

		delay = limiter.When(item)
		assert.NotNil(t, delay)
		delay2 := limiter.When(item)
		assert.GreaterOrEqual(t, delay2, delay)

		limiter.Forget(item)
		assert.Equal(t, 0, limiter.NumRequeues(item))
	})
}
