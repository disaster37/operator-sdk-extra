package helper

import (
	"testing"

	"dario.cat/mergo"
	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	Name  string
	Value int
}

func TestMergeUnit(t *testing.T) {
	t.Run("dst is not a pointer - should return ErrNonPointerArgument", func(t *testing.T) {
		var dst testStruct
		src := &testStruct{Name: "updated", Value: 2}

		err := Merge(dst, src)
		assert.Error(t, err)
		assert.Equal(t, mergo.ErrNonPointerArgument, err)
	})

	t.Run("src is nil - should skip and not return error", func(t *testing.T) {
		dst := &testStruct{Name: "original", Value: 1}
		var src *testStruct = nil

		err := Merge(dst, src)
		assert.NoError(t, err)
	})

	t.Run("src is nil pointer - should skip and not return error", func(t *testing.T) {
		dst := &testStruct{Name: "original", Value: 1}
		var src *testStruct = (*testStruct)(nil)

		err := Merge(dst, src)
		assert.NoError(t, err)
	})

	t.Run("multiple sources with nil values", func(t *testing.T) {
		dst := &testStruct{Name: "original", Value: 1}
		var src2 *testStruct = nil

		err := Merge(dst, src2)
		assert.NoError(t, err)
	})

	t.Run("error during merge", func(t *testing.T) {
		type conflictingStruct struct {
			Name  string
			Value string
		}

		dst := &testStruct{}
		src := &conflictingStruct{}

		err := Merge(dst, src)
		assert.Error(t, err)
	})
}