package workflow_test

import (
	"context"
	"testing"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/apis/shared"
	apworkflow "github.com/disaster37/operator-sdk-extra/v3/pkg/apis/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller/workflow"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// testWorkflowStatus is a test implementation of MultiPhaseObjectStatus
// that also implements apworkflow.WorkflowStatusGetter.
type testWorkflowStatus struct {
	Conditions []metav1.Condition
	PhaseName  shared.PhaseName
	IsOnError  *bool
	LastError  string
	ObsGen     int64
	Ws         *apworkflow.WorkflowStatus
}

func (s *testWorkflowStatus) GetWorkflowStatus() *apworkflow.WorkflowStatus {
	if s.Ws == nil {
		s.Ws = &apworkflow.WorkflowStatus{}
	}
	return s.Ws
}

func (s *testWorkflowStatus) GetConditions() []metav1.Condition                   { return s.Conditions }
func (s *testWorkflowStatus) SetConditions(c []metav1.Condition)                  { s.Conditions = c }
func (s *testWorkflowStatus) GetIsOnError() bool                                  { return s.IsOnError != nil && *s.IsOnError }
func (s *testWorkflowStatus) SetIsOnError(b bool)                                 { s.IsOnError = &b }
func (s *testWorkflowStatus) GetLastErrorMessage() string                         { return s.LastError }
func (s *testWorkflowStatus) SetLastErrorMessage(m string)                        { s.LastError = m }
func (s *testWorkflowStatus) GetObservedGeneration() int64                        { return s.ObsGen }
func (s *testWorkflowStatus) SetObservedGeneration(v int64)                       { s.ObsGen = v }
func (s *testWorkflowStatus) GetPhaseName() shared.PhaseName                      { return s.PhaseName }
func (s *testWorkflowStatus) SetPhaseName(n shared.PhaseName)                     { s.PhaseName = n }

type testMultiPhaseObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status testWorkflowStatus
}

func (o *testMultiPhaseObject) GetStatus() object.MultiPhaseObjectStatus {
	return &o.Status
}

func (o *testMultiPhaseObject) DeepCopyObject() runtime.Object {
	return &testMultiPhaseObject{
		TypeMeta:   o.TypeMeta,
		ObjectMeta: *o.ObjectMeta.DeepCopy(),
		Status:     o.Status,
	}
}

// compile-time checks
var (
	_ object.MultiPhaseObject       = (*testMultiPhaseObject)(nil)
	_ object.MultiPhaseObjectStatus = (*testWorkflowStatus)(nil)
)

func TestNewWorkflowStepReconcilerAction(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := workflow.NewWorkflowStepReconcilerAction[*testMultiPhaseObject, *corev1.Secret](
		c,
		"test-phase",
		"test-condition",
		recorder,
		"test-manager",
		true,
	)

	require.NotNil(t, action)
	assert.Equal(t, shared.PhaseName("test-phase"), action.GetPhaseName())
}

func TestCurrentPhase(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := workflow.NewWorkflowStepReconcilerAction[*testMultiPhaseObject, *corev1.Secret](
		c, "test-phase", "test-condition", recorder, "test-manager", true,
	)

	o := &testMultiPhaseObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status:     testWorkflowStatus{Ws: &apworkflow.WorkflowStatus{}},
	}

	assert.True(t, action.IsPhaseEmpty(o))
	assert.Equal(t, apworkflow.WorkflowPhase(""), action.CurrentPhase(o))

	action.AdvancePhase(context.Background(), o, "phase-1", logrus.NewEntry(logrus.StandardLogger()))
	assert.Equal(t, apworkflow.WorkflowPhase("phase-1"), action.CurrentPhase(o))
	assert.False(t, action.IsPhaseEmpty(o))
	assert.True(t, action.IsPhase(o, "phase-1"))
	assert.False(t, action.IsPhase(o, "phase-2"))
}

func TestAdvancePhase(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := workflow.NewWorkflowStepReconcilerAction[*testMultiPhaseObject, *corev1.Secret](
		c, "test-phase", "test-condition", recorder, "test-manager", true,
	)

	o := &testMultiPhaseObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status:     testWorkflowStatus{Ws: &apworkflow.WorkflowStatus{}},
	}

	res, err := action.AdvancePhase(context.Background(), o, "phase-2", logrus.NewEntry(logrus.StandardLogger()))
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)
	assert.True(t, action.IsPhase(o, "phase-2"))
}

func TestIsPhaseEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := workflow.NewWorkflowStepReconcilerAction[*testMultiPhaseObject, *corev1.Secret](
		c, "test-phase", "test-condition", recorder, "test-manager", true,
	)

	o := &testMultiPhaseObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	assert.True(t, action.IsPhaseEmpty(o))

	o.Status.Ws = &apworkflow.WorkflowStatus{}
	assert.True(t, action.IsPhaseEmpty(o))

	action.AdvancePhase(context.Background(), o, "phase-1", logrus.NewEntry(logrus.StandardLogger()))
	assert.False(t, action.IsPhaseEmpty(o))
}

func TestWorkflowStatusGetterIntegration(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	action := workflow.NewWorkflowStepReconcilerAction[*testMultiPhaseObject, *corev1.Secret](
		c, "test-phase", "test-condition", recorder, "test-manager", true,
	)

	o := &testMultiPhaseObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status:     testWorkflowStatus{Ws: &apworkflow.WorkflowStatus{}},
	}

	// Verify that workflow phase changes are reflected through the status getter
	o.Status.Ws.Advance("custom-phase")
	assert.True(t, action.IsPhase(o, "custom-phase"))
	assert.Equal(t, apworkflow.WorkflowPhase("custom-phase"), action.CurrentPhase(o))
}
