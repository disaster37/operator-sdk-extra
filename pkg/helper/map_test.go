package helper

import (
	"testing"

	"emperror.dev/errors"
	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	data := map[string]any{
		"test": "foo bar",
	}

	// When key exist
	d, err := Get(data, "test")
	assert.NoError(t, err)
	assert.Equal(t, data["test"], d)

	// When key not exist
	_, err = Get(data, "fake")
	assert.Error(t, err)
	assert.EqualError(t, errors.Cause(err), ErrKeyNotFound.Error())
}
