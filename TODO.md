# TODO

AGENTS.md and HUMANS.md describe what exists.

## Hand-authored .proto files

Recounted 2026-10-16. Hand-authored `.proto` files have spread past one repo. `buf` is already the canonical formatter for `.proto` and skips when `buf` is absent, the same way `rustfmt` is the canonical formatter for Rust (`internal/formatters/proto.go:5-11`, `internal/formatters/proto.go:21-23`, `internal/formatters/rust.go:5-7`, `internal/formatters/rust.go:18-20`). `HUMANS.md` lists both with the optional tools and routes proto via `buf` the same way it routes Rust via `rustfmt` (`HUMANS.md:9-10`, `HUMANS.md:104-105`). `.proto` is registered on that path (`internal/dispatch/dispatch.go:142`, `CHANGELOG.md:189-191`).

The walk found 22 hand-authored `.proto` files in 12 git repositories, all under `/usr/local/src/com.github/Rethunk-Tech/Bastion`. A walk of `/usr/local/src` that skips the directory names `node_modules`, `.git`, `vendor`, and `dist` found no other `.proto` files.

| Repository | Hand-authored `.proto` files |
| --- | --- |
| `bastion-adcon-engine` | `proto/adcon_engine/v1/adcon_engine.proto` |
| `bastion-adcon-mirror` | `proto/adcon_mirror/v1/adcon_mirror.proto` |
| `bastion-control-plane` | `proto/control_plane.proto`, `proto/egress_policy/v1/egress_policy.proto`, `proto/pki_trust/v1/pki_trust.proto` |
| `bastion-core` | 8 files under `proto/` and `apps/server/src/proto/` (see Traps) |
| `bastion-credential-keystore` | `pkg/proto/service.proto` |
| `bastion-hmac-urls` | `proto/hmac_urls/v1/hmac_urls.proto` |
| `bastion-intent-ledger-replicator` | `proto/intent_ledger_replicator/v1/ilr_replicator.proto` |
| `bastion-ironlaw-loader` | `proto/ironlaw_loader/v1/ironlaw_loader.proto` |
| `bastion-mfa` | `proto/mfa/v1/mfa.proto` |
| `bastion-pack-loader` | `proto/pack_loader.proto` |
| `bastion-qemu` | `api/qemu.proto` |
| `bastion-ssh` | `proto/bastion_ssh.proto`, `proto/keystore_service.proto` |

Leaving `bastion-core` out still leaves 14 files in the other 11 repositories. Three `bastion-core` copies are byte-identical to the sibling service file (`adcon_engine.proto`, `adcon_mirror.proto`, `ilr_replicator.proto`). Five have diverged (`bastion_ssh.proto`, `ironlaw_loader.proto`, `mfa.proto`, `qemu.proto`, `service.proto`).

### Traps

- Generated `.proto` files and third-party trees do not count as hand-authored. Skip `node_modules`, `.git`, `vendor`, and `dist`, and skip files whose headers say they are generated (`protoc`, `DO NOT EDIT`, `Code generated`). None of the 22 files above carried those markers in the opening bytes.
- `bastion-core` copies of sibling service protos are still hand-authored sources in that git root. They are not generated stubs. Counting only `bastion-core`, or collapsing all of `Bastion/` into one repo, both mis-state the spread.
- A second formatter beside `internal/formatters/proto.go` would duplicate the route that already exists.

### Acceptance

- The count names the 12 repositories in the table above: `bastion-adcon-engine`, `bastion-adcon-mirror`, `bastion-control-plane`, `bastion-core`, `bastion-credential-keystore`, `bastion-hmac-urls`, `bastion-intent-ledger-replicator`, `bastion-ironlaw-loader`, `bastion-mfa`, `bastion-pack-loader`, `bastion-qemu`, and `bastion-ssh`.
- Re-running the walk of `/usr/local/src`, skipping directory names `node_modules`, `.git`, `vendor`, and `dist`, still yields these 22 files and no others, until the tree itself changes.
