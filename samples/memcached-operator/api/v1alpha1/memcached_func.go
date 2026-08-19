package v1alpha1

import "github.com/disaster37/operator-sdk-extra/v3/pkg/object"

func (h *Memcached) GetStatus() object.MultiPhaseObjectStatus {
	return &h.Status
}
