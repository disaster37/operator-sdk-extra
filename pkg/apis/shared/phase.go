package shared

const (
	RunningPhase  PhaseName = "running"
	StartingPhase PhaseName = "starting"
)

// PhaseName is the the current phase name (step) on controller
type PhaseName string

// String return the phase name as string
func (o PhaseName) String() string {
	return string(o)
}
