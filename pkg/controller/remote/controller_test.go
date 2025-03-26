package remote_test

import (
	"context"
	"errors"
	"testing"
	"time"

	eshandler "github.com/disaster37/es-handler/v8"
	"github.com/disaster37/es-handler/v8/mocks"
	"github.com/disaster37/generic-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/test"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *ControllerRemoteTestSuite) TestTestReconciler() {
	key := types.NamespacedName{
		Name:      "t-test-" + helper.RandomString(10),
		Namespace: "default",
	}
	data := map[string]any{}

	testCase := test.NewTestCase[*RemoteObject](t.T(), t.k8sClient, key, 5*time.Second, data)
	testCase.Steps = []test.TestStep[*RemoteObject]{
		doCreateRemoteObjectStep(),
		doUpdateRemoteObjectStep(),
		doDeleteRemoteObjectStep(),
	}
	testCase.PreTest = doMockRemoteObject(t.mockElasticsearchHandler)

	testCase.Run()
}

// We need to mock the External API used for test purpose
// We take eshandler to avoid to create fake client to only test this framework
func doMockRemoteObject(mockES *mocks.MockElasticsearchHandler) func(stepName *string, data map[string]any) error {
	return func(stepName *string, data map[string]any) (err error) {
		isCreated := false
		isUpdated := false

		mockES.EXPECT().RoleGet(gomock.Any()).AnyTimes().DoAndReturn(func(name string) (*eshandler.XPackSecurityRole, error) {
			switch *stepName {
			case "create":
				if !isCreated {
					return nil, nil
				} else {

					resp := &eshandler.XPackSecurityRole{}
					return resp, nil
				}
			case "update":
				if !isUpdated {
					resp := &eshandler.XPackSecurityRole{}
					return resp, nil
				} else {
					resp := &eshandler.XPackSecurityRole{}
					return resp, nil
				}
			}

			return nil, nil
		})

		mockES.EXPECT().RoleDiff(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(actual, expected, original *eshandler.XPackSecurityRole) (*patch.PatchResult, error) {
			switch *stepName {
			case "create":
				if !isCreated {
					return &patch.PatchResult{
						Patch: []byte("fake change"),
					}, nil
				} else {
					return &patch.PatchResult{}, nil
				}
			case "update":
				if !isUpdated {
					return &patch.PatchResult{
						Patch: []byte("fake change"),
					}, nil
				} else {
					return &patch.PatchResult{}, nil
				}
			}

			return nil, nil
		})

		mockES.EXPECT().RoleUpdate(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(name string, policy *eshandler.XPackSecurityRole) error {
			switch *stepName {
			case "create":
				isCreated = true
				data["isCreated"] = true
				return nil
			case "update":
				isUpdated = true
				data["isUpdated"] = true
				return nil
			}

			return nil
		})

		mockES.EXPECT().RoleDelete(gomock.Any()).AnyTimes().DoAndReturn(func(name string) error {
			data["isDeleted"] = true
			return nil
		})

		return nil
	}
}

func doCreateRemoteObjectStep() test.TestStep[*RemoteObject] {
	return test.TestStep[*RemoteObject]{
		Name: "create",
		Do: func(c client.Client, key types.NamespacedName, o *RemoteObject, data map[string]any) (err error) {
			logrus.Infof("=== Add new ClientObject %s/%s ===\n\n", key.Namespace, key.Name)

			r := &RemoteObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: RemoteObjectSpec{
					Test: "test",
				},
			}
			if err = c.Create(context.Background(), r); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *RemoteObject, data map[string]any) (err error) {
			isCreated := false
			o = &RemoteObject{}

			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, o); err != nil {
					t.Fatal(err)
				}
				if b, ok := data["isCreated"]; ok {
					isCreated = b.(bool)
				}
				if !isCreated || o.GetStatus().GetObservedGeneration() == 0 {
					return errors.New("Not yet created")
				}
				return nil
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("Failed to get remoteRole: %s", err.Error())
			}
			assert.True(t, condition.IsStatusConditionPresentAndEqual(o.Status.Conditions, controller.ReadyCondition.String(), metav1.ConditionTrue))
			assert.True(t, *o.Status.IsSync)
			assert.NotEmpty(t, o.Status.LastAppliedConfiguration)
			assert.NotEmpty(t, o.Status.ObservedGeneration)
			assert.False(t, *o.Status.IsOnError)

			return nil
		},
	}
}

func doUpdateRemoteObjectStep() test.TestStep[*RemoteObject] {
	return test.TestStep[*RemoteObject]{
		Name: "update",
		Do: func(c client.Client, key types.NamespacedName, o *RemoteObject, data map[string]any) (err error) {
			logrus.Infof("=== Update RemoteObject %s/%s ===\n\n", key.Namespace, key.Name)

			if o == nil {
				return errors.New("RemoteObject is null")
			}

			data["lastGeneration"] = o.GetStatus().GetObservedGeneration()
			o.Spec.Test = "test2"
			if err = c.Update(context.Background(), o); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *RemoteObject, data map[string]any) (err error) {
			isUpdated := false

			lastGeneration := data["lastGeneration"].(int64)

			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, o); err != nil {
					t.Fatal(err)
				}
				if b, ok := data["isUpdated"]; ok {
					isUpdated = b.(bool)
				}
				if !isUpdated || lastGeneration == o.GetStatus().GetObservedGeneration() {
					return errors.New("Not yet updated")
				}
				return nil
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("Failed to get RemoteObject: %s", err.Error())
			}
			assert.True(t, condition.IsStatusConditionPresentAndEqual(o.Status.Conditions, controller.ReadyCondition.String(), metav1.ConditionTrue))
			assert.True(t, *o.Status.IsSync)
			assert.NotEmpty(t, o.Status.LastAppliedConfiguration)
			assert.NotEmpty(t, o.Status.ObservedGeneration)
			assert.False(t, *o.Status.IsOnError)

			return nil
		},
	}
}

func doDeleteRemoteObjectStep() test.TestStep[*RemoteObject] {
	return test.TestStep[*RemoteObject]{
		Name: "delete",
		Do: func(c client.Client, key types.NamespacedName, o *RemoteObject, data map[string]any) (err error) {
			logrus.Infof("=== Delete RemoteObject %s/%s ===\n\n", key.Namespace, key.Name)

			if o == nil {
				return errors.New("RemoteObject is null")
			}

			if err = c.Delete(context.Background(), o, &client.DeleteOptions{GracePeriodSeconds: ptr.To(int64(0))}); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *RemoteObject, data map[string]any) (err error) {
			isDeleted := false

			isTimeout, err := test.RunWithTimeout(func() error {
				if err = c.Get(context.Background(), key, o); err != nil {
					if k8serrors.IsNotFound(err) {
						isDeleted = true
						return nil
					}
					t.Fatal(err)
				}

				return errors.New("Not yet deleted")
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("RemoteObject stil exist: %s", err.Error())
			}
			assert.True(t, isDeleted)

			return nil
		},
	}
}
