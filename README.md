# vikunja-operator

A Kubernetes operator for managing [Vikunja](https://vikunja.io) resources -
projects, tasks, teams and labels - declaratively, via Custom Resources.

This is an independent, unofficial project. It is not affiliated with,
endorsed by, or part of Vikunja, [vikunja.io](https://vikunja.io), or
[`go-vikunja/vikunja`](https://github.com/go-vikunja/vikunja). The Vikunja
name is used only to identify the API this operator talks to.

## Description

vikunja-operator lets you manage a Vikunja instance's projects, tasks, teams
and labels as Kubernetes objects, reconciled against the
[Vikunja API v2](https://vikunja.io/docs/api-v2/). It defines five CRDs in the
`vikunja.paintedsky.io/v1alpha1` API group:

- **VikunjaInstance** - points at a Vikunja API base URL and a Secret holding
  an API token; other resources reference it by name. Its controller checks
  that the instance is reachable and the token valid, and reports that as a
  `Ready` condition.
- **Project** - a Vikunja project. Supports nesting via `parentProjectRef`,
  pointing at another Project resource in the same namespace.
- **Label** - a Vikunja label.
- **Team** - a Vikunja team, including declarative membership
  (`spec.members`, each with a `username` and optional `admin` flag). The
  operator adds and removes members to match spec, but never removes the user
  the instance's API token belongs to.
- **Task** - a Vikunja task belonging to a Project (`spec.projectRef`), with
  labels (`spec.labelRefs`, referencing Label resources) and assignees
  (`spec.assignees`, Vikunja usernames) reconciled to match spec.

Each resource is finalized: deleting the Kubernetes object deletes the
corresponding object in Vikunja first. Every resource reports a `Ready`
condition in `status.conditions`, and dependent resources (a Project's
parent, a Task's Project/Labels) wait for their dependency to be `Ready`
before creating anything.

See `config/samples/` for a complete example, including the Secret holding
the API token.

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) v5+ (`make` will download it into `bin/` if needed).
- Access to a Kubernetes v1.11.3+ cluster. Talos Linux works as-is; this operator does not need hostPath, privileged pods, or Talos extensions.

### Deploy with Kustomize (Talos or any cluster)

Talos nodes are immutable and do not run Docker. Build the controller image on your workstation (or let GitHub Actions / GoReleaser do it), push it to a registry the cluster can pull from, then apply the Kustomize packages with `kubectl`.

**1. Point kubectl at the cluster**

```sh
talosctl kubeconfig --nodes <control-plane-ip> --talosconfig ~/.talos/config
kubectl get nodes
```

**2. Publish a controller image**

After a tagged release, GoReleaser publishes a multi-arch image to GHCR:

```text
ghcr.io/go-paintedsky/vikunja-operator:<tag>
```

For a local or unreleased build, push an image the cluster can pull:

```sh
export IMG=ghcr.io/go-paintedsky/vikunja-operator:dev
make docker-build docker-push IMG=$IMG
```

Homelab registry notes for Talos:

- The cluster must pull the image over the network. There is no `kind load` / `ctr import` equivalent you should rely on for Talos workers.
- For a private GHCR package, either make the package public or give the cluster credentials. The Talos-native option is machine-config registry auth:

```yaml
machine:
  registries:
    config:
      ghcr.io:
        auth:
          username: <github-user>
          password: <github-pat-with-read-packages>
```

  The Kubernetes option is a `docker-registry` Secret plus `imagePullSecrets` (a commented patch is in `config/overlays/homelab/kustomization.yaml`).
- For a local registry, add a Talos `registries.mirrors` entry so nodes pull through that mirror.

**3. Pin the image and apply CRDs + operator**

`config/default` already includes the CRDs, RBAC, namespace, and controller. The homelab overlay only swaps the image:

```sh
# Edit config/overlays/homelab/kustomization.yaml and set images.newTag
# to a release tag (v0.1.0) or the tag you pushed (dev).
kubectl apply -k config/overlays/homelab
```

To install only the CRDs (for example before applying your own overlay):

```sh
kubectl apply -k config/crd
```

To apply the stock package and set the image without the overlay:

```sh
kubectl apply -k config/crd
cd config/manager && kustomize edit set image controller=${IMG}
cd ../..
kubectl apply -k config/default
```

`make install` / `make deploy IMG=$IMG` are wrappers around those same Kustomize packages.

**4. Confirm the controller is up**

```sh
kubectl -n vikunja-operator-system get deploy,pods
kubectl -n vikunja-operator-system logs deploy/vikunja-operator-controller-manager -c manager
```

**5. Create Vikunja resources**

Edit `config/samples/vikunja_v1alpha1_vikunjainstance.yaml` so the Secret holds a real API token and `baseURL` points at your Vikunja, then:

```sh
kubectl apply -k config/samples/
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the operator and CRDs:**

```sh
kubectl delete -k config/overlays/homelab
# or: kubectl delete -k config/default
```

`make undeploy` / `make uninstall` do the same via Kustomize.

## Releasing

This repo uses [Conventional Commits](https://www.conventionalcommits.org/) and [GoReleaser](https://goreleaser.com/).

- Pull request titles and commit messages are linted by `.github/workflows/conventional-commits.yml`.
- Pushing a tag matching `v*.*.*` runs `.github/workflows/release.yml`, which publishes:
  - manager binaries
  - a multi-arch GHCR image (`linux/amd64`, `linux/arm64`)
  - a GitHub Release whose `install.yaml` is a Kustomize build of `config/default` pinned to that tag

```sh
git tag -a v0.1.0 -m "chore(release): v0.1.0"
git push origin v0.1.0
```

Users can then install that release with either:

```sh
kubectl apply -f https://github.com/go-paintedsky/vikunja-operator/releases/download/v0.1.0/install.yaml
# or pin config/overlays/homelab images.newTag to v0.1.0 and:
kubectl apply -k config/overlays/homelab
```

The GHCR package must be public, or the cluster needs pull credentials (see above). GitHub Actions needs `contents: write` and `packages: write` on `GITHUB_TOKEN` (the release workflow sets these).

## Project Distribution

Tagged releases already attach a Kustomize-built `install.yaml` (see [Releasing](#releasing)). You can also build that bundle locally:

```sh
make build-installer IMG=<some-registry>/vikunja-operator:tag
# writes dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing

Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages and pull request titles (`feat:`, `fix:`, `docs:`, `chore:`, ...). CI rejects PRs that do not match.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).

