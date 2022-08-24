package mock

import (
	"context"

	"github.com/disaster37/operator-sdk-extra/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockReconciler struct {
	reconciler controller.Reconciler
	meta       any
}

func NewMockReconciler(reconciler controller.Reconciler, metaMock any) controller.Reconciler {
	return &MockReconciler{
		reconciler: reconciler,
		meta:       metaMock,
	}
}

func (h MockReconciler) Configure(ctx context.Context, req ctrl.Request, resource client.Object) (meta any, err error) {
	return h.meta, nil
}
func (h MockReconciler) Read(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error) {
	return h.reconciler.Read(ctx, r, data, meta)
}
func (h MockReconciler) Create(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error) {
	return h.reconciler.Create(ctx, r, data, meta)
}
func (h MockReconciler) Update(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error) {
	return h.reconciler.Update(ctx, r, data, meta)
}
func (h MockReconciler) Delete(ctx context.Context, r client.Object, data map[string]any, meta any) (err error) {
	return h.reconciler.Delete(ctx, r, data, meta)
}
func (h MockReconciler) Diff(r client.Object, data map[string]any, meta any) (diff controller.Diff, err error) {
	return h.reconciler.Diff(r, data, meta)
}

func (h MockReconciler) OnError(ctx context.Context, r client.Object, data map[string]any, meta any, err error) {
	h.reconciler.OnError(ctx, r, data, meta, err)
}
func (h MockReconciler) OnSuccess(ctx context.Context, r client.Object, data map[string]any, meta any, diff controller.Diff) (err error) {
	return h.reconciler.OnSuccess(ctx, r, data, meta, diff)
}
