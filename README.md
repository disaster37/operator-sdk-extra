# operator-sdk-extra

This framework is an extension of [operator-sdk](https://github.com/operator-framework/operator-sdk/tree/master). Its goal is to eliminate repetitive boilerplate when writing Kubernetes operators: finalizer management, status tracking, resource diff computation, and automatic create/update/delete orchestration.

After writing several operators, we identified 3 distinct use cases, each handled by a dedicated reconciler pattern:

| Pattern | Use case |
|---|---|
| **Multiphase** | Manage multiple K8s resources (ConfigMaps, Deployments, Services, etc.) from your own CRD |
| **Remote** | Manage external API resources (Elasticsearch roles, database users, etc.) from your own CRD |
| **Sentinel** | Watch standard K8s resources you do not own (Ingresses, Secrets, etc.) and derive new resources from their annotations |

## Patterns overview

### Multiphase Reconciler

Use it when your CRD creates and orchestrates a sequence of Kubernetes resources (e.g. a ConfigMap, then a Deployment). Each resource type is handled by its own **step reconciler**, executed sequentially by a **main reconciler**.

A 3-way merge patch engine powered by `k8s-objectmatcher` computes the diff between the current state, the expected state and the last applied configuration, producing create / update / delete operations automatically.

[Read the full documentation](documentations/multi-phase-reconciler.md)

### Remote Reconciler

Use it when your CRD manages resources external to Kubernetes via API calls (e.g. Elasticsearch roles, database users, cloud resources).

The pattern relies on a generic `RemoteExternalReconciler` that wraps your API client (`Build`, `Get`, `Create`, `Update`, `Delete`) and a 3-way merge diff that stores the last applied configuration as a compressed object in the CRD status.

[Read the full documentation](documentations/remote-reconciler.md)

### Sentinel Reconciler

Use it when you need to watch standard Kubernetes resources that your operator does not own — such as Ingresses, Secrets or ConfigMaps — and create derived resources based on their annotations or labels.

Unlike the other patterns, the Sentinel reconciler does **not** use a finalizer: child resources are cleaned up automatically by Kubernetes garbage collection through owner references.

[Read the full documentation](documentations/sentinel-reconciler.md)

## Project structure

```
operator-sdk-extra/
├── pkg/
│   ├── apis/                   Status type definitions (DefaultObjectStatus, shared types)
│   │   ├── multiphase/         DefaultMultiPhaseObjectStatus
│   │   ├── remote/             DefaultRemoteObjectStatus
│   │   └── shared/             PhaseName, ConditionName, FinalizerName
│   ├── controller/
│   │   ├── controller.go       Controller interface and setup helpers
│   │   ├── reconciler.go       Reconciler interface (finalizer, logger, recorder)
│   │   ├── multiphase/         Multiphase pattern implementation
│   │   ├── remote/             Remote pattern implementation (generic [k8sObject, apiObject, apiClient])
│   │   └── sentinel/           Sentinel pattern implementation (generic [k8sObject])
│   ├── helper/                 Utilities: diff, merge, slice ops, rate limiter, zip, CRD detection
│   ├── mock/                   Generated mocks for testing
│   ├── object/                 ObjectStatus, MultiPhaseObject, RemoteObject interfaces
│   └── test/                   TestCase / TestStep framework for integration tests
├── cmd/crd/                    CLI tool: clean-crd (remove @clean tagged fields from CRD YAML)
├── samples/
│   ├── memcached-operator/       Multiphase pattern sample
│   ├── elasticsearch-operator/   Remote pattern sample
│   └── ingress-sentinel-operator/ Sentinel pattern sample
├── documentations/             Pattern-specific documentation
├── ci/dagger/                  Dagger CI pipeline module
├── dagger.json                 Dagger module configuration
└── Makefile                    Local development targets
```

## Quick start

### Prerequisites

- Go **1.24+**
- [Dagger CLI](https://docs.dagger.io/install/)
- [kubectl](https://kubernetes.io/fr/docs/tasks/tools/install-kubectl/)
- [direnv](https://direnv.net/) (recommended)

### As a Go module dependency

```bash
go get github.com/disaster37/operator-sdk-extra/v2@latest
```

Refer to the [Multiphase](documentations/multi-phase-reconciler.md), [Remote](documentations/remote-reconciler.md) or [Sentinel](documentations/sentinel-reconciler.md) documentation to implement the pattern that fits your use case.

### Run the full CI pipeline locally (without pushing)

```bash
dagger call --src . ci
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full list of available commands.

## Ignore reconciliation

If you need to manually update a resource managed by an operator without it being reconciled back, add the following annotation:

```yaml
annotations:
  operator-sdk-extra.webcenter.fr/ignoreReconcile: "true"
```

## Key features

- **Finalizer management**: automatic add / remove on create / delete
- **Status tracking**: deep-copy + deferred-update only persists status when it actually changed
- **3-way merge diff**: current vs. expected vs. last-applied-configuration to compute precise create / update / delete lists
- **Rate limiter**: less aggressive than the default (`1s` to `1000s` exponential backoff, 10 QPS bucket)
- **Annotation-based reconciliation skip** via `operator-sdk-extra.webcenter.fr/ignoreReconcile`
- **Sentinel pattern**: watch any K8s resource, derive children, rely on GC for cleanup

## Samples

Each pattern has a reference operator implementation in `samples/` with its own `go.mod` and a `replace` directive pointing to the framework source:

| Pattern | Sample | What it does |
|---|---|---|
| **Multiphase** | [`samples/memcached-operator/`](samples/memcached-operator/) | Deploys Memcached by creating a ConfigMap and a Deployment from a CRD |
| **Remote** | [`samples/elasticsearch-operator/`](samples/elasticsearch-operator/) | Creates and reconciles Elasticsearch roles via the cluster API from a CRD |
| **Sentinel** | [`samples/ingress-sentinel-operator/`](samples/ingress-sentinel-operator/) | Watches Ingresses with annotations and derives ConfigMaps in target namespaces |

To build a sample, run `go build ./...` from its directory. The `replace` directive in its `go.mod` uses the framework from the local source tree (`../../`).

## License

Apache 2.0 — see [LICENSE](LICENSE).
