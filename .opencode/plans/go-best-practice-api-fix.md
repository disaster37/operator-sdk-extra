# Go Best-Practice API Fix: Package-Level `As`/`AsWithDiff` Functions

## Summary

After the Go 1.27 migration (see `go1.27-migration.md`), the `.As[D]()` and `.AsWithDiff[D]()` generic methods are only available on concrete types (`*DefaultMultiPhaseStepReconcilerAction`, `*DefaultMultiPhaseRead`, `*DefaultMultiPhaseDiff`), **not** on interfaces. This is a Go language limitation — generic methods cannot be defined on interface types.

Consumers whose constructors return interfaces (e.g., `controller_sample_test.go`'s `newConfiMapReconciler`) cannot call `.As[D]()` and must use deprecated constructors like `NewObjectMultiPhaseStepReconcilerActionWithDiff`.

**Solution**: Add non-deprecated package-level functions that accept interfaces, internally type-assert to the concrete type for the fast path, and fall back to the wrapper for custom implementations. This follows Go best practice: "accept interfaces, return structs."

---

## Files and Changes

### File 1: `pkg/controller/multiphase/multiphasestep_action.go`

#### 1a. Add package-level `As` function (insert before line 257)

```go
// As converts any MultiPhaseStepReconcilerAction[K, S] to operate on client.Object subtype D.
// It prefers the efficient .As[D]() generic method when the underlying type is
// *DefaultMultiPhaseStepReconcilerAction, falling back to a wrapper for custom implementations.
//
// This is the non-deprecated equivalent of NewObjectMultiPhaseStepReconcilerAction.
func As[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerAction[K, S]) MultiPhaseStepReconcilerAction[K, D] {
	if a, ok := in.(*DefaultMultiPhaseStepReconcilerAction[K, S]); ok {
		return a.As[D]()
	}
	if in == nil {
		return nil
	}
	return &objectMultiPhaseStepReconcilerAction[K, S, D]{
		in:               in,
		ReconcilerAction: controller.NewReconcilerAction(in.Client(), in.Recorder(), in.Condition()),
	}
}
```

#### 1b. Add package-level `AsWithDiff` function (insert before line 309, the `ObjectMultiPhaseStepReconcilerActionWithDiff` type alias)

```go
// AsWithDiff converts any MultiPhaseStepReconcilerActionWithDiff[K, S] to operate on
// client.Object subtype D.
// It prefers the efficient .AsWithDiff[D]() generic method when the underlying type is
// *DefaultMultiPhaseStepReconcilerActionWithDiff, falling back to a wrapper for custom
// implementations.
//
// This is the non-deprecated equivalent of NewObjectMultiPhaseStepReconcilerActionWithDiff.
func AsWithDiff[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerActionWithDiff[K, S]) MultiPhaseStepReconcilerActionWithDiff[K, D] {
	if a, ok := in.(*DefaultMultiPhaseStepReconcilerActionWithDiff[K, S]); ok {
		return a.AsWithDiff[D]()
	}
	if in == nil {
		return nil
	}
	return &objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]{
		in:               in,
		ReconcilerAction: controller.NewReconcilerAction(in.Client(), in.Recorder(), in.Condition()),
	}
}
```

#### 1c. Refactor deprecated `NewObjectMultiPhaseStepReconcilerAction` (replace lines 260-269)

```go
// Deprecated: Use As[K, S, D](in) instead.
func NewObjectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerAction[K, S]) MultiPhaseStepReconcilerAction[K, D] {
	return As[K, S, D](in)
}
```

#### 1d. Refactor deprecated `NewObjectMultiPhaseStepReconcilerActionWithDiff` (replace lines 312-321)

```go
// Deprecated: Use AsWithDiff[K, S, D](in) instead.
func NewObjectMultiPhaseStepReconcilerActionWithDiff[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerActionWithDiff[K, S]) MultiPhaseStepReconcilerActionWithDiff[K, D] {
	return AsWithDiff[K, S, D](in)
}
```

---

### File 2: `pkg/controller/multiphase/multiphase_read.go`

#### 2a. Add package-level `As` function (insert before line 96, the `ObjectMultiPhaseRead` type alias)

```go
// As converts any MultiPhaseRead[S] to expose client.Object subtype D.
// It prefers the efficient .As[D]() generic method when the underlying type is
// *DefaultMultiPhaseRead, falling back to a wrapper for custom implementations.
//
// This is the non-deprecated equivalent of NewObjectMultiphaseRead.
func As[S, D client.Object](in MultiPhaseRead[S]) MultiPhaseRead[D] {
	if a, ok := in.(*DefaultMultiPhaseRead[S]); ok {
		return a.As[D]()
	}
	if in == nil {
		return nil
	}
	return &objectMultiPhaseRead[S, D]{in: in}
}
```

#### 2b. Refactor deprecated `NewObjectMultiphaseRead` (replace lines 99-102)

```go
// Deprecated: Use As[S, D](in) instead.
func NewObjectMultiphaseRead[Src, Dst client.Object](in MultiPhaseRead[Src]) MultiPhaseRead[Dst] {
	return As[Src, Dst](in)
}
```

---

### File 3: `pkg/controller/multiphase/multiphase_diff.go`

#### 3a. Add package-level `As` function (insert before line 139, the `ObjectMultiPhaseDiff` type alias)

```go
// As converts any MultiPhaseDiff[S] to expose client.Object subtype D.
// It prefers the efficient .As[D]() generic method when the underlying type is
// *DefaultMultiPhaseDiff, falling back to a wrapper for custom implementations.
//
// This is the non-deprecated equivalent of NewObjectMultiphaseDiff.
func As[S, D client.Object](in MultiPhaseDiff[S]) MultiPhaseDiff[D] {
	if a, ok := in.(*DefaultMultiPhaseDiff[S]); ok {
		return a.As[D]()
	}
	if in == nil {
		return nil
	}
	return &objectMultiPhaseDiff[S, D]{in: in}
}
```

#### 3b. Refactor deprecated `NewObjectMultiphaseDiff` (replace lines 142-145)

```go
// Deprecated: Use As[S, D](in) instead.
func NewObjectMultiphaseDiff[Src, Dst client.Object](in MultiPhaseDiff[Src]) MultiPhaseDiff[Dst] {
	return As[Src, Dst](in)
}
```

---

### File 4: `pkg/controller/multiphase/controller_sample_test.go`

#### 4a. Replace deprecated constructor call on line 58

**Before:**
```go
multiphase.NewObjectMultiPhaseStepReconcilerActionWithDiff[*MultiPhaseObject, *corev1.ConfigMap, client.Object](newConfiMapReconciler(c, recorder)),
```

**After:**
```go
multiphase.AsWithDiff[*MultiPhaseObject, *corev1.ConfigMap, client.Object](newConfiMapReconciler(c, recorder)),
```

No other changes needed. The `newConfiMapReconciler` returns the interface type (not the concrete type), so this is the canonical use case for the new package-level function. The fast path won't trigger here (the underlying type is `*configMapReconciler`, not `*DefaultMultiPhaseStepReconcilerActionWithDiff`), but the fallback wrapper works correctly.

---

### File 5: `samples/memcached-operator/controllers/memcached_controller.go`

**No changes needed.** Lines 70-71 already use `.As[client.Object]()` and `.AsWithDiff[client.Object]()` on concrete-type variables:

```go
configMapStep.As[client.Object](),
deploymentStep.AsWithDiff[client.Object](),
```

This works because `newConfigMapReconciler` and `newDeploymentReconciler` return concrete types (`*configMapReconciler`, `*deploymentReconciler`).

---

### File 6: Tests — `pkg/controller/multiphase/action_unit_test.go`

Add tests for the new package-level functions. Insert after the existing `TestObjectMultiPhaseDiffEmptySliceHandling` (line 387) and before `TestMultiPhaseStepReconcilerAction` (line 389).

#### 6a. Test package-level `As` for actions

```go
// Test package-level As[K, S, D]() function
func TestPackageLevelAs(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	recorder := &mockEventRecorder{}

	t.Run("fast path: *DefaultMultiPhaseStepReconcilerAction uses .As[D]()", func(t *testing.T) {
		inner := NewMultiPhaseStepReconcilerAction[*MockMultiPhaseObject, *corev1.ConfigMap](cl, "phase", "condition", recorder, "fm")
		wrapped := As[*MockMultiPhaseObject, *corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)
		assert.Equal(t, shared.PhaseName("phase"), wrapped.GetPhaseName())
	})

	t.Run("fallback: custom implementation uses wrapper", func(t *testing.T) {
		// Create a custom type that embeds the interface (not the concrete type)
		type customAction struct {
			MultiPhaseStepReconcilerAction[*MockMultiPhaseObject, *corev1.ConfigMap]
		}
		inner := &customAction{
			MultiPhaseStepReconcilerAction: NewMultiPhaseStepReconcilerAction[*MockMultiPhaseObject, *corev1.ConfigMap](cl, "phase", "condition", recorder, "fm"),
		}
		wrapped := As[*MockMultiPhaseObject, *corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)
		assert.Equal(t, shared.PhaseName("phase"), wrapped.GetPhaseName())
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		var nilAction MultiPhaseStepReconcilerAction[*MockMultiPhaseObject, *corev1.ConfigMap]
		wrapped := As[*MockMultiPhaseObject, *corev1.ConfigMap, client.Object](nilAction)
		assert.Nil(t, wrapped)
	})
}
```

#### 6b. Test package-level `AsWithDiff` for actions

```go
// Test package-level AsWithDiff[K, S, D]() function
func TestPackageLevelAsWithDiff(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	recorder := &mockEventRecorder{}

	t.Run("fast path: *DefaultMultiPhaseStepReconcilerActionWithDiff uses .AsWithDiff[D]()", func(t *testing.T) {
		inner := NewMultiPhaseStepReconcilerActionWithDiff[*MockMultiPhaseObject, *corev1.ConfigMap](cl, "phase", "condition", recorder, "fm")
		wrapped := AsWithDiff[*MockMultiPhaseObject, *corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)
		assert.Equal(t, shared.PhaseName("phase"), wrapped.GetPhaseName())
	})

	t.Run("fallback: custom implementation uses wrapper", func(t *testing.T) {
		type customAction struct {
			MultiPhaseStepReconcilerActionWithDiff[*MockMultiPhaseObject, *corev1.ConfigMap]
		}
		inner := &customAction{
			MultiPhaseStepReconcilerActionWithDiff: NewMultiPhaseStepReconcilerActionWithDiff[*MockMultiPhaseObject, *corev1.ConfigMap](cl, "phase", "condition", recorder, "fm"),
		}
		wrapped := AsWithDiff[*MockMultiPhaseObject, *corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)
		assert.Equal(t, shared.PhaseName("phase"), wrapped.GetPhaseName())
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		var nilAction MultiPhaseStepReconcilerActionWithDiff[*MockMultiPhaseObject, *corev1.ConfigMap]
		wrapped := AsWithDiff[*MockMultiPhaseObject, *corev1.ConfigMap, client.Object](nilAction)
		assert.Nil(t, wrapped)
	})
}
```

#### 6c. Test package-level `As` for reads

```go
// Test package-level As[S, D]() function for MultiPhaseRead
func TestPackageLevelAsRead(t *testing.T) {
	t.Run("fast path: *DefaultMultiPhaseRead uses .As[D]()", func(t *testing.T) {
		inner := NewMultiPhaseRead[*corev1.ConfigMap]()
		wrapped := As[*corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
		wrapped.AddCurrentObject(cm)
		assert.Len(t, wrapped.GetCurrentObjects(), 1)
	})

	t.Run("fallback: custom implementation uses wrapper", func(t *testing.T) {
		type customRead struct {
			MultiPhaseRead[*corev1.ConfigMap]
		}
		inner := &customRead{
			MultiPhaseRead: NewMultiPhaseRead[*corev1.ConfigMap](),
		}
		wrapped := As[*corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
		wrapped.AddCurrentObject(cm)
		assert.Len(t, wrapped.GetCurrentObjects(), 1)
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		var nilRead MultiPhaseRead[*corev1.ConfigMap]
		wrapped := As[*corev1.ConfigMap, client.Object](nilRead)
		assert.Nil(t, wrapped)
	})
}
```

#### 6d. Test package-level `As` for diffs

```go
// Test package-level As[S, D]() function for MultiPhaseDiff
func TestPackageLevelAsDiff(t *testing.T) {
	t.Run("fast path: *DefaultMultiPhaseDiff uses .As[D]()", func(t *testing.T) {
		inner := NewMultiPhaseDiff[*corev1.ConfigMap]()
		wrapped := As[*corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
		wrapped.AddObjectToCreate(cm)
		assert.True(t, wrapped.NeedCreate())
	})

	t.Run("fallback: custom implementation uses wrapper", func(t *testing.T) {
		type customDiff struct {
			MultiPhaseDiff[*corev1.ConfigMap]
		}
		inner := &customDiff{
			MultiPhaseDiff: NewMultiPhaseDiff[*corev1.ConfigMap](),
		}
		wrapped := As[*corev1.ConfigMap, client.Object](inner)
		assert.NotNil(t, wrapped)

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
		wrapped.AddObjectToCreate(cm)
		assert.True(t, wrapped.NeedCreate())
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		var nilDiff MultiPhaseDiff[*corev1.ConfigMap]
		wrapped := As[*corev1.ConfigMap, client.Object](nilDiff)
		assert.Nil(t, wrapped)
	})
}
```

---

## Design Decisions

### 1. Fast path for reads/diffs (answered)

The new `As` functions for `MultiPhaseRead` and `MultiPhaseDiff` include the type-assert fast path for consistency with the action pattern, even though `*DefaultMultiPhaseRead.As[D]()` and `*DefaultMultiPhaseDiff.As[D]()` return the same wrapper type as the fallback. This future-proofs the API in case the concrete `.As[D]()` methods gain additional behavior.

### 2. Fast path won't trigger for embedded types

When a consumer wraps the default action in a custom struct (e.g., `*configMapReconciler` embedding `*DefaultMultiPhaseStepReconcilerActionWithDiff`), the type assertion `in.(*DefaultMultiPhaseStepReconcilerActionWithDiff[K, S])` will fail. The fallback wrapper is used instead. This is correct — the fallback creates a proper delegating wrapper. No correctness issue.

### 3. Name collision: package-level `As` vs method `.As[D]()`

Go allows package-level functions and methods to share names; there is no ambiguity. A bare `As[...](x)` resolves to the package function; `x.As[D]()` resolves to the method.

### 4. Internal uses of `NewObjectMultiphaseRead`/`NewObjectMultiphaseDiff`

The `objectMultiPhaseStepReconcilerAction` methods call `NewObjectMultiphaseRead[S, D](readTmp)` and `NewObjectMultiphaseDiff[S, D](diffTmp)` internally. Since these deprecated constructors now delegate to the new `As` functions, internal callers automatically benefit from the fast path. No additional internal changes needed.

---

## What NOT to change

- Do NOT change constructor return types from interfaces to concrete types (breaking change)
- Do NOT remove deprecated aliases or constructors (`ObjectMultiPhase*` types, `NewObject*` functions)
- Do NOT change the `.As[D]()` / `.AsWithDiff[D]()` generic methods on concrete types

---

## Validation Plan

1. **Build**: `go build ./...` — must compile cleanly
2. **Vet**: `go vet ./...` — no new warnings
3. **Tests**: `dagger call --src . test --withGotestsum 2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200`
4. **Specific test run**: `dagger call --src . test --withGotestsum --run "TestPackageLevel"` — all new tests pass
5. **Sample compiles**: `cd samples/memcached-operator && go build ./...`
6. **Deprecated API still works**: Verify existing tests using deprecated constructors still pass (they now delegate to new functions)

---

## Ordered Task List

1. **Add `As` and `AsWithDiff` to `multiphasestep_action.go`** — insert before deprecated type aliases
2. **Refactor deprecated action constructors** to delegate to new functions
3. **Add `As` to `multiphase_read.go`** — insert before deprecated type alias
4. **Refactor deprecated `NewObjectMultiphaseRead`** to delegate to new function
5. **Add `As` to `multiphase_diff.go`** — insert before deprecated type alias
6. **Refactor deprecated `NewObjectMultiphaseDiff`** to delegate to new function
7. **Update `controller_sample_test.go`** — replace deprecated constructor with `AsWithDiff`
8. **Add tests** for all four package-level functions in `action_unit_test.go`
9. **Run full test suite** and verify
