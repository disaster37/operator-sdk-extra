package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/disaster37/k8s-objectmatcher/patch"
	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/pkg/helper"
	"github.com/disaster37/operator-sdk-extra/pkg/test"
	cachecrd "github.com/disaster37/operator-sdk-extra/testdata/memcached-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	condition "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *ControllerTestSuite) TestMemcachedController() {
	key := types.NamespacedName{
		Name:      "t-cb-" + helper.RandomString(10),
		Namespace: "default",
	}
	mc := &cachecrd.Memcached{}
	data := map[string]any{}

	testCase := test.NewTestCase(t.T(), t.k8sClient, key, mc, 5*time.Second, data)
	testCase.Steps = []test.TestStep{
		doCreateMemcachedStep(),
		doUpdateMemcachedStep(),
		doDeleteMemcachedStep(),
	}

	testCase.Run()
}

func doCreateMemcachedStep() test.TestStep {
	return test.TestStep{
		Name: "create",
		Do: func(c client.Client, key types.NamespacedName, o client.Object, data map[string]any) (err error) {
			logrus.Infof("=== Add new Memcached %s/%s ===", key.Namespace, key.Name)

			mc := &cachecrd.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: cachecrd.MemcachedSpec{
					Size:          1,
					ContainerPort: 8080,
				},
			}

			if err = c.Create(context.Background(), mc); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o client.Object, data map[string]any) (err error) {
			mc := &cachecrd.Memcached{}
			var (
				cm  *corev1.ConfigMap
				dpl *appv1.Deployment
			)

			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, mc); err != nil {
					t.Fatal("Memcached not found")
				}

				// In envtest, no kubelet
				// So the condition never set as true
				if condition.FindStatusCondition(mc.Status.Conditions, controller.ReadyCondition.String()) != nil && condition.FindStatusCondition(mc.Status.Conditions, controller.ReadyCondition.String()).Reason != "Initialize" {
					return nil
				}

				return errors.New("Not yet created")

			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("All Memcached step provisionning not finished: %s", err.Error())
			}

			// ConfigMaps must exist
			cm = &corev1.ConfigMap{}
			if err = c.Get(context.Background(), types.NamespacedName{Namespace: key.Namespace, Name: mc.Name}, cm); err != nil {
				t.Fatal(err)
			}
			assert.NotEmpty(t, cm.OwnerReferences)
			assert.NotEmpty(t, cm.Annotations[patch.LastAppliedConfig])

			// Deployment musts exist
			dpl = &appv1.Deployment{}
			if err = c.Get(context.Background(), types.NamespacedName{Namespace: key.Namespace, Name: mc.Name}, dpl); err != nil {
				t.Fatal(err)
			}
			assert.NotEmpty(t, dpl.OwnerReferences)
			assert.NotEmpty(t, dpl.Annotations[patch.LastAppliedConfig])

			// Status must be update
			assert.NotEmpty(t, mc.Status.PhaseName)
			assert.False(t, *mc.Status.IsOnError)

			return nil
		},
	}
}

func doUpdateMemcachedStep() test.TestStep {
	return test.TestStep{
		Name: "update",
		Do: func(c client.Client, key types.NamespacedName, o client.Object, data map[string]any) (err error) {
			logrus.Infof("=== Update Memcached %s/%s ===", key.Namespace, key.Name)

			if o == nil {
				return errors.New("Memcached is null")
			}
			mc := o.(*cachecrd.Memcached)

			// Add labels must force to update all resources
			mc.Labels = map[string]string{
				"test": "fu",
			}

			// Keep the actual version to know when is updating on Etcd
			data["lastVersion"] = mc.ResourceVersion

			if err = c.Update(context.Background(), mc); err != nil {
				return err
			}

			time.Sleep(5 * time.Second)

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o client.Object, data map[string]any) (err error) {
			mc := &cachecrd.Memcached{}

			var (
				cm  *corev1.ConfigMap
				dpl *appv1.Deployment
			)

			lastVersion := data["lastVersion"].(string)

			isTimeout, err := test.RunWithTimeout(func() error {
				if err := c.Get(context.Background(), key, mc); err != nil {
					t.Fatal("Memached not found")
				}

				if lastVersion != mc.ResourceVersion {
					return nil
				}

				return errors.New("Not yet updated")

			}, time.Second*30, time.Second*1)
			if err != nil || isTimeout {
				t.Fatalf("All Memcached step upgrading not finished: %s", err.Error())
			}

			// ConfigMaps must exist
			cm = &corev1.ConfigMap{}
			if err = c.Get(context.Background(), types.NamespacedName{Namespace: key.Namespace, Name: mc.Name}, cm); err != nil {
				t.Fatal(err)
			}
			assert.NotEmpty(t, cm.OwnerReferences)
			assert.NotEmpty(t, cm.Annotations[patch.LastAppliedConfig])
			assert.Equal(t, "fu", cm.Labels["test"])

			// Deployment musts exist
			dpl = &appv1.Deployment{}
			if err = c.Get(context.Background(), types.NamespacedName{Namespace: key.Namespace, Name: mc.Name}, dpl); err != nil {
				t.Fatal(err)
			}
			assert.NotEmpty(t, dpl.OwnerReferences)
			assert.NotEmpty(t, dpl.Annotations[patch.LastAppliedConfig])
			assert.Equal(t, "fu", dpl.Labels["test"])

			// Status must be update
			assert.NotEmpty(t, mc.Status.PhaseName)
			assert.False(t, *mc.Status.IsOnError)

			return nil
		},
	}
}

func doDeleteMemcachedStep() test.TestStep {
	return test.TestStep{
		Name: "delete",
		Do: func(c client.Client, key types.NamespacedName, o client.Object, data map[string]any) (err error) {
			logrus.Infof("=== Delete Memcached %s/%s ===", key.Namespace, key.Name)

			if o == nil {
				return errors.New("Memcached is null")
			}
			mc := o.(*cachecrd.Memcached)

			wait := int64(0)
			if err = c.Delete(context.Background(), mc, &client.DeleteOptions{GracePeriodSeconds: &wait}); err != nil {
				return err
			}

			return nil
		},
		Check: func(t *testing.T, c client.Client, key types.NamespacedName, o client.Object, data map[string]any) (err error) {

			return nil
		},
	}
}
