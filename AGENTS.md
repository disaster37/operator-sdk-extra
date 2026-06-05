# Agent Guide

## Running tests (with filtered output)

When running tests, use `gotestsum` for pretty output and filter to failures only:

```bash
dagger call --src . test --withGotestsum 2>&1 | grep -E "(FAIL|build failed|\.go:[0-9]+:|panic|--- FAIL)" | head -200
```

To run a specific test:

```bash
dagger call --src . test --withGotestsum --run "TestName"
```