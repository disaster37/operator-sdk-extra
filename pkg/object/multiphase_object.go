package object

import (
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis/shared"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiPhaseObject is used when your CRD is used to create multiple K8s resources
type MultiPhaseObject interface {
	client.Object

	// GetStatus permit to get the Status interface
	GetStatus() MultiPhaseObjectStatus
}

// MultiPhaseObjectStatus is the interface to control status of multi phase object
type MultiPhaseObjectStatus interface {
	ObjectStatus

	// GetPhaseName permit to get the current phase name
	GetPhaseName() shared.PhaseName

	// SetPhaseName permit to set the current phase
	SetPhaseName(name shared.PhaseName)
}
