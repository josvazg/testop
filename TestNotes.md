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

The logger is attached to the Go context already in samples: `log := log.FromContext(ctx)`
