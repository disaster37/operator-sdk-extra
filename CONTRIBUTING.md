# Contributing

PRs are welcome here. Please start from the `main` branch and create a fix or feature branch before opening a Pull Request.

When the PR is opened, the CI pipeline runs automatically. If it passes, the new catalog image is published and ready to be tested by the maintainers before merge.

---

## Project overview

**operator-sdk-extra** is a companion library to [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) (the foundation of operator-sdk). It provides higher-level reconciler patterns on top of controller-runtime:

- **Multiphase reconciler** — orchestrates multiple "step" reconcilers to manage child Kubernetes resources using Server-Side Apply (SSA).
- **Remote reconciler** — manages external API resources (e.g., Elasticsearch indices, Cerebro roles) with 3-way merge via a last-applied-configuration stored in the CRD status.
- **Sentinel reconciler** — watches and manages child Kubernetes objects using Server-Side Apply (SSA).

The library does **not** fork or replace operator-sdk; it extends the ecosystem with reusable reconciler patterns, status management, helper utilities, and a structured test framework.

---

## Project structure

```
pkg/
  apis/              Status types (DefaultObjectStatus, PhaseName, conditions, finalizers)
  controller/
    multiphase/      Multiphase reconciler pattern (MultiPhaseStepReconciler, actions)
    remote/          Remote reconciler pattern (manages external API objects)
    sentinel/        Sentinel reconciler pattern (manages child K8s objects)
    reconciler.go    Base reconciler interface and shared logic
    helper.go        Shared controller helpers (network policy, misc utilities)
  helper/
    diff/            Generic struct diff utilities
    merge/           Mergo-based merge helpers
    slice/           Slice manipulation utilities
    map/             Map utilities
    zip/             Zip utilities
    crd/             CRD detection helpers
    log/             Structured logging helpers
    random/          Random string generator
    controller/      Rate limiter, controller-runtime helpers
    operator/        Operator-level helpers
  mock/              Generated mocks (go.uber.org/mock)
  object/            Core interfaces: MultiPhaseObject, RemoteObject, ObjectStatus
  test/              TestCase / TestStep integration test framework

cmd/crd/             CLI tool: clean @clean-tagged fields from CRD YAML
samples/
  memcached-operator/       Multiphase pattern sample
  elasticsearch-operator/   Remote pattern sample
ci/                  Dagger CI pipeline module
documentations/      Extended documentation (architecture, patterns)
```

---

## Diff / merge libraries

This project uses different diff strategies depending on the reconciler pattern:

| Pattern | Strategy | Library |
|---------|----------|---------|
| Multiphase reconciler | Server-Side Apply (SSA) | Built-in controller-runtime `Patch` with `client.Apply` |
| Sentinel reconciler | Server-Side Apply (SSA) | Built-in controller-runtime `Patch` with `client.Apply` |
| Remote reconciler | Client-side 3-way merge | [`github.com/disaster37/generic-objectmatcher`](https://github.com/disaster37/generic-objectmatcher) |

The remote reconciler stores a last-applied-configuration in the CRD status and uses `generic-objectmatcher` to compute diffs. The multiphase and sentinel patterns delegate conflict detection and field ownership entirely to the Kubernetes API server via SSA.

---

## Symbiosis with operator-sdk / controller-runtime

This library is designed to be used **inside** operators scaffolded with operator-sdk or kubebuilder:

1. Your CRD types implement the interfaces from `pkg/object/` (e.g., `MultiPhaseObject`, `RemoteObject`).
2. Your controller embeds one of the reconciler patterns from `pkg/controller/`.
3. The reconciler handles the full lifecycle: apply (via SSA) or create/update (via 3-way diff for remote), delete, status, conditions, finalizers.
4. You focus only on the business logic: what resources to create and how to build them.

The `samples/` directory contains full working operators that demonstrate this integration.

---

## Prerequisites

You need the following tools installed locally:

- **[Dagger CLI](https://docs.dagger.io/install/)** — used to run all local tasks and the CI pipeline
- **[kubectl](https://kubernetes.io/fr/docs/tasks/tools/install-kubectl/)**
- **[direnv](https://direnv.net/)** (recommended)

## Bootstrap local tooling

Download `controller-gen`, `setup-envtest` and other helper binaries into `./bin`:

```bash
dagger call -m operator-sdk --src . sdk get-cli export --path ./bin
```

---

## Local development commands

Everything runs through Dagger — no need to install Go linting, testing or formatting tools on your machine.

### Run the full CI pipeline (without pushing images)

```bash
dagger call --src . ci
```

This runs, in order: format → lint → vulncheck → tests → generate manifests.

### Format code

```bash
dagger call -m golang --src . format export --path .
```

### Lint code

```bash
dagger call -m golang --src . lint
```

### Vulnerability check

```bash
dagger call -m golang --src . vulncheck
```

### Run tests (with envtest)

```bash
# All tests
dagger call --src . test --withGotestsum

# Run a specific test by name pattern
dagger call --src . test --withGotestsum --run "TestMultiphaseReconciler"

# Skip specific tests
dagger call --src . test --withGotestsum --skip "TestSlowIntegration"

# Run tests from a specific path
dagger call --src . test --withGotestsum --path ./pkg/controller/multiphase/
```

---

## Testing

### Framework and tools

| Tool | Role |
|------|------|
| Go `testing` package | Test runner |
| `github.com/stretchr/testify` | Assertions (`assert`, `require`) |
| `go.uber.org/mock` | Mock generation (mocks in `pkg/mock/`) |
| `sigs.k8s.io/controller-runtime/tools/setup-envtest` | Real K8s API server for integration tests |
| `gotestsum` (via Dagger) | Pretty test output and JUnit reports |

### Test organization

- **Unit tests**: colocated with source files (`*_test.go`), use mocks from `pkg/mock/`.
- **Integration tests**: use `suite_test.go` with envtest to spin up a real K8s API server.
- **Test framework**: `pkg/test/` provides `TestCase` and `TestStep` abstractions for structured integration test scenarios.

### Code coverage — 100% target

This project targets **100% code coverage**. Every PR must maintain full coverage on the `pkg/` tree.

- Coverage is generated with `-coverprofile cover.out` over `./pkg/...`.
- CI uploads coverage to **Codecov** — regressions block merge.
- When adding new code, always add corresponding tests. If a code path is unreachable or defensive, add a test that exercises it or document why coverage is excluded.

To check coverage locally:

```bash
# Via Makefile
make test
go tool cover -func cover.out

# Via Dagger (preferred)
dagger call --src . test --withGotestsum
```

---

## Dagger pipeline options

For the full CI on GitHub Actions, the following contextual flags are passed automatically:

| Flag | Description |
|---|---|
| `--ci` | Enable CI mode (push images, commit generated files) |
| `--git-token env:GITHUB_TOKEN` | GitHub token for pushing manifests commit |
| `--codecov-token env:CODECOV_TOKEN` | Token for uploading coverage to Codecov |
| `--is-tag` | Set when triggered by a git tag |
| `--is-pull-request` | Set when triggered by a PR |
| `--git-branch` | Branch name (tags use `ref_name`, PRs use `pr.{number}`, otherwise `rc.{run_number}`) |

## Makefile targets (alternative without Dagger)

| Target | Command | Description |
|---|---|---|
| `make generate` | `controller-gen` | Generate DeepCopy / DeepCopyInto / DeepCopyObject methods |
| `make test` | `envtest go test` | Run tests under `./pkg/...` with envtest (K8s 1.23.x) and produce `cover.out` |
| `make controller-gen` | Download | Install `controller-gen v0.13.0` to `./bin/` |
| `make envtest` | Download | Install `setup-envtest` to `./bin/` |

---

## Guide for AI agents

This section helps AI coding agents (Copilot, Kilo, Cursor, etc.) contribute effectively to this project.

### How to contribute code

1. **Understand the pattern**: identify which reconciler pattern (multiphase, remote, sentinel) the change touches. Read the corresponding `documentations/*.md` file and the sample operator.
2. **Follow existing conventions**: all reconcilers use an action-based pattern (`Apply`, `Delete`, `Read` for multiphase/sentinel; `Create`, `Update`, `Delete` for remote). New multiphase/sentinel steps must use SSA with a `fieldManager` parameter.
3. **Interfaces first**: if adding a new feature, define or extend the interface in `pkg/object/` before implementing.
4. **Write tests alongside code**: never submit code without tests. Target 100% coverage on new code.
5. **Use mocks for unit tests**: generate mocks with `go.uber.org/mock` and place them in `pkg/mock/`.
6. **Run the full pipeline**: `dagger call --src . ci` must pass before submitting.

### How to do a code review

When reviewing a PR on this project, verify:

1. **Coverage**: does the PR maintain 100% coverage? Check that all new code paths are tested.
2. **SSA correctness** (multiphase/sentinel): verify that all expected objects have TypeMeta set (apiVersion+kind), use deterministic names (no generateName), and the correct field manager is passed. For remote reconciler PRs, verify the 3-way merge (current/expected/last-applied) is correctly handled.
3. **Interface compliance**: ensure CRD types still satisfy the interfaces in `pkg/object/`. Breaking changes must be caught.
4. **Status management**: reconcilers must correctly set conditions, phase, and status. Verify `SetCondition`, `SetPhase` calls.
5. **Error handling**: errors must be propagated correctly. Check that `ctrl.Result` and error returns follow the pattern (requeue on transient errors, don't requeue on permanent errors).
6. **No regression in samples**: if core library code changes, verify the `samples/` still compile and their tests pass.
7. **Backward compatibility**: this is a library — breaking changes to exported types/functions require a major version bump.
8. **Linting and formatting**: CI enforces this, but verify there are no `//nolint` directives added without justification.

### Key files to understand

| File | Why it matters |
|------|---------------|
| `pkg/controller/reconciler.go` | Base reconciler interface — all patterns implement this |
| `pkg/controller/helper.go` | Shared controller helpers (network policy) |
| `pkg/object/interfaces.go` | Core interfaces that CRD types must implement |
| `pkg/apis/status.go` | Status types used by all reconcilers |
| `pkg/test/` | Test framework — understand this to write good tests |
