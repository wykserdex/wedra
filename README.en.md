# WEDRA

[Русский](README.md) | English

WEDRA is a local, deterministic pipeline runner with a human approval gate.
Agents can propose and inspect a plan, while a person remains the decision
point for gated actions.

## Current contract

- Product version: `0.32a` from [`VERSION`](VERSION)
- Pipeline/plugin protocol: `0.2` from [`protocol/VERSION`](protocol/VERSION)
- Primary CLI: `wedra`
- Compatibility CLI: `tool` (legacy surface; do not add new behavior there)

The product version and protocol version are independent. See the
[versioning policy](docs/versioning.md).

## Build and run

```bash
go build -o wedra ./cmd/wedra
./wedra version
./wedra plugin list
./wedra pipeline validate examples/email_check.yaml
./wedra pipeline run examples/gate_demo.yaml
./wedra runs list
```

The primary CLI supports pipeline validation and execution, plugin registry
operations, resume, the local GUI, and an MCP stdio adapter. Plugin manifests
declare their own component version and protocol compatibility separately.

## Repository layout

```text
cmd/wedra/            primary CLI
cmd/wedragui/         desktop launcher
cmd/tool/             compatibility CLI
internal/pipeline/    model, parsing, validation, planning
internal/execution/   runner and resume
internal/plugin/      plugin process and manifest contract
internal/registry/    registry and install pins
internal/journal/     append-only run journal
internal/gate/        human gate
internal/runctx/      shared context
plugins/official/     maintained plugins
plugins/community/    community plugins
examples/             canonical examples and presets
conformance/fixtures/ public conformance corpus
var/runs/             runtime output, ignored by git
```

`internal/core` is a transitional compatibility layer. The layout and its
compatibility surfaces are documented in [architecture](docs/architecture.md)
and are not to be reorganized without a public proposal and migration plan.

## Community and safety

WEDRA is currently maintained by one primary maintainer. The project is
transparent about that risk and uses public proposals, CI gates, immutable
release tags, and explicit registry admission. See [governance](GOVERNANCE.md)
and [contributing](CONTRIBUTING.md).

Plugin permissions are declarations, not an operating-system sandbox. Review
network, filesystem, and secret permissions before installing a plugin.

A plugin may declare `sandbox: untrusted` in its manifest. Such a plugin only
runs inside an OS sandbox (`bwrap` on Linux, `sandbox-exec` on macOS) and only
with explicit operator consent via `--allow-untrusted-plugins`;
`--deny-untrusted-plugins` refuses every plugin in the run and is the
recommended flag for CI. On Windows there is no isolation backend, so untrusted
code is refused outright. A backend that is installed but cannot isolate on the
host (for example, user namespaces are blocked) counts as absent. The sandbox
restricts writes but not reads, and does not filter egress for a plugin that
declares `permissions.network`, so it is not a complete boundary for hostile
code. Report vulnerabilities according to [SECURITY.md](SECURITY.md).
