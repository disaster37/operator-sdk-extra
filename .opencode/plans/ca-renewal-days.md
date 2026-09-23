# Plan: CA-specific renewal window (`CARenewalDays`) in `operator-sdk-extra`

**Target repository:** `/projects/operator-sdk-extra` — Go module `github.com/disaster37/operator-sdk-extra/v3`.
**Go version:** `go 1.27.0` (verified in `go.mod`, line 3). *(Correction to the earlier external plan header, which said "Go 1.26"; the `go1.27` migration has already landed.)*
**Active development branch:** `v3` (verified via `.git/HEAD` → `ref: refs/heads/v3`). `origin/HEAD` points to `v2` and `CONTRIBUTING.md` says "main"; both are stale for the v3 module. **Branch from `v3`.**
**Type:** additive feature (minor version bump).

---

## 1. Source document status

An external plan/spec was located at
`/projects/operator-sdk-extra/.opencode/plans/ca-renewal-days.md`
(the same file this plan supersedes). It was **read and validated against the actual code**.

**Soundness verdict: SOUND.** Every grounding fact, line anchor, function signature,
and test helper referenced by the external plan was verified against the repo:

| Claim | Verified |
|---|---|
| `TLSSpec.RenewalDays` is the shared window; no `CARenewalDays` field exists | `pkg/controller/certificate/backend.go:219-221` |
| `CAValidityDays` is CA **lifetime**, not a renewal trigger | `backend.go:205-207`, `GetValidCADays` at `:387-396` |
| `GetValidRenewalDays` defaults to 30, clamps to `MaxRenewalDays` (36500) | `backend.go:398-411`, constants `DefaultRenewalDays=30` (`:41`), `MaxRenewalDays=36500` (`:50`) |
| `MaxValidityDays == MaxRenewalDays == 36500` | `backend.go:50,59` |
| `CANeedsRenewal` uses `GetValidRenewalDays` | `selfmanaged/backend.go:275-290` (window at `:288`) |
| `LeafNeedsChange` (single-leaf) uses `GetValidRenewalDays` | `selfmanaged/backend.go:393-453` (window at `:411`) |
| `LeafNeedsChange` (per-node) uses `GetValidRenewalDays` | `selfmanaged/pernode/backend.go:97-184` (window at `:121`) |
| cert-manager maps `RenewalDays` → `renewBefore` (leaf only) | `certmanager/backend.go:189-191` |
| Saga CA-need gate calls `CANeedsRenewal` then `CAContentChanged` | `rotation/rotation.go:277-292` |
| No `samples/` code uses `TLSSpec`/`NewTLSStep` | grep over `samples/` → no matches |
| Test helpers exist at the referenced lines | `makeTestCert` (`selfmanaged/backend_test.go:364`); `testSpecBuilder` (`:118`), `newStepProvider` (`:139`), `findSecret` (`:166`), `makeCertPEM` (`:177`), `newSecret` (`:197`) in `rotation_test.go` |

**Corrections applied to the external plan:**
1. Go version is `1.27.0`, not `1.26` (header metadata only; no design impact).
2. The cert-manager function is `setCommonCertificateSpec` (the external plan's §4.4 name is correct); the `renewBefore` mapping is at `certmanager/backend.go:189-191`.

`CARenewalDays`/`GetValidCARenewalDays`/`caRenewalDays` currently appear **only** in plan
files — no partial implementation exists in source.

---

## 2. Goal & motivation

The library has a **single shared renewal window** (`TLSSpec.RenewalDays`, default 30) used by both:

- **CA renewal** — `selfmanaged.CANeedsRenewal` (`selfmanaged/backend.go:288`)
- **Leaf renewal** — `selfmanaged.LeafNeedsChange` (`selfmanaged/backend.go:411`) and `pernode.LeafNeedsChange` (`pernode/backend.go:121`)

Operators that want a **CA-specific** window (e.g. `elasticsearch-operator`'s `spec.tls.caRenewalDays`) cannot express it in the library and must work around it locally (`transportCARenewalCustomizer`, `kibanaCARenewalCustomizer`, a `caSpec.RenewalDays` override in `apiCANeedsRegeneration`, and a local `pki.EffectiveCARenewalDays` guard).

**Goal:** add `TLSSpec.CARenewalDays` + `GetValidCARenewalDays()` and make `CANeedsRenewal` use it, so operators declare the CA window declaratively and delete their local workarounds.

**Non-goals:**
- No change to leaf renewal semantics (`RenewalDays` stays the leaf window).
- No change to cert-manager `renewBefore` (leaf-only; the CA is owned by the Issuer).
- No new backend, no operator-specific code, no new dependencies.
- No change to the rotation saga's phase machine.

---

## 3. Design

### 3.1 API additions (all additive)

`pkg/controller/certificate/backend.go` — add one field to `TLSSpec` (immediately
after `RenewalDays`, currently line 221):

```go
	// CARenewalDays is the CA-specific renewal window in days. When <= 0, the
	// shared RenewalDays window is used (backward compatible). Applies to the
	// self-managed backends (selfmanaged, selfmanaged/pernode), where the CA is
	// rotated by the rotation saga. Ignored by cert-manager (the CA is owned by
	// the Issuer) and BYO (no generation).
	CARenewalDays int `json:"caRenewalDays,omitempty"`
```

Update the `RenewalDays` doc comment (currently lines 219-220) to:

```go
	// RenewalDays is the window before expiry during which leaf renewal is
	// triggered. Defaults to 30 (see GetValidRenewalDays). It is also the
	// fallback CA renewal window when CARenewalDays is not set.
```

Add the getter immediately after `GetValidRenewalDays` (currently line 411):

```go
// GetValidCARenewalDays returns the CA renewal window in days.
//
// Resolution order:
//  1. CARenewalDays <= 0 -> GetValidRenewalDays(spec) (shared window; exact
//     pre-CARenewalDays behavior).
//  2. CARenewalDays > MaxRenewalDays -> clamped to MaxRenewalDays (defensive
//     overflow guard, mirroring GetValidRenewalDays).
//  3. CARenewalDays >= GetValidCADays(spec) -> falls back to
//     min(DefaultRenewalDays, CAValidityDays/2), floor 1. A window >= the CA
//     lifetime makes a freshly issued CA immediately due for renewal, which
//     would rotate the CA on every reconcile (perpetual rotation loop).
//
// The guard (3) only applies when CARenewalDays is explicitly set (> 0), so
// existing specs that only set RenewalDays keep their exact behavior.
func GetValidCARenewalDays(spec TLSSpec) int {
	if spec.CARenewalDays <= 0 {
		return GetValidRenewalDays(spec)
	}
	days := spec.CARenewalDays
	if days > MaxRenewalDays {
		days = MaxRenewalDays
	}
	if caDays := GetValidCADays(spec); caDays > 0 && days >= caDays {
		fallback := DefaultRenewalDays
		if half := caDays / 2; half < fallback {
			fallback = half
		}
		if fallback < 1 {
			fallback = 1
		}
		return fallback
	}
	return days
}
```

**Decisions (carried over from the validated external plan):**
- **Reuse `MaxRenewalDays`** rather than adding `MaxCARenewalDays = MaxRenewalDays`.
- **Guard lives in the getter**, not in `CANeedsRenewal`, so it is the single source of truth for the effective CA window. It mirrors the operator's existing formula exactly (`min(30, validity/2)`, floor 1).
- **No `ValidateContent` change.** `RenewalDays` is not validated today (only sanitized in the getter); `CARenewalDays` follows the same pattern (`<= 0` = unset).
- **Clamp is defensive.** Given `MaxValidityDays == MaxRenewalDays`, any value that hits the clamp also hits the guard, so the clamped value is never returned. The branch is still exercised by the huge-value test (coverage-safe) and protects against future divergence of the two constants.

### 3.2 Behavior matrix

| `RenewalDays` (leaf) | `CARenewalDays` (CA) | CA window used | Leaf window used |
|---|---|---|---|
| unset (30) | unset | 30 (shared) | 30 |
| 45 | unset | 45 (shared) | 45 |
| 30 | 90 | 90 | 30 |
| 90 | 30 | 30 | 90 |
| 30 | 0 / negative | 30 (shared) | 30 |
| 30 | 800, CA validity 730 | 30 (guard fallback) | 30 |
| 30 | 10, CA validity 12 | 6 (guard fallback = 12/2) | 30 |
| 30 | 1, CA validity 1 | 1 (guard floor) | 30 |
| 30 | 1<<62 | 30 (clamp → guard fallback) | 30 |

### 3.3 Where it applies

| Backend | CA renewal | Leaf renewal | Change |
|---|---|---|---|
| `selfmanaged` | `CANeedsRenewal` → **new getter** | `LeafNeedsChange` → `GetValidRenewalDays` | 1 line + doc |
| `selfmanaged/pernode` | shared CA, saga calls `CANeedsRenewal` → **new getter** | per-node certs → `GetValidRenewalDays` | comment only |
| `certmanager` | cert-manager owns the CA | `renewBefore` → `GetValidRenewalDays` | doc only |
| `byo` | user-owned | user-owned | none |
| `rotation` saga | calls `CANeedsRenewal` | calls `LeafNeedsChange` | none (flows through) |

---

## 4. File-by-file implementation

### 4.1 `pkg/controller/certificate/backend.go` (EDIT)

1. Add `CARenewalDays int \`json:"caRenewalDays,omitempty"\`` to `TLSSpec` after `RenewalDays`, with the doc comment from §3.1.
2. Update the `RenewalDays` doc comment to state it is the **leaf** window and the CA fallback (see §3.1).
3. Add `GetValidCARenewalDays` immediately after `GetValidRenewalDays` (line 411).

### 4.2 `pkg/controller/certificate/selfmanaged/backend.go` (EDIT)

1. `CANeedsRenewal` (line 288): `GetValidRenewalDays` → `GetValidCARenewalDays`.
2. Update its doc comment (line 279): `within GetValidRenewalDays(spec)` → `within GetValidCARenewalDays(spec)`.
3. `LeafNeedsChange` (line 411): **unchanged** (leaf window). Add a one-line comment:
   ```go
   // Leaf expiry uses the leaf window; CA renewal uses GetValidCARenewalDays
   // in CANeedsRenewal.
   ```

### 4.3 `pkg/controller/certificate/selfmanaged/pernode/backend.go` (EDIT)

1. `LeafNeedsChange` (line 121): **unchanged**. Add the same clarifying comment: per-node certs are leaves; the shared CA is renewed by the saga via `CANeedsRenewal`.

### 4.4 `pkg/controller/certificate/certmanager/backend.go` (EDIT — comment only)

1. `setCommonCertificateSpec` (lines 189-191): **unchanged** (`renewBefore` is leaf-only).
2. Add a comment above it:
   ```go
   // renewBefore applies to the leaf Certificate only. CARenewalDays is not
   // mapped: the CA is owned by the Issuer (or is self-signed per Certificate).
   ```

### 4.5 `pkg/controller/certificate/rotation/rotation.go` (EDIT — comment only)

1. In the CA-need block (lines 277-292), extend the comment:
   ```go
   // 2. CA need (expiry against GetValidCARenewalDays, or content drift).
   ```
2. No logic change: `CANeedsRenewal` already encapsulates the window.

### 4.6 `documentations/tls-and-workflow.md` (EDIT)

1. **TLSSpec snippet** (lines 70-84): add `CARenewalDays int // CA renewal window (default = RenewalDays)` after `RenewalDays`.
2. **Field table** (lines 86-93): add a row:
   `| CARenewalDays | int | RenewalDays | selfmanaged, selfmanaged/pernode | CA-specific renewal window; GetValidCARenewalDays(spec). Falls back to RenewalDays when unset. Guarded against >= CAValidityDays. |`
   Update the `RenewalDays` row to say "leaf renewal window (also the CA fallback)".
3. **CA renewal bullet** (lines 201-202): change `GetValidRenewalDays` → `GetValidCARenewalDays` and add: "Leaf drift/expiry keeps using `GetValidRenewalDays`."
4. **Saga step description** (line 404): note `CANeedsRenewal` uses the CA window.
5. Add a short subsection **"CA vs leaf renewal windows"** documenting the resolution order, the guard, and cert-manager/BYO non-applicability.

---

## 5. Validation rules & error handling

- **Validation is by sanitization, not rejection.** `CARenewalDays` mirrors `RenewalDays`: `<= 0` means "unset" (falls back to the shared window), and values `> MaxRenewalDays` are clamped. No new error is introduced by this feature. This is consistent with how the library already handles `RenewalDays`/`ValidityDays` (no `ValidateContent` involvement).
- **Guard against perpetual rotation.** `CARenewalDays >= GetValidCADays(spec)` (only when explicitly `> 0`) silently falls back to `min(DefaultRenewalDays, CAValidityDays/2)` (floor 1). This is the single behavior decision of the feature; the alternative (a freshly issued CA immediately due for renewal → rotate every reconcile) is strictly worse.
- **No error surface change.** `CANeedsRenewal` keeps its `(bool, error)` signature; existing error paths (nil/missing/malformed CA secret) are untouched. The saga still propagates `CANeedsRenewal` errors unchanged (`rotation/rotation.go:280-283`).

---

## 6. Edge cases

| Case | Behavior |
|---|---|
| `CARenewalDays` unset / 0 / negative | Falls back to `GetValidRenewalDays` (exact old behavior). |
| `CARenewalDays` > `MaxRenewalDays` | Clamped, then guard applies (result: guard fallback). |
| `CARenewalDays >= CAValidityDays` | Guard fallback `min(30, CAValidity/2)`, floor 1. |
| `CARenewalDays` set, `RenewalDays` unset | CA uses `CARenewalDays`; leaf uses 30. |
| Both set | CA uses `CARenewalDays`; leaf uses `RenewalDays`. |
| CA validity defaulted (2× leaf = 730) | Guard uses the resolved `GetValidCADays`. |
| `CARenewalDays` set on cert-manager | Ignored (`renewBefore` stays leaf-only). |
| `CARenewalDays` set on BYO | Ignored (no generation). |
| `CARenewalDays` set on per-node | Applies to the shared CA (saga); per-node leaves keep the leaf window. |
| CA missing / malformed | Unchanged (`CANeedsRenewal` returns true / error). |
| CA already expired (`NotAfter` in past) | Unchanged: `certsNeedRenewal` uses `now.After(NotAfter.Add(-window))`, so expired CA is within window → renew. |
| CA expiring **exactly** at the boundary (`now == NotAfter - window`) | Not renewed: `After` is strict `>` (existing, unchanged semantics; consistent with `RenewalDays`). |
| Renewal attempted **outside** the window | `CANeedsRenewal` returns `false` → no rotation. |
| **Timezone / DST / leap year** | `time.Time` comparisons are absolute instants; `x509.Certificate.NotAfter` is a UTC instant. "days" is defined as `days * 24 * time.Hour` (no calendar-day/DST/leap semantics) — identical to the existing `GetValidRenewalDays`/`BuildCASigner` convention. No special handling. |
| **Clock skew** | No clock-skew tolerance exists today and none is added; `CANeedsRenewal` compares `now` (caller-supplied `time.Now()`) against `NotAfter`. Unchanged behavior, out of scope. |
| **Multiple CAs / CA bundle** | `parsePEMCerts` + `certsNeedRenewal` return true if *any* PEM cert is within window. `CANeedsRenewal` is only invoked at phase `""` (steady state, single CA), so this is unchanged. |
| **Zero-value config / backward compat** | The library is stateless (no DB). "Existing configs lacking the field" = operators whose computed `TLSSpec` leaves `CARenewalDays == 0` (zero value). `GetValidCARenewalDays` returns `GetValidRenewalDays(spec)` for `<= 0`, so renewal decisions are byte-identical to today. |

---

## 7. Test plan (100% coverage target on `pkg/`)

### 7.1 `pkg/controller/certificate/backend_test.go` (extend — mirrors existing `TestGetValidRenewalDays*` style)

| Test | Input | Expected |
|---|---|---|
| `TestGetValidCARenewalDaysDefault` | `TLSSpec{}` | 30 |
| `TestGetValidCARenewalDaysFallsBackToShared` | `{RenewalDays: 45}` | 45 |
| `TestGetValidCARenewalDaysCustom` | `{RenewalDays: 30, CARenewalDays: 90}` | 90 |
| `TestGetValidCARenewalDaysZero` | `{CARenewalDays: 0, RenewalDays: 45}` | 45 |
| `TestGetValidCARenewalDaysNegative` | `{CARenewalDays: -5, RenewalDays: 45}` | 45 |
| `TestGetValidCARenewalDaysGuard` | `{CARenewalDays: 800, CAValidityDays: 730}` | 30 |
| `TestGetValidCARenewalDaysGuardHalf` | `{CARenewalDays: 10, CAValidityDays: 12}` | 6 |
| `TestGetValidCARenewalDaysGuardFloor` | `{CARenewalDays: 1, CAValidityDays: 1}` | 1 |
| `TestGetValidCARenewalDaysHugeValue` | `{CARenewalDays: 1 << 62}` | 30 (clamp branch + guard) |
| `TestGetValidCARenewalDaysGuardUsesDefaultCAValidity` | `{CARenewalDays: 800, LeafValidityDays: 365}` (CA validity defaults to 730) | 30 |

### 7.2 `pkg/controller/certificate/selfmanaged/backend_test.go` (extend)

Existing `TestCANeedsRenewal_*` (lines 527-591) must stay green unchanged — they set only `RenewalDays`, which now falls back through `GetValidCARenewalDays` → `GetValidRenewalDays`. Add (using `makeTestCert` at line 364):

| Test | Setup | Expected |
|---|---|---|
| `TestCANeedsRenewal_UsesCAWindow` | CA expires in 60d; `RenewalDays: 30`, `CARenewalDays: 90` | `true` (inside CA window, outside leaf window) |
| `TestCANeedsRenewal_OutsideCAWindow` | CA expires in 60d; `RenewalDays: 90`, `CARenewalDays: 30` | `false` (outside CA window, inside leaf window) |
| `TestCANeedsRenewal_CAWindowGuard` | CA expires in 60d; `CARenewalDays: 800`, `CAValidityDays: 730` | `true` (guard fallback 30 → inside) |
| `TestCANeedsRenewal_CAWindowClamped` | CA expires in 1h; `CARenewalDays: 1 << 62` | `true` (no overflow; renewal still triggers) |

### 7.3 `pkg/controller/certificate/rotation/rotation_test.go` (extend)

Provider-based tests using `newStepProvider` (line 139) with a custom `TLSSpecProviderFunc`:

| Test | Setup | Expected |
|---|---|---|
| `TestRead_Saga_CAWindowTriggersRotation` | CA expires in 60d; spec `{RenewalDays: 30, CARenewalDays: 90}`; leaf fresh | `data["rotationRenewed"] == true`, `LayerSignals.CARotated == true`, phase `Rotate` |
| `TestRead_CAWindowOutside_NoCASaga` | CA expires in 60d; spec `{RenewalDays: 90, CARenewalDays: 30}`; leaf fresh | no `rotationRenewed`; steady state (expected == current) |
| `TestRead_CAWindowGuard_PreventsPerpetualRotation` | Fresh CA (expires in 700d); `{CARenewalDays: 800, CAValidityDays: 730}` | no `rotationRenewed` (guard fallback 30 → outside) |

Use `makeCertPEM` (line 177), `newSecret` (line 197), `findSecret` (line 166). CA secret name `test-tls-ca`, leaf `test-tls` (from `testSpecBuilder`, line 118).

### 7.4 `pkg/controller/certificate/selfmanaged/pernode/backend_test.go` (extend)

| Test | Setup | Expected |
|---|---|---|
| `TestPerNodeLeafNeedsChange_IgnoresCARenewalDays` | Node cert expires in 60d; `{RenewalDays: 30, CARenewalDays: 90}` | `LeafExpiring` (leaf window 30 → inside) |
| `TestPerNodeLeafNeedsChange_CAWindowDoesNotExtendLeaf` | Node cert expires in 60d; `{RenewalDays: 30, CARenewalDays: 10}` | `LeafExpiring` (leaf window still 30) |

### 7.5 `pkg/controller/certificate/certmanager/backend_test.go` (extend)

| Test | Setup | Expected |
|---|---|---|
| `TestCertManagerBackendCARenewalDaysIgnored` | `{RenewalDays: 20, CARenewalDays: 90}` | `spec.renewBefore == "480h"` (20d, unchanged) |

### 7.6 Coverage notes

- Every new branch in `GetValidCARenewalDays` is covered by §7.1.
- The clamp branch is covered by `TestGetValidCARenewalDaysHugeValue` even though its result is subsumed by the guard.
- No existing test should need modification; if any fails, it indicates a backward-compatibility break and must be investigated, not adjusted.

---

## 8. Ordered implementation steps (with Definition of Done)

1. **`backend.go`** — add `CARenewalDays`, `GetValidCARenewalDays`, update `RenewalDays` doc. *DoD:* `go build ./pkg/controller/certificate/...`; existing `backend_test.go` green.
2. **`selfmanaged/backend.go`** — switch `CANeedsRenewal` to the new getter; update docs/comments. *DoD:* build green; existing `TestCANeedsRenewal_*` green.
3. **`pernode/backend.go`** + **`certmanager/backend.go`** — clarifying comments only. *DoD:* build green.
4. **`rotation/rotation.go`** — comment update only. *DoD:* build green.
5. **Tests** — §7.1-7.5. *DoD:* 100% coverage on `pkg/controller/certificate/...`; all existing tests green.
6. **Docs** — §4.6. *DoD:* doc renders; field table complete.
7. **Full CI** — `dagger call --src . ci`. *DoD:* format, lint, vulncheck, tests, manifests all green.

---

## 9. Verification commands

```bash
# Build (plain Go)
go build ./pkg/controller/certificate/...

# Targeted tests (Dagger, per AGENTS.md / CONTRIBUTING.md)
dagger call --src . test --withGotestsum --run "TestGetValidCARenewalDays"
dagger call --src . test --withGotestsum --run "TestCANeedsRenewal"
dagger call --src . test --withGotestsum --run "TestRead_Saga_CAWindow"
dagger call --src . test --withGotestsum --run "TestPerNodeLeafNeedsChange"
dagger call --src . test --withGotestsum --run "TestCertManagerBackendCARenewalDaysIgnored"

# All certificate tests, failures only
dagger call --src . test --withGotestsum --path ./pkg/controller/certificate/ \
  2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200

# Coverage (100% target on pkg/)
dagger call --src . test --withGotestsum export --path cover.out
go tool cover -func cover.out | grep -E "certificate"

# Full CI (format → lint → vulncheck → tests → generate manifests)
dagger call --src . ci
```

Plain-Go fallbacks (if Dagger is unavailable for unit-level work): `go vet ./pkg/controller/certificate/...` and `go test ./pkg/controller/certificate/...`.

---

## 10. Branch & PR workflow

1. `git fetch origin`
2. `git checkout -b feat/ca-renewal-days origin/v3` — **from `v3`**, not `main`/`v2`.
3. Implement steps 1-7; run the full CI locally (`dagger call --src . ci`) before opening the PR.
4. Open a PR **against `v3`** titled `feat: add CA-specific renewal window (CARenewalDays)`.
5. After merge, tag the release. Semver: additive feature → **`v3.1.0`** (the repo's history has shipped features in patch tags, so `v3.0.8` is acceptable if the maintainer prefers; decide at release time).
6. **Consumer follow-up (separate PR, out of scope here)** — in `elasticsearch-operator`: bump `go.mod` to the new tag, map `caRenewalDays` → `TLSSpec.CARenewalDays`, and delete `transportCARenewalCustomizer`, `kibanaCARenewalCustomizer`, the `caSpec.RenewalDays` override in `apiCANeedsRegeneration`, and `pkg/pki.EffectiveCARenewalDays`.

---

## 11. Out of scope / non-goals

- Leaf renewal semantics (unchanged).
- cert-manager `renewBefore` (unchanged; CA is Issuer-owned).
- The rotation saga phase machine (unchanged).
- New backends, new dependencies, operator-specific code.
- `elasticsearch-operator` cleanup (separate follow-up PR, see §10.6).
- Clock-skew tolerance (no such tolerance exists today).
- Release-tag choice (`v3.1.0` vs `v3.0.8`) — release-time decision.

---

## 12. Risks & open questions

| # | Item | Recommendation |
|---|---|---|
| R1 | **Guard placement.** Guard in `GetValidCARenewalDays` vs. a separate `EffectiveCARenewalDays` vs. operator-side. | Guard in the getter (single source of truth; lets the operator delete its local guard). |
| R2 | **`MaxCARenewalDays` constant.** | Reuse `MaxRenewalDays`; add the alias only if the maintainer wants a dedicated symbol. |
| R3 | **Guard is a behavior choice.** A user who intentionally sets `CARenewalDays >= CAValidityDays` gets a silent fallback. | Documented in the getter and field comment; perpetual CA rotation is strictly worse. |
| R4 | **Clamp is subsumed by the guard** while `MaxValidityDays == MaxRenewalDays`. | Keep as defensive; covered by the huge-value test. |
| R5 | **Branch base.** `CONTRIBUTING.md` says `main`; `origin/HEAD` is `v2`; active v3 work is on `v3`. | Branch from `v3`; optionally fix `CONTRIBUTING.md` in a separate docs PR. |
| R6 | **Release tag.** Patch vs. minor. | `v3.1.0` (semver-correct for an additive feature); maintainer decides. |

---

**Definition of Done (summary):** `CARenewalDays` + `GetValidCARenewalDays` implemented; `CANeedsRenewal` uses the CA window; leaf/cert-manager/BYO behavior unchanged; all new branches covered (100% on `pkg/`); docs updated; `dagger call --src . ci` green; PR opened against `v3` from `feat/ca-renewal-days`.
