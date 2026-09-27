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

## Untrusted plugin code (`sandbox: untrusted`)

A plugin manifest may declare `sandbox: untrusted`, meaning the author
considers the code to be third-party. Such a plugin is refused unless the
operator explicitly opts in with `--allow-untrusted-plugins`, and even then it
only runs inside an OS-level sandbox. A `untrusted` plugin may not declare
`permissions.secrets`.

| Platform | Backend | Notes |
| --- | --- | --- |
| Linux | `bwrap` (bubblewrap) | read-only host filesystem (including `/tmp` and the plugin directory), separate PID/IPC/UTS, a private writable scratch directory created per run and deleted afterwards, network namespace dropped unless `permissions.network` is declared |
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

**What this does not yet buy.** On Linux the plugin still shares the host network
namespace, so `HTTP_PROXY` is advisory, not enforced: a plugin that ignores the
variable can open a socket directly. Kernel enforcement needs the plugin in its
own namespace with no route out, reachable only by loopback — which in practice
means a userspace network stack (`pasta`/`slirp4netns`, the mechanism rootless
Podman uses) and plumbing `bwrap` does not currently expose. Until that lands,
the filter is honest for cooperative plugins and bypassable for hostile ones, and
this document should not claim otherwise. It also does not address exfiltration
through a file in the scratch directory, through stdout, or through a DNS tunnel
carried inside request names.

### Known gap: an unset `network` field is not "deny"

`network: deny` is the documented default, but the runner only applies its deny
check when the field is literally `deny`. A pipeline that omits `network`
entirely runs neither branch and therefore receives **no network check at all**,
which means a plugin that declares a host gets unrestricted egress. This is
pre-existing and contradicts the documentation.

Treating unset as deny was tried and reverted: it refuses existing pipelines
whose plugins declare a host (`crtsh`, the `net_demo` conformance fixtures), so
it needs its own migration rather than riding along with the granularity work.
`TestNetworkPolicyMatrix` in `internal/execution` pins the current behaviour and
names these rows as the known gap, so the day the fix lands the expectations and
this section move together.