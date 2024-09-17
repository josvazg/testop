# Test Notes

## Kubebuilder

The main scaffolding creation is very nice. It gives a fully working skeleton of the project.
- So nice that we still left much of it in AKO, even parts we should have touched:
  - Top of source code Copyrights.
- Some nice parts as make run just works were removed and re-added (by me) later.

The per API CRD/Controller scaffolding creating is also very useful.
- Will come in very handy for new CRDs.
- Controllers are created within `internal/` which makes more sense TBH.

[Sample code do returns errors on reconciling](https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/getting-started/testdata/project/internal/controller/memcached_controller.go#L89), unlike our code.

The [sample code does update the status upon entry](https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/getting-started/testdata/project/internal/controller/memcached_controller.go#L95), as we used to do. That does not sound like a best practice.

## Reconciling learnings

### Logging

The logger is attached to the Go context already in samples: `log := log.FromContext(ctx)`

### Patching
Patching works fine for annotations lists with this approach:

```go
  r.Patch(ctx, modifiedObject, client.MergeFrom(originalObject)
```

Old annotations stay and the patched annotations are added or replaced.

### Reconcile calls

Creations get several reconciliation calls, the first includes the spec as it was initially set. The rest include changes to metadata around the object, but nothing usually actionable to be reconciled.

Best approach, just reconcile, but avoid noise on updates skipping if there are no spec changes.
