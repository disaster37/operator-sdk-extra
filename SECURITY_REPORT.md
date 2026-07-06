# Security Audit Report — operator-sdk-extra

**Generated:** 2026-07-06T21:06:56+00:00
**Tooling:** gosec, golangci-lint (errcheck/govet/staticcheck/unused), staticcheck, manual grep analysis
**Scope:** `pkg/`, `cmd/`, `samples/` (Go source files, excluding generated mocks and deepcopy)

---

## Summary

| Severity | Count | Description |
|----------|-------|-------------|
| HIGH     | 1     | Weak random number generator |
| MEDIUM   | 3     | File inclusion via variable, context.Background() in production, unsafe type assertions |
| LOW      | 12    | Unused fields/types, unchecked error returns, code style issues |

---

## HIGH Severity Findings

### BUG-H01: Weak Random Number Generator (CWE-338)

- **File:** `pkg/helper/random.go:11`
- **Tool:** gosec (G404)
- **Confidence:** MEDIUM

```go
seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
```

**Issue:** Uses `math/rand` instead of `crypto/rand` for random string generation. The `math/rand` package is not cryptographically secure and its output is predictable if the seed (UnixNano timestamp) is known.

**Impact:** If `RandomString` is used for generating tokens, passwords, session IDs, or any security-sensitive identifier, an attacker can predict the generated values.

**Fix:** Replace with `crypto/rand`:
```go
import "crypto/rand"

func RandomString(length int) string {
    b := make([]byte, length)
    if _, err := rand.Read(b); err != nil {
        panic(err)
    }
    // Map bytes to charset
    for i := range b {
        b[i] = charset[int(b[i])%len(charset)]
    }
    return string(b)
}
```

**Note:** This approach introduces a small bias due to modulo operation when `len(charset)` doesn't divide 256 evenly. For production-grade security, use `encoding/base64` with `crypto/rand` or a proper UUID library.

---

## MEDIUM Severity Findings

### BUG-M01: Potential File Inclusion via Variable (CWE-22)

- **File:** `pkg/test/equal.go:23`
- **Tool:** gosec (G304)
- **Confidence:** HIGH

```go
f, err := os.ReadFile(expectedYamlFile)
```

**Issue:** File path comes from a function parameter without any validation or sanitization. An attacker could potentially supply a path with `../` traversal to read arbitrary files on the system.

**Impact:** Path traversal could expose sensitive files if this function is exposed through an API or CLI entry point that accepts user-controlled paths.

**Fix:** Validate and restrict the file path:
```go
// Option 1: Resolve to absolute and check against allowed directory
absPath, err := filepath.Abs(expectedYamlFile)
if err != nil { ... }
if !strings.HasPrefix(absPath, allowedBaseDir) { ... }

// Option 2 (Go >=1.24): Use os.Root
root, err := os.OpenRoot(allowedBaseDir)
f, err := root.Open(expectedYamlFile)
```

### BUG-M02: context.Background() in Production Code

- **File:** `pkg/controller/helper.go:172,179,199`
- **Tool:** Manual analysis

```go
if err := c.Get(context.Background(), ...); err != nil { ... }
if err = c.Create(context.Background(), expectedNetworkPolicy); err != nil { ... }
if err = c.Update(context.Background(), networkPolicy); err != nil { ... }
```

**Issue:** `EnsureNetworkPolicyForWebhook` uses `context.Background()` instead of accepting a `context.Context` parameter. This means:
- No deadline/cancellation propagation from the caller
- Network operations can hang indefinitely with no timeout
- Cannot be cancelled when the reconciler shuts down

**Impact:** If the Kubernetes API server is slow or unreachable, these calls can block the reconciler goroutine indefinitely.

**Fix:** Accept a `context.Context` parameter:
```go
func EnsureNetworkPolicyForWebhook(ctx context.Context, c client.Client, ...) error {
```

### BUG-M03: Unsafe Type Assertions (Panic Risk)

- **Files:** 
  - `pkg/controller/helper.go:93` — `om.Interface().(metav1.ObjectMeta)`
  - `pkg/controller/helper.go:198` — `patchResult.Patched.(*networkv1.NetworkPolicy)`
  - `pkg/helper/ssa/diff.go:18` — `obj.DeepCopyObject().(client.Object)`
  - `pkg/helper/object.go:11` — `reflect.ValueOf(o).Interface().(dstType)`
  - `pkg/controller/sentinel/helper.go:26` — `valueField.Index(i).Addr().Interface().(k8sObject)`
  - `pkg/controller/sentinel/helper.go:69` — `reflect.New(...).Interface().(objectType)`
  - `pkg/controller/multiphase/controller_test.go:121` — `data["lastGeneration"].(int64)`

**Issue:** Single-return type assertions (`x.(T)`) panic if the assertion fails. When the underlying type doesn't match, these crash the entire operator process. The reflection-based assertions in `helper.go`, `object.go`, and `sentinel/helper.go` are particularly concerning because reflection bypasses compile-time type checking.

**Impact:** A type mismatch at runtime (e.g., unexpected object type from the API server) would cause a panic, crashing the operator. This could be triggered by:
- API version changes in Kubernetes
- Custom resource definition updates
- Unexpected object serialization

**Fix:** Use comma-ok patterns where possible:
```go
om, ok := rv.FieldByName("ObjectMeta").Interface().(metav1.ObjectMeta)
if !ok {
    return metav1.ObjectMeta{}, errors.New("field is not ObjectMeta")
}
```

Alternatively, document that these are intentional panics for unrecoverable programming errors by adding `//nolint:forcetypeassert` comments with justification.

---

## LOW Severity Findings

### Unchecked Error Returns (errcheck)

| File | Line | Issue |
|------|------|-------|
| `pkg/controller/multiphase/multiphasestep_action_unit_test.go` | 106, 143, 183 | `clientgoscheme.AddToScheme` return ignored |
| `pkg/helper/log_unit_test.go` | 34, 54 | `os.Unsetenv` return ignored |
| `pkg/helper/operator_unit_test.go` | 60 | `os.Unsetenv` return ignored |

### Unused Types & Fields (unused)

| File | Issue |
|------|-------|
| `pkg/controller/controller_unit_test.go` | `addIndexerErr`, `addWebhookErr` fields |
| `pkg/controller/multiphase/multiphasestep_action_unit_test.go` | `mockMultiPhaseRead`, `mockLogrusEntry` |
| `pkg/controller/remote/remote_reconciler_unit_test.go` | `createRes`, `createErr`, `updateRes`, `updateErr` |

### Code Style Issues

- `pkg/controller/helper.go:85,99,115`: Inline constant `reflect.Ptr` should be used
- `pkg/test/assertions.go:62,83,104`: Merge variable declaration with assignment

---

## ZIP Bomb Vulnerability (Additional Finding)

- **File:** `pkg/helper/zip.go:43-73`
- **Tool:** Manual analysis

**Issue:** `UnZipBase64Decode` decodes and unzips incoming data without any size limit. A malicious or corrupted `lastAppliedConfiguration` field in a CRD status could be a ZIP bomb — a small compressed payload that expands to gigabytes of data.

**Impact:** An attacker who gains write access to a CRD's status field could inject a crafted ZIP bomb that:
1. Exhausts memory when `io.ReadAll` reads the decompressed data
2. Causes the operator's Go process to OOM and crash repeatedly
3. Creates a denial-of-service condition by crashing the operator on every reconcile loop

**Fix:** Limit the total decompressed size:
```go
const maxDecompressedSize = 10 * 1024 * 1024 // 10 MiB
limitedReader := io.LimitReader(f, maxDecompressedSize+1)
unzippedFileBytes, err := io.ReadAll(limitedReader)
if len(unzippedFileBytes) > maxDecompressedSize {
    return errors.New("decompressed data exceeds size limit")
}
```

---

## Positive Findings (No Issues Found)

- No hardcoded credentials, API keys, or secrets found
- No use of `os.Exec`, `exec.Command`, or `syscall.Exec`
- No insecure TLS configurations (`InsecureSkipVerify`, `InsecureTLS`)
- No use of `unsafe` package
- No SQL/NoSQL injection vectors (project uses Kubernetes API server, not databases)
- No HTTP client configurations with insecure settings
- No race conditions detected (no `sync.Mutex` misuse, but also no manual synchronization primitives used in production code — relies on controller-runtime concurrency model)
- Proper use of `defer f.Close()` for resource cleanup in `pkg/helper/zip.go`

---

## Recommendations (Priority Order)

1. **Replace `math/rand` with `crypto/rand`** in `pkg/helper/random.go` if the function is ever used for security-sensitive identifiers.

2. **Add path validation** to `pkg/test/equal.go` to prevent path traversal via user-supplied YAML file paths.

3. **Accept `context.Context`** in `EnsureNetworkPolicyForWebhook` (`pkg/controller/helper.go`) instead of hardcoding `context.Background()`.

4. **Add decompressed size limit** in `UnZipBase64Decode` (`pkg/helper/zip.go`) to prevent ZIP bomb attacks.

5. **Audit unsafe type assertions** and either:
   - Replace with comma-ok patterns for recoverable errors, OR
   - Document intentional panics with `//nolint:forcetypeassert` comments

6. **Remove unused types and fields** from test files to reduce maintenance burden and confusion.

7. **Add error handling** for `os.Unsetenv` calls in test files.
