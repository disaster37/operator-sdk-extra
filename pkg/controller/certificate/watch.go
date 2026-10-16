package certificate

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/v3/pkg/controller"
	"github.com/disaster37/operator-sdk-extra/v3/pkg/object"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// TLSSecretNameIndexField is the default field-indexer key used by
	// SetupTLSSecretIndexer and SecretToTLSObjectsMapFunc to map leaf and CA
	// TLS Secret names back to the owning MultiPhaseObject CR.
	TLSSecretNameIndexField = "spec.tls.secretName"

	// caSecretWatchSuffix mirrors selfmanaged.CASecretSuffix ("-ca") without
	// importing subpackages.
	caSecretWatchSuffix = "-ca"
)

// ExtractTLSSecretNames returns a client.IndexerFunc that indexes a
// MultiPhaseObject by both its leaf Secret name (<SecretName>) and its CA
// Secret name (<SecretName>-ca). Objects with an empty SecretName are not
// indexed.
func ExtractTLSSecretNames[T object.MultiPhaseObject](provider TLSSpecProvider[T]) client.IndexerFunc {
	return func(rawObj client.Object) []string {
		if provider == nil {
			return nil
		}
		typedObj, ok := rawObj.(T)
		if !ok {
			return nil
		}
		spec := provider.TLSSpec(typedObj)
		if spec.SecretName == "" {
			return nil
		}
		return []string{spec.SecretName, spec.SecretName + caSecretWatchSuffix}
	}
}

// SetupTLSSecretIndexer returns a controller.Indexer (compatible with
// controller.SetupIndexerWithManager) that indexes objects of type T by their
// leaf and CA TLS Secret names under the given field key. If field is empty,
// TLSSecretNameIndexField is used.
func SetupTLSSecretIndexer[T object.MultiPhaseObject](prototype T, field string, provider TLSSpecProvider[T]) controller.Indexer {
	if field == "" {
		field = TLSSecretNameIndexField
	}
	return func(mgr ctrl.Manager) error {
		return mgr.GetFieldIndexer().IndexField(context.Background(), prototype, field, ExtractTLSSecretNames(provider))
	}
}

// SecretToTLSObjectsMapFunc returns a handler.MapFunc (for use with
// builder.Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(...)))
// that enqueues reconcile requests for every object in newListFunc() within
// the Secret's namespace whose indexed TLS Secret name matches secret.GetName().
// If field is empty, TLSSecretNameIndexField is used.
func SecretToTLSObjectsMapFunc(c client.Client, newListFunc func() client.ObjectList, field string) handler.MapFunc {
	if field == "" {
		field = TLSSecretNameIndexField
	}
	return func(ctx context.Context, secret client.Object) []reconcile.Request {
		if secret == nil || secret.GetName() == "" {
			return nil
		}
		list := newListFunc()
		if err := c.List(ctx, list, client.InNamespace(secret.GetNamespace()), client.MatchingFields{field: secret.GetName()}); err != nil {
			return nil
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			if obj, ok := item.(client.Object); ok {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: obj.GetNamespace(),
						Name:      obj.GetName(),
					},
				})
			}
		}
		return reqs
	}
}
