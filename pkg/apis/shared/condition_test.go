package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConditionString(t *testing.T) {
	var o ConditionName = "test"

	assert.Equal(t, "test", o.String())
}
