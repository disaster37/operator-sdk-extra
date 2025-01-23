package multiphase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/v2/pkg/test"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *ControllerMultiphaseTestSuite) TestController() {
	key := types.NamespacedName{
		Name:      "t-test-" + helper.RandomString(10),
		Namespace: "default",
	}
	data := map[string]any{}

	testCase := test.NewTestCase[*MultiPhaseObject](t.T(), t.k8sClient, key, 5*time.Second, data)
	testCase.Steps = []test.TestStep[*MultiPhaseObject]{
		doCreateStep(),
		doUpdateStep(),
		doDeleteStep(),
	}

	testCase.Run()
}

func doCreateStep() test.TestStep[*MultiPhaseObject] {
	return test.TestStep[*MultiPhaseObject]{
		Name: "create",
		Do: func(c client.Client, key types.NamespacedName, o *MultiPhaseObject, data map[string]any) (err error) {
			logrus.Infof("=== Add new MultiphaseObject %s/%s ===\n\n", key.Namespace, key.Name)

			m := &MultiPhaseObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: MultiPhaseObjectSpec{
					Test: "test",
				},
			}

			if err = c.Create(context.Background(), m); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *MultiPhaseObject, data map[string]any) (err error) {
			o = &MultiPhaseObject{}
			var cm *corev1.ConfigMap

			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, o); err != nil {
					t.Fatal("MultiPhaseObject not found")
				}

				if o.GetStatus().GetObservedGeneration() > 0 {
					return nil
				}

				return errors.New("Not yet created")
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("All MultiPhaseObject step provisionning not finished: %s", err.Error())
			}

			// ConfigMaps must exist
			cm = &corev1.ConfigMap{}
			if err = c.Get(context.Background(), types.NamespacedName{Namespace: key.Namespace, Name: key.Name}, cm); err != nil {
				t.Fatal(err)
			}
			assert.NotEmpty(t, cm.OwnerReferences)
			assert.NotEmpty(t, cm.Annotations[patch.LastAppliedConfig])

			return nil
		},
	}
}

func doUpdateStep() test.TestStep[*MultiPhaseObject] {
	return test.TestStep[*MultiPhaseObject]{
		Name: "update",
		Do: func(c client.Client, key types.NamespacedName, o *MultiPhaseObject, data map[string]any) (err error) {
			logrus.Infof("=== Update MultiPhaseObject %s/%s ===\n\n", key.Namespace, key.Name)

			if o == nil {
				return errors.New("MultiphaseObject is null")
			}

			// Add labels must force to update all resources
			o.Labels = map[string]string{
				"test": "fu",
			}

			// Change spec to track generation
			o.Spec.Test = "test2"

			data["lastGeneration"] = o.GetStatus().GetObservedGeneration()

			if err = c.Update(context.Background(), o); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *MultiPhaseObject, data map[string]any) (err error) {
			var cm *corev1.ConfigMap

			lastGeneration := data["lastGeneration"].(int64)

			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, o); err != nil {
					t.Fatal("MultiPhaseObject not found")
				}

				if lastGeneration < o.GetStatus().GetObservedGeneration() {
					return nil
				}

				return errors.New("Not yet updated")
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("All MultiPhaseObject step upgrading not finished: %s", err.Error())
			}

			// ConfigMaps must exist
			cm = &corev1.ConfigMap{}
			if err = c.Get(context.Background(), types.NamespacedName{Namespace: key.Namespace, Name: key.Name}, cm); err != nil {
				t.Fatal(err)
			}
			assert.NotEmpty(t, cm.OwnerReferences)
			assert.NotEmpty(t, cm.Annotations[patch.LastAppliedConfig])
			assert.Equal(t, "fu", cm.Labels["test"])

			return nil
		},
	}
}

func doDeleteStep() test.TestStep[*MultiPhaseObject] {
	return test.TestStep[*MultiPhaseObject]{
		Name: "delete",
		Do: func(c client.Client, key types.NamespacedName, o *MultiPhaseObject, data map[string]any) (err error) {
			logrus.Infof("=== Delete MultiPhaseObject cluster %s/%s ===\n\n", key.Namespace, key.Name)

			if o == nil {
				return errors.New("MultiPhaseObject is null")
			}

			if err = c.Delete(context.Background(), o, &client.DeleteOptions{GracePeriodSeconds: ptr.To(int64(0))}); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *MultiPhaseObject, data map[string]any) (err error) {
			isDeleted := false

			// In envtest, no kubelet
			// So the cascading children delation not works
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
				t.Fatalf("MultiPhaseObject stil exist: %s", err.Error())
			}

			assert.True(t, isDeleted)

			return nil
		},
	}
}
