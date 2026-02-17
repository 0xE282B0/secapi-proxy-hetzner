# Resources

This folder contains upstream references used to design and implement a SecAPI provider backed by Hetzner Cloud.

## Git submodules

- `secapi-spec`: https://github.com/eu-sovereign-cloud/spec
- `secapi-go-sdk`: https://github.com/eu-sovereign-cloud/go-sdk
- `hetzner-cli`: https://github.com/hetznercloud/cli
- `hetzner-go-sdk`: https://github.com/hetznercloud/hcloud-go

## Downloaded docs snapshots

- `docs/hetzner-cloud-reference`: mirror of `https://docs.hetzner.cloud/reference/cloud`
- `docs/hetzner-reference`: mirror of `https://docs.hetzner.cloud/reference/hetzner`
- `docs/secapi-docs`: mirror of `https://spec.secapi.cloud/` and linked docs pages

## Refresh commands

Run from repository root:

```bash
# Update submodules to latest remote commits
git submodule update --init --remote

# Refresh Hetzner docs snapshots
wget --mirror --convert-links --adjust-extension --page-requisites --no-parent \
  --directory-prefix=resources/docs/hetzner-cloud-reference \
  https://docs.hetzner.cloud/reference/cloud

wget --mirror --convert-links --adjust-extension --page-requisites --no-parent \
  --directory-prefix=resources/docs/hetzner-reference \
  https://docs.hetzner.cloud/reference/hetzner

# Refresh SecAPI docs snapshots
wget --mirror --convert-links --adjust-extension --page-requisites --no-parent \
  --directory-prefix=resources/docs/secapi-docs \
  https://spec.secapi.cloud/ \
  https://spec.secapi.cloud/docs/content/intro \
  https://spec.secapi.cloud/docs/api/Foundation/Authorization-v1/authorization
```
