package shared

// FinalizerName is the finalizer name
type FinalizerName string

// String return the finalizer name as string
func (o FinalizerName) String() string {
	return string(o)
}
