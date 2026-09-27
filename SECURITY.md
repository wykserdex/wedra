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
| macOS | `sandbox-exec` | writes limited to the plugin directory and scratch; the plugin directory is writable because `sandbox-exec` cannot express a read-only bind mount |
| Windows | none | fail-closed: untrusted plugins cannot run (blocked on low-integrity sandbox root, see below) |

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
- The restriction is effective: the acting user's own unelevated token is
  locked out of the directory immediately afterwards (`ReadDir` â†’
  `Access is denied`). The `C:\ProgramData` `ACCESS_DENIED` previously recorded
  for a "DACL-only write" does not reproduce for a directory the process created
  itself; it applies to rewriting the DACL of an existing subtree.
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
not available to a non-elevated owner â€” `icacls /setintegritylevel` returns
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

### Egress is not filtered by destination

A plugin's `permissions.network` already carries structure — `host`, `port`, and
an explicit `any_host` escape hatch. That structure was never enforced. The
sandbox collapsed the whole list into "network was declared, so grant the
network", so a plugin stating `api.telegram.org:443` received unrestricted
egress, and the manifest read like a constraint that was in force.

A per-destination filter is not implementable inside either backend: `bwrap`
cannot express one (it either drops the network namespace or shares the host's),
and `sandbox-exec` only understands `allow`/`deny network*`. Rather than keep an
unenforced restriction in the manifest, the enforceable case is now explicit:

- `any_host: true` — blanket egress, granted, and now something the author has to
  write on purpose.
- a list of concrete hosts without `any_host` — **not** silently widened. The
  sandbox denies the network, and under `network: allow` the run stops with
  `network_not_enforceable` and an explanation.

Official LLM plugins that previously declared a precise host were moved to
`any_host: true` with the intended target kept in `note`. That is not a
tightening — they already had blanket egress — it removes a false claim from the
manifests. The declared host lists in those notes are still aspirational.

Consequence to be explicit about: **egress filtering does not exist yet.** Any
plugin that legitimately needs the network can still read everything its sandbox
lets it read and send it anywhere. On Linux the default is still a separate
network namespace, so the no-network case is genuinely closed; the gap is the
`allow` case. Closing it needs an egress proxy outside the sandbox that enforces
the declared destinations, reachable from the sandbox only by a socket or named
pipe, plus DNS pinned to the same allowlist. The model is ready; the enforcement
is not written.