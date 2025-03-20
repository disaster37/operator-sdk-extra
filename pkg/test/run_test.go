package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRunWithTimeout(t *testing.T) {

	var (
		isTimeout bool
		err       error
	)

	// When no timeout
	isTimeout, err = RunWithTimeout(func() error {
		return nil
	}, 10*time.Second, 1*time.Millisecond)

	assert.NoError(t, err)
	assert.False(t, isTimeout)

	// When timeout
	isTimeout, err = RunWithTimeout(func() error {
		return errors.New("fake")
	}, 3*time.Second, 1*time.Millisecond)

	assert.Error(t, err)
	assert.True(t, isTimeout)

}
