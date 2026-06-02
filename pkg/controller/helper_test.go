package controller

import (
	"errors"
	"strings"
	"testing"

	emperrors "emperror.dev/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestGetObjectMeta(t *testing.T) {
	meta := v1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
	}

	o := &corev1.ConfigMap{
		ObjectMeta: meta,
	}

	assert.Equal(t, meta, GetObjectMeta(o))

	assert.Panics(t, func() {
		GetObjectMeta(nil)
	})

	var test struct {
		client.Object
	}

	assert.Panics(t, func() {
		GetObjectMeta(test)
	})

	test2 := &struct{ client.Object }{}

	assert.Panics(t, func() {
		GetObjectMeta(test2)
	})
}

func TestGetObjectStatus(t *testing.T) {
	status := corev1.PodStatus{
		Message: "test",
	}
	o := &corev1.Pod{
		Status: status,
	}

	assert.Equal(t, status, GetObjectStatus(o))

	assert.Panics(t, func() {
		GetObjectStatus(nil)
	})

	var test struct {
		client.Object
	}

	assert.Panics(t, func() {
		GetObjectStatus(test)
	})

	test2 := &struct{ client.Object }{}

	assert.Panics(t, func() {
		GetObjectStatus(test2)
	})
}

func TestMustInjectTypeMeta(t *testing.T) {
	meta := v1.TypeMeta{
		Kind: "ConfigMap",
	}
	src := &corev1.ConfigMap{
		TypeMeta: meta,
	}

	dst := &corev1.ConfigMap{}

	MustInjectTypeMeta(src, dst)
	assert.Equal(t, src, dst)

	assert.Panics(t, func() {
		MustInjectTypeMeta(nil, nil)
	})

	var test struct {
		client.Object
	}

	assert.Panics(t, func() {
		MustInjectTypeMeta(test, dst)
	})
	assert.Panics(t, func() {
		MustInjectTypeMeta(src, test)
	})

	test2 := &struct{ client.Object }{}

	assert.Panics(t, func() {
		MustInjectTypeMeta(test2, dst)
	})
	assert.Panics(t, func() {
		MustInjectTypeMeta(src, test2)
	})
}

func TestDefaultControllerRateLimiter(t *testing.T) {
	rateLimiter := DefaultControllerRateLimiter[reconcile.Request]()
	assert.NotNil(t, rateLimiter)
}

func TestUserFacingError(t *testing.T) {
	rootErr := errors.New("connection refused")
	wrapped1 := emperrors.Wrap(rootErr, "read configmap")
	wrapped2 := emperrors.Wrap(wrapped1, "reconcile step Configmap")

	// Extracts deepest cause, ignoring wrapper layers
	got := UserFacingError(wrapped2, MaxConditionMessage)
	assert.Equal(t, "connection refused", got, "expected root cause message")

	// Simple error without wrapping returns its own message
	got = UserFacingError(rootErr, MaxConditionMessage)
	assert.Equal(t, "connection refused", got)

	// nil error returns empty string
	got = UserFacingError(nil, MaxConditionMessage)
	assert.Equal(t, "", got)

	// Long message gets truncated with "..." suffix
	longMsg := strings.Repeat("a", 200)
	longErr := errors.New(longMsg)
	got = UserFacingError(longErr, 20)
	assert.Equal(t, 20, len(got))
	assert.True(t, strings.HasSuffix(got, "..."), "truncated message should end with '...'")
	assert.Equal(t, strings.Repeat("a", 17)+"...", got)

	// Long wrapping chain: extracts root cause even when that cause itself is long
	longCause := strings.Repeat("b", 200)
	wrapped := emperrors.Wrap(errors.New(longCause), "outer context ignored")
	got = UserFacingError(wrapped, 20)
	assert.Equal(t, strings.Repeat("b", 17)+"...", got,
		"should extract and truncate the root cause, not the wrapper")

	// maxLen smaller than 3: truncates without "..."
	got = UserFacingError(longErr, 2)
	assert.Equal(t, "aa", got)

	// Message exactly at maxLen is not truncated
	exact := strings.Repeat("c", 20)
	got = UserFacingError(errors.New(exact), 20)
	assert.Equal(t, exact, got)

	// Message shorter than maxLen is not truncated
	got = UserFacingError(errors.New("tiny"), 100)
	assert.Equal(t, "tiny", got)
}
