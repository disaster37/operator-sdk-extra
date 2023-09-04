# Sample with memcached operator

We will rewrite the sample of memcached-operator with the extended framework.

## Create project

```bash
operator-sdk init --domain=example.com --repo=github.com/disaster37/operator-sdk-extra/testdata/memcached-operator

operator-sdk create api --group cache --version v1alpha1 --kind Memcached --resource --controller
```

## Use operator-sdk-extra

### Resources (CRD)

- First, you need to implements the interface `MultiPhaseObject` on `api/v1alpha1/memcached_types`. Just need to herit of BasicMultiPhaseObject