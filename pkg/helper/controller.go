package helper

import (
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

// DefaultControllerRateLimiter is default rate limit to avoid burst the kubeapi-server
func DefaultControllerRateLimiter[T comparable]() workqueue.TypedRateLimiter[T] {
	return workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[T](1*time.Second, 1000*time.Second),
		&workqueue.TypedBucketRateLimiter[T]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}
