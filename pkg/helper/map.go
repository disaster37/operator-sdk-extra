package helper

import "github.com/pkg/errors"

// Get permit to get value on from from key
// It return error if key not exist
func Get(m map[string]any, key string) (value any, err error) {
	d, ok := m[key]
	if !ok {
		return nil, errors.Errorf("key %s not found", key)
	}

	return d, nil
}
