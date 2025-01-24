package v1alpha1

import "github.com/disaster37/operator-sdk-extra/v2/pkg/object"

// GetStatus return the status object
func (o *Role) GetStatus() object.RemoteObjectStatus {
	return &o.Status
}

// GetExternalName return the role name
// If name is empty, it use the ressource name
func (o *Role) GetExternalName() string {
	if o.Spec.Name == "" {
		return o.Name
	}

	return o.Spec.Name
}
