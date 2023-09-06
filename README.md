# operator-sdk-extra

This framework is an extention of [operator-sdk](https://github.com/operator-framework/operator-sdk/tree/master) framework.
The goal is to not rewrite all operator logic each time you implement new operator: Add finalizer, handle the deletion, read resources, compute expected resources and diffing resources to know if operator need update or create resources.

After write some operators, we know 3 different use case:
 - Multi phase reconciler: Use it when you need to handle some K8s resources from your own CRD (configmap, deployment, ingress, etc.)
 - Remote reconciler: Use it when you need to handle remote resources (not K8s resource) from API (create role, user, on databse server for example)
 - Sentinel reconciler: Use it when you need to track K8s resources that you doesn't own CRD (create some resources from ingress, etc ...)

 ## Use cases

We will treat each use case with a real operator sample.

### Multi phase reconciler

[Read the dedicated documentation](documentations/multi-phase-reconciler.md)

### Remote reconciler

[Read the dedicated documentation](documentations/remote-reconciler.md)

### Sentinel reconciler

[Read the dedicated documentation](documentations/sentinel-reconciler.md)



## Tips

### Ignore reconcile

If you should to manually change ressources handled by operator, it can be usefull to ignore reconcilation on them. To to that, you can add the following annotation: `operator-sdk-extra.webcenter.fr/ignoreReconcile: "true"`