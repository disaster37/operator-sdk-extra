package sentinel_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func (t *ControllerSentinelTestSuite) TestController() {
	name := "t-test-" + helper.RandomString(10)
	key := types.NamespacedName{
		Name:      name,
		Namespace: name,
	}
	data := map[string]any{}

	testCase := test.NewTestCase[*corev1.Namespace](t.T(), t.k8sClient, key, 5*time.Second, data)
	testCase.Steps = []test.TestStep[*corev1.Namespace]{
		doCreateStep(),
		doUpdateStep(),
		doRemoveAnnotationStep(),
		doDeleteStep(),
	}

	testCase.Run()
}

func doCreateStep() test.TestStep[*corev1.Namespace] {
	return test.TestStep[*corev1.Namespace]{
		Name: "create",
		Do: func(c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {
			logrus.Infof("=== Add new SentinelObject %s/%s ===\n\n", key.Namespace, key.Name)

			o = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: key.Name,
					Annotations: map[string]string{
						annotation: "test",
					},
				},
			}

			if err = c.Create(context.Background(), o); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {
			s := &corev1.Secret{}
			cm := &corev1.ConfigMap{}

			// Check configMap
			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, cm); err != nil {
					if k8serrors.IsNotFound(err) {
						return errors.New("Not yet created")
					}
					t.Fatalf("Error when get ConfigMap: %s", err.Error())
				}
				return nil
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("Failed to get ConfigMap: %s", err.Error())
			}
			assert.Equal(t, map[string]string{"val": "test"}, cm.Data)
			assert.NotEmpty(t, cm.OwnerReferences)

			// Check secret
			isTimeout, err = test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, s); err != nil {
					if k8serrors.IsNotFound(err) {
						return errors.New("Not yet created")
					}
					t.Fatalf("Error when get Secret: %s", err.Error())
				}
				return nil
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("Failed to get Secret: %s", err.Error())
			}
			assert.Equal(t, map[string][]byte{"val": []byte("test")}, s.Data)
			assert.NotEmpty(t, s.OwnerReferences)

			return nil
		},
	}
}

func doUpdateStep() test.TestStep[*corev1.Namespace] {
	return test.TestStep[*corev1.Namespace]{
		Name: "update",
		Do: func(c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {
			logrus.Infof("=== Update SentinelObject %s/%s ===\n\n", key.Namespace, key.Name)

			if o == nil {
				return errors.New("SentinelObject is null")
			}

			// Add labels must force to update all resources
			o.Annotations = map[string]string{
				annotation: "test2",
			}

			if err = c.Update(context.Background(), o); err != nil {
				return err
			}

			time.Sleep(10 * time.Second)

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {

			s := &corev1.Secret{}
			cm := &corev1.ConfigMap{}

			// Check configMap
			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, cm); err != nil {
					if k8serrors.IsNotFound(err) {
						return errors.New("Not yet created")
					}
					t.Fatalf("Error when get ConfigMap: %s", err.Error())
				}
				return nil
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("Failed to get ConfigMap: %s", err.Error())
			}
			assert.Equal(t, map[string]string{"val": "test2"}, cm.Data)
			assert.NotEmpty(t, cm.OwnerReferences)

			// Check secret
			isTimeout, err = test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, s); err != nil {
					if k8serrors.IsNotFound(err) {
						return errors.New("Not yet created")
					}
					t.Fatalf("Error when get Secret: %s", err.Error())
				}
				return nil
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("Failed to get Secret: %s", err.Error())
			}
			assert.Equal(t, map[string][]byte{"val": []byte("test2")}, s.Data)
			assert.NotEmpty(t, s.OwnerReferences)

			return nil
		},
	}
}

func doRemoveAnnotationStep() test.TestStep[*corev1.Namespace] {
	return test.TestStep[*corev1.Namespace]{
		Name: "update",
		Do: func(c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {
			logrus.Infof("=== Remove annotations on SentinelObject %s/%s ===\n\n", key.Namespace, key.Name)

			if o == nil {
				return errors.New("SentinelObject is null")
			}

			// Add labels must force to update all resources
			o.Annotations = map[string]string{}

			if err = c.Update(context.Background(), o); err != nil {
				return err
			}

			time.Sleep(10 * time.Second)

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {

			s := &corev1.Secret{}
			cm := &corev1.ConfigMap{}

			// Check configMap
			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, cm); err != nil {
					if k8serrors.IsNotFound(err) {
						return nil
					}
					t.Fatalf("Error when get ConfigMap: %s", err.Error())
				}
				return errors.New("Not yet deleted")
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("ConfigMap is not deleted: %s", err.Error())
			}

			// Check secret
			isTimeout, err = test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, s); err != nil {
					if k8serrors.IsNotFound(err) {
						return nil
					}
					t.Fatalf("Error when get Secret: %s", err.Error())
				}
				return errors.New("Not yet deleted")
			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("Secret is not deleted: %s", err.Error())
			}

			return nil
		},
	}
}

func doDeleteStep() test.TestStep[*corev1.Namespace] {
	return test.TestStep[*corev1.Namespace]{
		Name: "delete",
		Do: func(c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {
			logrus.Infof("=== Delete SentinelObject cluster %s/%s ===\n\n", key.Namespace, key.Name)

			if o == nil {
				return errors.New("SentinelObject is null")
			}

			if err = c.Delete(context.Background(), o, &client.DeleteOptions{GracePeriodSeconds: ptr.To(int64(0))}); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o *corev1.Namespace, data map[string]any) (err error) {

			// In envtest, no kubelet
			// So the cascading children delation not works
			// We can't test the deletion of Namespace

			return nil
		},
	}
}
