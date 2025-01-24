# Contribute

PR are awlays welcome here. Please use the `main` branch to start.

## Getting start

You need to the following tools:
  - [dagger cli](https://docs.dagger.io/install/)
  - [kubectl](https://kubernetes.io/fr/docs/tasks/tools/install-kubectl/)
  - [direnv](https://direnv.net/)

Create a fix or feature branch and then make your stuff on it.
After that, you are ready to make a Pull request. The PR will launch the CI and if all is right, it will publish the new catalog image that you and git owner will test before to merge the PR.

### Get binaries tools on local

```bash
dagger call -m operator-sdk --src . sdk get-cli export --path ./bin
```

## CI / tools

We use dagger.io to run local task or to run pipeline on CI.

### Run all step on local (without push image)

```bash
dagger call --src . ci
```

### Format code

```bash
dagger call -m golang --src . format export --path .
```

### Lint Golang project

```bash
dagger call -m golang --src . lint
```

### Vulnerability check

```bash
dagger call -m golang --src . vulncheck
```

### Run local test with envtest

```bash
dagger call --src . test --withGotestsum
```


### Generate SDK manifests

```bash
dagger call -m operator-sdk --src . sdk generate-manifests export --path .
```



