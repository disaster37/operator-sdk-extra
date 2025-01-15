package apis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"
)

func TestMapAny(t *testing.T) {
	o := &MapAny{
		Data: map[string]any{
			"foo": "bar",
		},
	}

	expectedO := &MapAny{
		Data: map[string]any{
			"foo": "bar",
		},
	}

	b, err := yaml.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "foo: bar\n", string(b))
	if err = yaml.Unmarshal(b, o); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, expectedO, o)
}
