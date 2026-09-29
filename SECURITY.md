# Security policy

## Reporting a vulnerability

Do not open a public issue containing exploit details, credentials, or a
minimal proof of concept. Prefer GitHub's private vulnerability reporting for
this repository. If private reporting is unavailable, contact the repository
owner through the GitHub profile without publishing technical details in a
public thread.

Include the affected WEDRA version, platform, reproduction steps, and the
expected impact. Maintainers will acknowledge a complete report when they can
and will coordinate disclosure timing with the reporter.

## Supported line

The current development line is the only line receiving security fixes until
a support policy is published. Historical releases and compatibility aliases
are not automatically patched. The current product version is in `VERSION`;
the protocol version is in `protocol/VERSION`.

## Plugin and secret handling

Plugins are subprocesses and may run with the permissions of the current user.
Review `permissions.network`, `permissions.filesystem`, and
`permissions.secrets` before installing a plugin. Never put real API keys in
YAML, fixtures, logs, examples, or pull requests.

MCP path checks are a reference and path policy, not an operating-system
sandbox. Do not treat an MCP client or a plugin manifest as a security boundary
without separate OS-level isolation.

## The GUI HTTP server (`wedra gui`, `wedragui`, the `wedra mcp` gate console)

The GUI is an HTTP server that runs step plugins, so its port is a real
attack surface on the machine. What it enforces:

- **Session on everything.** Every path under `/api/*` requires the human's
  session cookie — reads included: run journals, step inputs and outputs, plugin
  lists, pipelines, gate state. Two paths are open and are listed in one place in
  the code (`publicAPI`): `GET /api/health` (version and liveness, no run data)
  and `/api/session` (the code exchange itself). `--no-session` turns the
  requirement off and is refused on a non-loopback address.
- **Host allow-list.** Requests are served only for `127.0.0.1`, `localhost`,
  `[::1]`, the address the operator passed, and any `--public-host`. A foreign
  `Host` is refused with `E_HOST_NOT_ALLOWED`. This is what stops DNS
  rebinding: a page on someone else's domain resolving to `127.0.0.1` sends its
  own `Host` header, and that header is now checked.
- **No implicit proxies.** `X-Forwarded-Host` is never read.
  `X-Forwarded-Proto` is read only under an explicit `--trusted-proxy`, and only
  to set the `Secure` cookie flag — the value cannot come from the client that
  is being judged.
- **Origin against the server's own identity.** The `Origin`/`Referer` check on
  mutations compares against the configured allow-list and external scheme, not
  against a value taken from the same request (which would be a tautology).
- **Entry by one-time code.** The server prints a short, single-use pairing code.
  It exchanges for a cookie whose value is a separate token with a 12-hour TTL
  held in server memory; the code itself dies on first use. The cookie name
  carries the port, so two instances on `localhost` cannot overwrite each
  other's session.
- **Listening beyond loopback is explicit.** `--listen` on a non-loopback
  address is refused unless `--allow-remote` is given, `0.0.0.0`/`::` additionally
  requires at least one `--public-host`, and `--no-session` is refused there
  outright. Pipelines are written with mode `0600` on POSIX.

What this does **not** do: it does not stop a process running as the same OS
user that can read the terminal, the browser profile, or the server's memory.
The session protects against an agent (or any process) that can only send HTTP
to the local port. TLS is not provided: put a reverse proxy in front and declare
it with `--trusted-proxy --public-host=...`, or keep the server on loopback.

## Untrusted plugin code (`sandbox: untrusted`)

A plugin manifest may declare `sandbox: untrusted`, meaning the author
considers the code to be third-party. Such a plugin is refused unless the
operator explicitly opts in with `--allow-untrusted-plugins`, and even then it
only runs inside an OS-level sandbox. A `untrusted` plugin may not declare
`permissions.secrets`.

| Platform | Backend | Notes |
| --- | --- | --- |
| Linux | `bwrap` (bubblewrap) | read-only host filesystem (including `/tmp` and the plugin directory), separate PID/IPC/UTS, a private writable scratch directory created per run and deleted afterwards, **network namespace always separate**. A plugin that declares `any_host` gets egress through a userspace stack (`slirp4netns`); without it there is no network at all, and without `slirp4netns` installed the run is refused |
| macOS | none | fail-closed: backend archived, see below |
| Windows | none | fail-closed: untrusted plugins cannot run (blocked on low-integrity sandbox root, see below) |

### macOS: backend archived

The `sandbox-exec` backend was removed from the build and kept at
`attic/sandbox_darwin.go.archived`, with its behavioural test at
`attic/sandbox_darwin_test.go.archived`. macOS now resolves through the same
fail-closed path as Windows: an untrusted plugin is refused rather than run
without isolation.

The reason is integrity, not egress. `sandbox-exec` cannot express a read-only
bind mount, so the backend had to open the plugin directory for write, and a
plugin could rewrite itself and persist on disk. The Linux backend states the
opposite as a property it provides; on macOS that property did not hold. Note
also that the refusal message no longer points macOS users at `sandbox-exec`,
since recommending an isolator that is not wired up would be worse than saying
nothing.

Restoring it is a revert, but it should not happen before there is an answer for
the writable plugin directory. The archived test is the only behavioural
verification of a real isolator on real hardware in this repository, so it is
kept rather than deleted.

An AppContainer backend for Windows was investigated and is not enabled. This
section previously blamed host ACL state. That diagnosis was wrong, and the
measurements behind it probed the wrong operation. Corrected findings, all
measured on a clean Windows 10 Enterprise 22H2 x64 VM from a non-elevated
process (`elevated: false`), using `SetNamedSecurityInfo` with
`PROTECTED_DACL_SECURITY_INFORMATION`:

- A non-elevated process **can** create a directory and restrict its DACL to
  `SYSTEM` + `Administrators` + the container SID. Verified in three
  placements: `%LOCALAPPDATA%\Temp`, a nested subdirectory of it, and
  `C:\ProgramData`. All three returned `OK`.
- The restriction is effective: the acting user's own **unelevated** token is
  locked out of the directory immediately afterwards (`ReadDir` →
  `Access is denied`). The user's SID is gone from the DACL entirely, which is
  what is asserted by the test. Note the scope: an *elevated* token, or one whose
  `Administrators` SID is enabled rather than deny-only, still gets in through
  the `Administrators` ACE — and it could reclaim the directory regardless, so
  that is the correct outcome and not a gap. The `C:\ProgramData`
  `ACCESS_DENIED` previously recorded for a "DACL-only write" does not reproduce
  for a directory the process created itself; it applies to rewriting the DACL
  of an existing subtree.
- `PROTECTED_DACL_SECURITY_INFORMATION` is load-bearing. Without it the new ACL
  merges with inherited ACEs (6 ACEs instead of 3) and the user token still
  passes. With it, inheritance is severed and the user token is excluded.
- `SetNamedSecurityInfo` on `%LOCALAPPDATA%` itself succeeds in seconds. The
  previously recorded indefinite block did not reproduce; it looks like a slow
  operation rather than a hang, and the screen-idle symptom remains unexplained.
- **Rewriting the DACL of `%LOCALAPPDATA%` is not merely unnecessary, it is
  destructive.** Doing it locks the user out of their entire profile and breaks
  unrelated tooling (during this work it broke `go build`, whose scratch
  directory lives in `%LOCALAPPDATA%\Temp`). Any Windows backend must therefore
  restrict only directories it created itself.

The real remaining blocker is not the DACL but the **integrity level**. A real
AppContainer token was created via `CreateAppContainerProfile` and a child
process was launched under it with `PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES`.
The container token is still denied: AppContainer tokens run at low integrity,
the candidate directories are at medium, and the resulting write-up is refused.
This is a mandatory-label (SACL) decision, not a DACL one. Lowering the label is
not available to a non-elevated owner — `icacls /setintegritylevel` returns
`Access is denied` because it needs `WRITE_OWNER`.

Windows therefore stays fail-closed, but the requirement is now narrow and
specific rather than "host ACL state": a Windows backend needs one directory it
owns created at **low integrity**, which in practice means either an install-time
or elevated step, or hosting the sandbox root somewhere the OS already creates
at low integrity. Until that exists, the ACL work above is necessary but not
sufficient, and the platform stays closed.

If the backend is missing, or the host forbids creating one, the run stops
with `platform:sandbox_unavailable` before any process is created. The
availability probe runs once per process: `bwrap` is verified by actually
building a namespace, `sandbox-exec` by actually writing a probe file under the
real profile. A backend that is installed but cannot isolate anything is treated
as absent. There is no path that executes untrusted code outside a sandbox.

What the backends do **not** provide: read isolation (a plugin can read any
file the user can read, subject to the host mount set), egress filtering when a
plugin declares network permissions, and resource limits. Do not treat
`--allow-untrusted-plugins` as a complete boundary for hostile code.

`--deny-untrusted-plugins` refuses every plugin in the run, including plugins
that do not declare `sandbox`. It is the recommended flag for CI. Without
either flag, a plugin that declares nothing keeps running with the current
user's permissions, which is the historical behaviour and is not a security
boundary.

The trust decision belongs to the core, not to the plugin: the manifest field
is enforced by the kernel, and the policy is set once per run and inherited by
every step.

### Egress filtering: proxy built, kernel enforcement still missing

`permissions.network` already carries structure — `host`, `port`, and an
explicit `any_host` escape hatch. That structure was never enforced. The
sandbox collapsed the whole list into "network was declared, so grant the
network", so a plugin stating `api.telegram.org:443` received unrestricted
egress, and the manifest read like a constraint that was in force.

A per-destination filter is not implementable inside either backend: `bwrap`
cannot express one (it either drops the network namespace or shares the host's),
and `sandbox-exec` only understands `allow`/`deny network*`. So the enforceable
case is now explicit:

- `any_host: true` — blanket egress, granted, and now something the author has to
  write on purpose.
- a list of concrete hosts without `any_host` — **not** silently widened. The
  sandbox denies the network, and under `network: allow` the run stops with
  `network_not_enforceable` and an explanation.

Official LLM plugins that previously declared a precise host were moved to
`any_host: true` with the intended target kept in `note`. That is not a
tightening — they already had blanket egress — it removes a false claim from the
manifests. The declared host lists in those notes are still aspirational.

On top of that, `internal/plugin/egress.go` implements the standard design used
by other agent sandboxes: the proxy runs in the **unsandboxed** WEDRA process on
loopback, the plugin receives `HTTP_PROXY`/`HTTPS_PROXY`, and the proxy allows
only declared destinations. It resolves every A/AAAA record, refuses private,
loopback, link-local and CGNAT ranges before dialling, then dials the **IP
literal** so DNS cannot rebind between the check and the connection. `CONNECT`
is restricted to TLS ports, and `stop` closes hijacked tunnels explicitly because
`http.Server.Shutdown` does not. If the proxy cannot start, the plugin does not
run (`sandbox_unavailable`).

**The proxy is currently dormant, and saying otherwise would be the easy mistake.**
A plugin inside a separate network namespace cannot reach the host: measured on
WSL2 with `bwrap` and `slirp4netns`, a service listening on the host's
`127.0.0.1` is refused from inside, and the stack's gateway address is
unreachable too. So the proxy's loopback address is simply not visible to a
sandboxed plugin, and `HTTP_PROXY` is not handed to it — pointing a cooperative
plugin at a dead address would only hand it a refusal on every request. The
allowlist code stays in the tree with its tests, but no current backend gives a
plugin a reachable host loopback, so nothing calls it. It is plumbing for a
future shared-netns backend, not a control in force today.

### The plugin's network namespace is now always its own

`--unshare-net` used to be added only when the plugin declared no network, which
meant a plugin declaring `any_host: true` ran with the **host's** network
namespace. That was a wider hole than unfiltered egress: the plugin could reach
every service bound to the host's loopback, including WEDRA's own HTTP API.

The namespace is now always separate, and egress is provided by a userspace
stack (`slirp4netns`): `tap0` inside the plugin's namespace, traffic NAT-ed by the
host. Measured inside such a sandbox:

| From inside the plugin | Result |
| --- | --- |
| host service on `127.0.0.1` | refused (`ConnectionRefusedError`) |
| the stack's gateway `10.0.2.2` | unreachable |
| internet by IP (`1.1.1.1:443`) | reachable |
| DNS via the stack's forwarder | resolves |

Two details the implementation depends on, both found the hard way:

- The plugin must not start before `tap0` exists, or its first requests fail.
  WEDRA runs the command behind a gate: `/bin/sh -c 'while [ ! -e "$1" ]; ...;
  exec "$@"'`, and the gate file is created only after the interface is up. The
  internal `$$` cannot be used to find the namespace either — under `--unshare-pid`
  it is a PID-namespace-relative pid, useless to `slirp4netns`. The host-visible
  pid comes from `/proc/<bwrap>/task/*/children`, and **its network namespace is
  checked before attaching**: aiming `slirp4netns` at the `bwrap` process itself
  adds `tap0` to the *host* namespace, which breaks the host.
- `/etc/resolv.conf` is a symlink on most hosts, and `bwrap` cannot create a file
  over a symlink (`Can't create file`). The override is bound at the *resolved*
  path. It is needed at all because the host's resolver address is not reachable
  from inside the isolated namespace, so without the override every DNS lookup
  times out.

**What this still does not buy.** Per-destination filtering. Inside
`unshare -Ur -n` — the same primitives `bwrap` uses — the sandbox is `UID=0` in a
user namespace that owns the network namespace, and on both a Debian 13.7 host
(kernel `6.12.107+deb13-amd64`) and WSL2 (kernel `6.18.33.2-microsoft-standard-WSL2`)
it can `ip link set lo up`, `nft add table`, `nft add chain` and `nft delete
table`. Rules installed inside the plugin's own namespace are not a control: the
plugin deletes them, or runs `nft flush ruleset`, before dialling. A userspace
stack does not change that — it hands the whole namespace outbound connectivity,
and the plugin dials through it directly rather than through a proxy.

So a plugin that declares `any_host: true` gets **unfiltered** egress, but no
longer inside the host's network namespace. That is the trade: the host's own
services are out of reach, the destination is not filtered.

Enforcing per-destination egress on Linux needs privilege on the *host*: a veth
pair with nftables applied in the host namespace (`CAP_NET_ADMIN` or a privileged
helper), or cgroup-scoped egress as systemd applies it. A user namespace cannot
reach the host's nftables, so host-side rules would hold — but they cannot be
installed unprivileged, and a setuid helper in a CLI people install without
reading is a poor trade.

This still does not address exfiltration through a file in the scratch
directory, through stdout, or through a DNS tunnel carried inside request names.

### Fixed: an unset `network` field is now "deny"

`network: deny` has always been the documented default, but the gate only ran
when the field was literally `deny`. A pipeline that omitted `network` matched
neither branch, so it got **no network check at all**, and the subprocess was
handed `WEDRA_NETWORK=allow` — the empty field granted network while the docs
promised it denied it.

An earlier attempt at this was reverted because it refused existing pipelines
whose plugins declare a host. That migration has now been done, so the fix is
in place. `pipeline.EffectiveNetwork` is the single place that resolves the
field, and the runner, both validators and the editor all go through it — the
bug existed because four layers each re-implemented `p.Network == "deny"` and
drifted apart.

Making the empty field mean `deny` surfaced two more consequences of the same
drift, both fixed here:

- The validators reported "valid" for a pipeline the runner then refused
  (`network: allow` plus a `host:port` list, which is not enforceable). There is
  now `E_NETWORK_NOT_ENFORCEABLE`, so `validate` and `run` agree.
- The GUI editor treated `allow` as the default and dropped it when serializing,
  because the empty field used to mean `allow`. Under the new meaning that
  silently turned a permission into a prohibition: a pipeline round-tripped
  through the editor stopped being able to reach the network. An explicit
  `network: allow` now survives the round-trip, while a genuinely unset field
  stays unset.

MCP keeps its own, stricter check (`checkPipelineSafety` refuses any network
declaration outright, whatever the pipeline asks for) — that is deliberate and
separate from the pipeline-level gate.

`TestNetworkPolicyMatrix` in `internal/execution` pins the resulting matrix,
including the two unset rows that used to be the known gap, and
`TestRunNetworkUnsetEnvIsDeny` in `internal/core` pins the value the subprocess
actually receives.