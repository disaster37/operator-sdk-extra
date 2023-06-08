# operator-sdk-extra

This framework must be used on top on `operator-sdk`. It permit to not manage the deletion process (finalizer and delete). 
It introduce a way to test your operator with envtest.

You just need to implement the following interfaces:

1. Your operator interact only with kubernetes resources:

```golang
type K8sReconciler interface {
	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource client.Object) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, r client.Object, data map[string]any) (err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, r client.Object, data map[string]any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)
}

type K8sPhaseReconciler interface {
	// Configure permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource client.Object) (res ctrl.Result, err error)

	// Read permit to read kubernetes resources
	Read(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Create permit to create resources on kubernetes
	Create(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Update permit to update resources on kubernetes
	Update(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// Delete permit to delete resources on kubernetes
	Delete(ctx context.Context, r client.Object, data map[string]any) (res ctrl.Result, err error)

	// OnError is call when error is throwing on current phase
	// It the right way to set status condition when error
	OnError(ctx context.Context, r client.Object, data map[string]any, currentErr error) (res ctrl.Result, err error)

	// OnSuccess is call at the end of current phase, if not error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r client.Object, data map[string]any, diff K8sDiff) (res ctrl.Result, err error)

	// Diff permit to compare the actual state and the expected state
	Diff(ctx context.Context, r client.Object, data map[string]any) (diff K8sDiff, res ctrl.Result, err error)

	// GetName return the reconciler name
	GetName() string
}
```

2. Your operator interfact with external ressources over API

```golang
type Reconciler interface {
	// Confirgure permit to init external provider driver (API client REST)
	// It can also permit to init condition on status
	Configure(ctx context.Context, req ctrl.Request, resource client.Object) (meta any, err error)

	// Read permit to read the actual resource state from provider and set it on data map
	Read(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// Create permit to create resource on provider
	// It only call if diff.NeeCreated is true
	Create(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// Update permit to update resource on provider
	// It only call if diff.NeedUpdated is true
	Update(ctx context.Context, r client.Object, data map[string]any, meta any) (res ctrl.Result, err error)

	// Delete permit to delete resource on provider
	// It only call if you have specified finalizer name when you create reconciler and if resource as marked to be deleted
	Delete(ctx context.Context, r client.Object, data map[string]any, meta any) (err error)

	// OnError is call when error is throwing
	// It the right way to set status condition when error
	OnError(ctx context.Context, r client.Object, data map[string]any, meta any, err error)

	// OnSuccess is call at the end if no error
	// It's the right way to set status condition when everithink is good
	OnSuccess(ctx context.Context, r client.Object, data map[string]any, meta any, diff Diff) (err error)

	// Diff permit to compare the actual state and the expected state
	Diff(r client.Object, data map[string]any, meta any) (diff Diff, err error)
}
```


## Tips

### Ignore reconcile

If you should to manually change ressources handled by operator, it can be usefull to ignore reconcilation on them. To to that, you can add the following annotation: `operator-sdk-extra.webcenter.fr/ignoreReconcile: "true"`