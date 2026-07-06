package controller

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureNetworkPolicyForWebhook(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)

	labels := map[string]string{"app": "test"}
	podSelectors := map[string]string{"app": "test-pod"}

	t.Run("create network policy when it does not exist", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())

		err := EnsureNetworkPolicyForWebhook(context.Background(), c, logger, "test-ns", labels, podSelectors)
		assert.NoError(t, err)

		np := &networkv1.NetworkPolicy{}
		err = c.Get(context.Background(), types.NamespacedName{Namespace: "test-ns", Name: "allow-webhook-access-from-any"}, np)
		assert.NoError(t, err)
		assert.Equal(t, "test-ns", np.Namespace)
		assert.Equal(t, labels, np.Labels)
		assert.Equal(t, podSelectors, np.Spec.PodSelector.MatchLabels)
	})

	t.Run("update succeeds when network policy exists with different spec", func(t *testing.T) {
		existingNP := &networkv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test-ns",
				Name:      "allow-webhook-access-from-any",
				Labels:    labels,
			},
			Spec: networkv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"old": "selector"},
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingNP).Build()
		logger := logrus.NewEntry(logrus.StandardLogger())

		err := EnsureNetworkPolicyForWebhook(context.Background(), c, logger, "test-ns", labels, podSelectors)
		assert.NoError(t, err)
	})
}
