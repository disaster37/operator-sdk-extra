package object

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseObject is used when your CRD is used to create multiple K8s resources
type RemoteObject interface {
	client.Object

	GetStatus() RemoteObjectStatus
}

// RemoteObject is use when your CRD is used to call remote API (not create K8s resources)
type RemoteObjectStatus interface {
	ObjectStatus

	// GetIsSync permit to get if object is sync from status
	GetIsSync() bool

	// SetIsSync permit to set if object is sync from status
	SetIsSync(isSync bool)

	// GetLastAppliedConfiguration permit to get the original object from annotations (like kubectl do)
	// The goal is to apply 3 way patch merge
	GetLastAppliedConfiguration() string

	// SetLastAppliedConfiguration permit to set the original object from annotations (like kubectl do)
	// The goal is to apply 3 way patch merge
	SetLastAppliedConfiguration(object string)
}
