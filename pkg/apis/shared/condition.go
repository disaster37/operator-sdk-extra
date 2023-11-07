package shared

// Condition is the condition name
type ConditionName string

// String return the condition name as string
func (o ConditionName) String() string {
	return string(o)
}
