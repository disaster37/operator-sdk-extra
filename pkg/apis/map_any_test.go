package apis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func TestMapAny(t *testing.T) {
	o := &MapAny{
		Data: map[string]any{
			"foo": "bar",
		},
	}

	// Test Marshal / Unmarshall
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

	// Test clone
	assert.Equal(t, o, o.DeepCopy())

	o2 := &MapAny{}
	o.DeepCopyInto(o2)
	assert.Equal(t, o, o2)
}

func TestMapAnyDeepCopyNilReceiver(t *testing.T) {
	var m *MapAny = nil
	assert.Nil(t, m.DeepCopy())
}

func TestMapAnyUnmarshalJSONError(t *testing.T) {
	m := &MapAny{}
	err := m.UnmarshalJSON([]byte("invalid json {{"))
	assert.Error(t, err)
}

func TestDefaultObjectStatusDeepCopy(t *testing.T) {
	trueVal := true
	status := &DefaultObjectStatus{
		IsOnError: &trueVal,
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue},
			{Type: "Progressing", Status: metav1.ConditionFalse},
		},
	}

	copy := status.DeepCopy()
	assert.NotNil(t, copy)
	assert.Equal(t, *status.IsOnError, *copy.IsOnError)
	assert.Equal(t, len(status.Conditions), len(copy.Conditions))
	assert.Equal(t, status.Conditions[0].Type, copy.Conditions[0].Type)
	assert.Equal(t, status.Conditions[1].Status, copy.Conditions[1].Status)

	// Modify original to verify deep copy is independent
	*status.IsOnError = false
	status.Conditions[0].Status = metav1.ConditionUnknown
	assert.NotEqual(t, *status.IsOnError, *copy.IsOnError)
	assert.NotEqual(t, status.Conditions[0].Status, copy.Conditions[0].Status)
}

func TestDefaultObjectStatusDeepCopyNilReceiver(t *testing.T) {
	var s *DefaultObjectStatus = nil
	assert.Nil(t, s.DeepCopy())
}
