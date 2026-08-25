# Go 1.27 Migration Plan

## Summary

Migrate `github.com/disaster37/operator-sdk-extra/v3` from Go 1.26 to 1.27, leveraging three new language features:

| Feature | Impact on this project |
|---|---|
| **Generic methods** | High — replace verbose `ObjectMultiPhase*` wrapper types with `.As[D]()` methods |
| **Function type inference in assignments** | Medium — callers can omit explicit type parameters in many test helpers |
| **Struct composite literal keys as selectors** | None — no applicable patterns found |

---

## Prerequisite: Update go.mod

**File:** `go.mod` (line 3)

```diff
- go 1.26.0
+ go 1.27.0
```

```diff
- toolchain go1.26.6
+ toolchain go1.27.0
```

Run `go mod tidy` after the version bump.

---

## Change 1 (HIGH): Add `.As[D]()` generic method to `DefaultMultiPhaseStepReconcilerAction`

### Why

The `ObjectMultiPhaseStepReconcilerAction[K, Src, Dst]` wrapper (79 lines, 3 type parameters) exists solely to convert a `MultiPhaseStepReconcilerAction[K, Src]` into `MultiPhaseStepReconcilerAction[K, Dst]` (where both `Src` and `Dst` are `client.Object`). Consumers must write:

```go
multiphase.NewObjectMultiPhaseStepReconcilerAction[*MyCRD, *corev1.ConfigMap, client.Object](configMapStep)
```

With Go 1.27 generic methods, the base type can expose:

```go
configMapStep.As[client.Object]()
```

This reduces 3 explicit type parameters to 1, eliminates the exported wrapper type from the public API, and removes the need for consumers to import and understand the wrapper.

### Files affected

| File | Lines | Action |
|---|---|---|
| `pkg/controller/multiphase/multiphasestep_action.go` | 234–284 | Replace exported `ObjectMultiPhaseStepReconcilerAction` with unexported `objectMultiPhaseStepReconcilerAction` + add `As[D]()` method on `DefaultMultiPhaseStepReconcilerAction` |
| `pkg/controller/multiphase/multiphasestep_action.go` | 286–341 | Same for `ObjectMultiPhaseStepReconcilerActionWithDiff` → add `AsWithDiff[D]()` on `DefaultMultiPhaseStepReconcilerActionWithDiff` |
| `pkg/controller/multiphase/multiphase_read.go` | 80–114 | Replace exported `ObjectMultiPhaseRead` with unexported + add `As[D]()` on `DefaultMultiPhaseRead` |
| `pkg/controller/multiphase/multiphase_diff.go` | 126–199 | Replace exported `ObjectMultiPhaseDiff` with unexported + add `As[D]()` on `DefaultMultiPhaseDiff` |
| `samples/memcached-operator/controllers/memcached_controller.go` | 70–71 | Update call sites |
| Test files | various | Update call sites |

### Before/After: Action wrapper

**BEFORE** (`multiphasestep_action.go`, lines 234–284):

```go
// ObjectMultiPhaseStepReconcilerAction is the implementation of MultiPhaseStepReconcilerAction for a specific client.Object type needed by multiphase reconciler
// It's kind of wrapper to convert MultiPhaseStepReconcilerAction[k8sStepObject] to MultiPhaseStepReconcilerAction[client.Object]
type ObjectMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
	controller.ReconcilerAction
	in MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc]
}

func NewObjectMultiPhaseStepReconcilerAction[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc]) MultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectDst] {
	return &ObjectMultiPhaseStepReconcilerAction[k8sObject, k8sStepObjectSrc, k8sStepObjectDst]{
		in: in,
		ReconcilerAction: controller.NewReconcilerAction(
			in.Client(),
			in.Recorder(),
			in.Condition(),
		),
	}
}
// ... 30+ lines of delegating methods ...
```

**AFTER**:

```go
// As converts this step action to operate on a different client.Object subtype D.
// The conversion is identity-based (all client.Object subtypes share the same
// underlying interface); it is provided as a method for ergonomics so callers
// don't need to construct the wrapper type manually.
func (h *DefaultMultiPhaseStepReconcilerAction[K, S]) As[D client.Object]() MultiPhaseStepReconcilerAction[K, D] {
	return &objectMultiPhaseStepReconcilerAction[K, S, D]{
		in: h,
		ReconcilerAction: controller.NewReconcilerAction(
			h.Client(),
			h.Recorder(),
			h.Condition(),
		),
	}
}

// objectMultiPhaseStepReconcilerAction is the unexported wrapper (was ObjectMultiPhaseStepReconcilerAction).
type objectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object] struct {
	controller.ReconcilerAction
	in MultiPhaseStepReconcilerAction[K, S]
}
// ... delegating methods unchanged but on unexported type ...
```

### Before/After: WithDiff wrapper

**BEFORE** (`multiphasestep_action.go`, lines 286–341):

```go
type ObjectMultiPhaseStepReconcilerActionWithDiff[k8sObject object.MultiPhaseObject, k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
	controller.ReconcilerAction
	in MultiPhaseStepReconcilerActionWithDiff[k8sObject, k8sStepObjectSrc]
}

func NewObjectMultiPhaseStepReconcilerActionWithDiff[...](in MultiPhaseStepReconcilerActionWithDiff[K, Src]) MultiPhaseStepReconcilerActionWithDiff[K, Dst] {
    // ...
}
// ... delegating methods ...
```

**AFTER**:

```go
// AsWithDiff converts this diff-variant step action to operate on a different
// client.Object subtype D.
func (h *DefaultMultiPhaseStepReconcilerActionWithDiff[K, S]) AsWithDiff[D client.Object]() MultiPhaseStepReconcilerActionWithDiff[K, D] {
	return &objectMultiPhaseStepReconcilerActionWithDiff[K, S, D]{
		in: h,
		ReconcilerAction: controller.NewReconcilerAction(
			h.Client(),
			h.Recorder(),
			h.Condition(),
		),
	}
}

// objectMultiPhaseStepReconcilerActionWithDiff is the unexported wrapper.
type objectMultiPhaseStepReconcilerActionWithDiff[K object.MultiPhaseObject, S, D client.Object] struct {
	controller.ReconcilerAction
	in MultiPhaseStepReconcilerActionWithDiff[K, S]
}
```

### Before/After: Read wrapper

**BEFORE** (`multiphase_read.go`, lines 80–114):

```go
type ObjectMultiPhaseRead[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
	in MultiPhaseRead[k8sStepObjectSrc]
}

func NewObjectMultiphaseRead[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseRead[k8sStepObjectSrc]) MultiPhaseRead[k8sStepObjectDst] {
    // ...
}
```

**AFTER**:

```go
// As converts this read container to expose a different client.Object subtype D.
func (h *DefaultMultiPhaseRead[S]) As[D client.Object]() MultiPhaseRead[D] {
	return &objectMultiPhaseRead[S, D]{in: h}
}

// objectMultiPhaseRead is the unexported wrapper (was ObjectMultiPhaseRead).
type objectMultiPhaseRead[S, D client.Object] struct {
	in MultiPhaseRead[S]
}
```

### Before/After: Diff wrapper

**BEFORE** (`multiphase_diff.go`, lines 126–199):

```go
type ObjectMultiPhaseDiff[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object] struct {
	in MultiPhaseDiff[k8sStepObjectSrc]
}

func NewObjectMultiphaseDiff[k8sStepObjectSrc client.Object, k8sStepObjectDst client.Object](in MultiPhaseDiff[k8sStepObjectSrc]) MultiPhaseDiff[k8sStepObjectDst] {
    // ...
}
```

**AFTER**:

```go
// As converts this diff container to expose a different client.Object subtype D.
func (h *DefaultMultiPhaseDiff[S]) As[D client.Object]() MultiPhaseDiff[D] {
	return &objectMultiPhaseDiff[S, D]{in: h}
}

// objectMultiPhaseDiff is the unexported wrapper (was ObjectMultiPhaseDiff).
type objectMultiPhaseDiff[S, D client.Object] struct {
	in MultiPhaseDiff[S]
}
```

### Call site update: `samples/memcached-operator/controllers/memcached_controller.go`

**BEFORE** (line 70–71):

```go
multiphase.NewObjectMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap, client.Object](configMapStep),
multiphase.NewObjectMultiPhaseStepReconcilerActionWithDiff[*cachecrd.Memcached, *appv1.Deployment, client.Object](deploymentStep),
```

**AFTER**:

```go
configMapStep.As[client.Object](),
deploymentStep.AsWithDiff[client.Object](),
```

**Note:** `AsWithDiff` is a method on `*DefaultMultiPhaseStepReconcilerActionWithDiff`, but `configMapStep` and `deploymentStep` are typed as the interface `MultiPhaseStepReconcilerAction[K, S]` / `MultiPhaseStepReconcilerActionWithDiff[K, S]`. Since Go generic methods can only be called on concrete types (not interfaces), callers must either:
- Keep the concrete type reference after construction, or
- Use a type assertion to get back to the concrete type before calling `.As[D]()`

**Design decision needed:** Should we add a helper function that does the type assertion internally? For now, the plan documents both options. The recommended approach is to keep the concrete type in a variable before converting:

```go
configMapAction := multiphase.NewMultiPhaseStepReconcilerAction[*cachecrd.Memcached, *corev1.ConfigMap](...)
deploymentAction := multiphase.NewMultiPhaseStepReconcilerActionWithDiff[*cachecrd.Memcached, *appv1.Deployment](...)
// later:
configMapAction.As[client.Object](),
deploymentAction.AsWithDiff[client.Object](),
```

---

## Change 2 (MEDIUM): Add `.As[D]()` generic method to `DefaultSentinelAction`

### Why

The sentinel reconciler uses `multiphase.MultiPhaseDiff[client.Object]` directly (no wrapper needed for the action), but the `SentinelRead` type has internal `MultiPhaseRead[client.Object]` maps. Adding `As[D]` to the multiphase read/diff types (Change 1) already covers sentinel needs. No additional sentinel-specific changes needed.

**Verdict:** No changes needed — covered by Change 1.

---

## Change 3 (MEDIUM): Function type inference improvements — call-site simplifications

### Why

Go 1.27 extends function type inference to all assignment contexts (variable declarations, field assignments, return statements), not just call expressions. This means several generic test helper functions no longer require explicit type arguments at call sites.

These are **compiler-level improvements** — the source code of the helpers themselves does not change, but callers can omit type parameters they previously had to write explicitly.

### Affected functions (callers benefit, no source changes needed)

| Function | File | Current call pattern | 1.27 call pattern |
|---|---|---|---|
| `AssertReadyCondition[T]` | `pkg/test/assertions.go:19` | `AssertReadyCondition[MyType](t, obj, fn)` | `AssertReadyCondition(t, obj, fn)` — `T` inferred from `fn` |
| `AssertErrorCondition[T]` | `pkg/test/assertions.go:27` | `AssertErrorCondition[MyType](t, obj, fn, reason)` | `AssertErrorCondition(t, obj, fn, reason)` — `T` inferred from `fn` |
| `WaitForReadyCondition[T]` | `pkg/test/assertions.go:73` | `WaitForReadyCondition[MyType](t, c, key, timeout)` | Depends on call context |
| `WaitForGenerationIncrement[T]` | `pkg/test/assertions.go:90` | Same | Depends on call context |
| `WaitForResourceVersionChange[T]` | `pkg/test/assertions.go:107` | Same | Depends on call context |
| `NewCR[T]` | `pkg/test/builders.go:12` | `NewCR[MyType](name, ns)` | May still need explicit `T` (no contextual clue) |
| `NewTestCase[T]` | `pkg/test/reconciler.go:33` | `NewTestCase[MyType](t, c, key, ...)` | May still need explicit `T` |
| `NewCreateStep[T]` | `pkg/test/steps.go:26` | `NewCreateStep[MyType](builder)` | `NewCreateStep(builder)` — `T` inferred from builder return type |
| `NewDeleteStep[T]` | `pkg/test/steps.go:12` | `NewDeleteStep[MyType]()` | `T` inferred from surrounding `TestStep[T]` assignment |
| `EqualFromYamlFile[T]` | `pkg/test/equal.go:18` | `EqualFromYamlFile[MyType](t, file, obj, s)` | May still need explicit `T` |
| `GetItems[L, O]` | `pkg/controller/sentinel/helper.go:16` | `GetItems[ListType, ObjType](list)` | `T` may be inferred from assignment |
| `CloneObject[T]` | `pkg/controller/sentinel/helper.go:63` | `CloneObject[MyType](obj)` | `T` inferred from argument |
| `GetObjectWithMeta[T]` | `pkg/controller/sentinel/helper.go:35` | `GetObjectWithMeta[MyType](obj, s)` | `T` inferred from argument |

**Action:** No source code changes needed. Document these as "callers may now omit explicit type arguments." Add a note to CONTRIBUTING.md about the new minimum Go version.

---

## Change 4 (LOW): Add `.As[D]()` generic method to `DefaultWorkflowStepReconcilerAction`

### Why

`DefaultWorkflowStepReconcilerAction[K, S]` embeds `*DefaultMultiPhaseStepReconcilerAction[K, S]`, so it automatically inherits the `.As[D]()` method added in Change 1. Similarly, `DefaultWorkflowStepReconcilerActionWithDiff[K, S]` inherits `.AsWithDiff[D]()`.

**Verdict:** No additional changes needed — inherited from Change 1.

---

## Change 5 (LOW): Struct composite literal keys — no applicable patterns

After reviewing all struct composite literals in the codebase, none would benefit from selector-key syntax. The primary struct types are:
- Kubernetes API types (`corev1.Secret`, `metav1.ObjectMeta`, etc.) — constructed with flat field names
- `certificate.TLSSpec` — large struct but fields are assigned directly, not through nested paths
- Internal data types — use flat field access

**Verdict:** No changes. Document that this feature was evaluated and found not applicable.

---

## Change 6 (LOW): `new(apiObject)` pattern in `remote_action.go`

**File:** `pkg/controller/remote/remote_action.go`, line 202

```go
originalObject := new(apiObject)
```

This uses `new()` on a type parameter constrained by `comparable`. With Go 1.27 generic methods, this could theoretically be refactored, but the current code is already clean and idiomatic. No change needed.

---

## Edge Cases and Risks

### Risk 1: Generic methods on concrete types, not interfaces

Go 1.27 generic methods can only be **defined** on concrete types (structs), not on interface types. This means `.As[D]()` is a method on `*DefaultMultiPhaseStepReconcilerAction[K, S]`, not on `MultiPhaseStepReconcilerAction[K, S]`.

**Impact:** Callers who hold the interface type must either:
- Keep a reference to the concrete type, OR
- Type-assert back to the concrete type before calling `.As[D]()`

**Mitigation:** The existing `New*` constructors return concrete types. Callers who assign to a concrete-typed variable can call `.As[D]()` directly. This is the recommended pattern.

### Risk 2: Backward compatibility

The exported `ObjectMultiPhase*` types and their `New*` constructors are part of the public API. Removing them is a breaking change for consumers who use them directly.

**Mitigation:** Keep the exported types as deprecated aliases for one release cycle:

```go
// Deprecated: Use DefaultMultiPhaseStepReconcilerAction.As[D]() instead.
type ObjectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object] = objectMultiPhaseStepReconcilerAction[K, S, D]

// Deprecated: Use DefaultMultiPhaseStepReconcilerAction.As[D]() instead.
func NewObjectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerAction[K, S]) MultiPhaseStepReconcilerAction[K, D] {
	return (*DefaultMultiPhaseStepReconcilerAction[K, S])(nil).As[D]() // won't work
}
```

Actually, since the constructor takes an interface and returns an interface, and the new `.As[D]()` method is on the concrete type, we need a thin wrapper:

```go
// Deprecated: Use a concrete action's .As[D]() method instead.
func NewObjectMultiPhaseStepReconcilerAction[K object.MultiPhaseObject, S, D client.Object](in MultiPhaseStepReconcilerAction[K, S]) MultiPhaseStepReconcilerAction[K, D] {
	if a, ok := in.(*DefaultMultiPhaseStepReconcilerAction[K, S]); ok {
		return a.As[D]()
	}
	// Fallback for non-default implementations: use the wrapper directly.
	return &objectMultiPhaseStepReconcilerAction[K, S, D]{
		in: in,
		ReconcilerAction: controller.NewReconcilerAction(in.Client(), in.Recorder(), in.Condition()),
	}
}
```

### Risk 3: Test coverage

The wrapper types have dedicated tests in:
- `pkg/controller/multiphase/action_unit_test.go` (lines 244–478)
- `pkg/controller/multiphase/multiphasestep_action_withdiff_unit_test.go` (lines 119–162)

These tests validate that the wrappers delegate correctly. After migration, these tests should be updated to test the `.As[D]()` methods on the concrete types instead.

### Risk 4: Go version compatibility

After bumping to Go 1.27, any downstream consumers must also use Go ≥ 1.27. This is a major version of the `operator-sdk-extra` module (already at `/v3`), so a `/v4` might be warranted if strict backward compatibility is required.

**Decision needed:** Should this migration be released as v3.x (requiring Go 1.27 for all v3 consumers) or as v4?

---

## Migration Steps (ordered)

1. **Update `go.mod`**: Bump `go 1.26.0` → `go 1.27.0` and `toolchain go1.26.6` → `toolchain go1.27.0`. Run `go mod tidy`.

2. **Add `.As[D]()` to `DefaultMultiPhaseRead`** (`pkg/controller/multiphase/multiphase_read.go`):
   - Unexport `ObjectMultiPhaseRead` → `objectMultiPhaseRead`
   - Add `func (h *DefaultMultiPhaseRead[S]) As[D client.Object]() MultiPhaseRead[D]`
   - Keep deprecated type alias and constructor for backward compat

3. **Add `.As[D]()` to `DefaultMultiPhaseDiff`** (`pkg/controller/multiphase/multiphase_diff.go`):
   - Unexport `ObjectMultiPhaseDiff` → `objectMultiPhaseDiff`
   - Add `func (h *DefaultMultiPhaseDiff[S]) As[D client.Object]() MultiPhaseDiff[D]`
   - Keep deprecated type alias and constructor for backward compat

4. **Add `.As[D]()` to `DefaultMultiPhaseStepReconcilerAction`** (`pkg/controller/multiphase/multiphasestep_action.go`):
   - Unexport `ObjectMultiPhaseStepReconcilerAction` → `objectMultiPhaseStepReconcilerAction`
   - Add `func (h *DefaultMultiPhaseStepReconcilerAction[K, S]) As[D client.Object]() MultiPhaseStepReconcilerAction[K, D]`
   - Add `func (h *DefaultMultiPhaseStepReconcilerActionWithDiff[K, S]) AsWithDiff[D client.Object]() MultiPhaseStepReconcilerActionWithDiff[K, D]`
   - Keep deprecated type aliases and constructors for backward compat

5. **Update internal call sites**:
   - `samples/memcached-operator/controllers/memcached_controller.go`: Replace `NewObjectMultiPhase*` calls with `.As[D]()` / `.AsWithDiff[D]()`
   - Test files: Replace wrapper constructor calls with method calls

6. **Update tests** for wrapper types:
   - `action_unit_test.go`: Test `.As[D]()` methods instead of wrapper constructors
   - `multiphasestep_action_withdiff_unit_test.go`: Same

7. **Verify**: Run full test suite with `dagger call --src . test --withGotestsum`

8. **Document**: Update `CONTRIBUTING.md` with new minimum Go version and note that callers can now omit type parameters in many contexts.

---

## Validation Plan

1. `go build ./...` — must compile cleanly with Go 1.27
2. `go vet ./...` — no new warnings
3. `dagger call --src . test --withGotestsum` — all tests pass
4. Manual review: verify deprecated aliases still compile for backward compat
5. Check that `samples/memcached-operator` compiles and tests pass

---

## Open Questions

1. **v3 vs v4?** Bumping the Go version requirement is technically a breaking change. Should this be released as v4, or is requiring Go 1.27 for new v3.x releases acceptable?
2. **Deprecation timeline?** How many releases should the deprecated `ObjectMultiPhase*` types remain before removal?
3. **Sentinel `.As[D]()`?** The sentinel action (`DefaultSentinelAction`) doesn't have a step-object type parameter (it uses `client.Object` directly). Should we add an `.As[D]()` for consistency even though it's a no-op?
