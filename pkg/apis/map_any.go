package apis

import (
	"encoding/json"
)

// MapAny is YAML map[string]any representation
// +kubebuilder:validation:Type=object
type MapAny struct {
	// Data is the map[string]any contend
	Data map[string]any `json:"-"`
}

// MarshalJSON implements the Marshaler interface.
func (ma MapAny) MarshalJSON() ([]byte, error) {
	return json.Marshal(ma.Data)
}

// UnmarshalJSON implements the Unmarshaler interface.
func (ma *MapAny) UnmarshalJSON(data []byte) error {
	d := map[string]any{}
	err := json.Unmarshal(data, &d)
	if err != nil {
		return err
	}
	ma.Data = d
	return nil
}

// DeepCopyInto is needed by controller
func (ma *MapAny) DeepCopyInto(maCopy *MapAny) {
	bytes, err := json.Marshal(ma.Data)
	if err != nil {
		panic(err)
	}
	clone := &map[string]any{}
	err = json.Unmarshal(bytes, clone)
	if err != nil {
		panic(err)
	}
	maCopy.Data = *clone
}
