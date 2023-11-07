package helper

import "emperror.dev/errors"

var ErrKeyNotFound error = errors.Sentinel("Key not found in map")

// Get permit to get value on from from key
// It return error if key not exist
func Get(m map[string]any, key string) (value any, err error) {
	d, ok := m[key]
	if !ok {
		return nil, errors.Wrapf(ErrKeyNotFound, "key %s not found", key)
	}

	return d, nil
}
