# TODO

AGENTS.md and HUMANS.md describe what exists.

## Re-count hand-authored .proto files before treating buf as default

Recounted 2026-10-16. Hand-authored `.proto` files have spread past one repo, so a later change makes `buf` the default for `.proto` the way `rustfmt` is the default for Rust. This entry is that count. The formatter change is the follow-on.

`CHANGELOG.md` records that Kotlin, XML, Lua, and Gradle were surveyed and passed over, and that `.proto` was a near-term watch once protobuf usage spread past its one then-current repo (`CHANGELOG.md:374-377`). `HUMANS.md` lists `buf` with the optional tools (`HUMANS.md:9-10`). The walk found 22 hand-authored `.proto` files in 12 git repositories, all under `/usr/local/src/com.github/Rethunk-Tech/Bastion`. A walk of `/usr/local/src` that skips the directory names `node_modules`, `.git`, `vendor`, and `dist` found no other `.proto` files. A follow-on default is now justified.

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

`.proto` already routes to `buf format -w` and skips when `buf` is absent (`CHANGELOG.md:189-191`, `internal/dispatch/dispatch.go:142`, `internal/formatters/proto.go:21-23`). The follow-on gives that path the same default standing `rustfmt` has for Rust (`internal/formatters/rust.go:5-7`). It does not add a second protobuf formatter.

### Read

- `CHANGELOG.md:374-377` — Kotlin/XML/Lua/Gradle were surveyed and passed over; `.proto` was the watch item once usage left its one then-current repo. That spread is the condition this count checks.
- `HUMANS.md:9-10` — `buf` is listed with the optional tools.
- `HUMANS.md:104-105` — proto is formatted via `buf` when the tool is on `PATH`.
- `CHANGELOG.md:189-191` — the existing route is `buf format -w`, silent skip when `buf` is absent, same system-binary pattern as `rustfmt`.
- `internal/formatters/rust.go:5-7` — `rustfmt` is the canonical formatter for Rust. The follow-on matches that standing.
- `internal/formatters/proto.go:5-11` — `buf` is already the single canonical `.proto` formatter. `internal/formatters/proto.go:21-23` skips when it is missing.
- `internal/dispatch/dispatch.go:142` — `.proto` is already registered. The follow-on edits standing, not the registry shape.

### Traps

- Generated `.proto` files and third-party trees do not count as hand-authored. Skip `node_modules`, `.git`, `vendor`, and `dist`, and skip files whose headers say they are generated (`protoc`, `DO NOT EDIT`, `Code generated`). None of the 22 files above carried those markers in the opening bytes.
- `bastion-core` copies of sibling service protos are still hand-authored sources in that git root. They are not generated stubs. Counting only `bastion-core`, or collapsing all of `Bastion/` into one repo, both mis-state the spread.
- A second formatter beside `internal/formatters/proto.go` would duplicate the route that already exists.

### Acceptance

- The count names the 12 repositories in the table above: `bastion-adcon-engine`, `bastion-adcon-mirror`, `bastion-control-plane`, `bastion-core`, `bastion-credential-keystore`, `bastion-hmac-urls`, `bastion-intent-ledger-replicator`, `bastion-ironlaw-loader`, `bastion-mfa`, `bastion-pack-loader`, `bastion-qemu`, and `bastion-ssh`.
- The section states that a follow-on default is now justified: `buf` becomes the default for `.proto` the way `rustfmt` is the default for Rust.
- Re-running the walk of `/usr/local/src`, skipping directory names `node_modules`, `.git`, `vendor`, and `dist`, still yields these 22 files and no others, until the tree itself changes.
