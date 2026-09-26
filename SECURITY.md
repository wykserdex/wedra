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
| Windows | none | fail-closed: untrusted plugins cannot run |

An AppContainer backend for Windows was investigated and is not enabled. The
blocker is not the AppContainer API but host ACL state. An AppContainer token
holds neither the user SID nor `Users`/`Everyone`, so the container can only
traverse a directory chain that grants one of those, and any plugin that needs
an explicit grant requires a DACL write on that chain. Measured on a Windows
workstation, both candidate placements fail without elevation:

- inside the user profile (the plugin directory, and `%LOCALAPPDATA%` for a
  Python interpreter): the `SetNamedSecurityInfo` call on `%LOCALAPPDATA%`
  blocks indefinitely, reproducibly, from Go, PowerShell and .NET alike. No
  visible dialog is involved — the screen stays idle — which is consistent with
  a filesystem minifilter serialising security-descriptor writes. Defender's
  Controlled Folder Access is off, so it is not that specific feature.
- outside the profile (`C:\ProgramData`, the only user-writable root):
  a DACL-only write returns `ACCESS_DENIED` immediately for a non-elevated
  process. `Users` may create subdirectories there but not modify its DACL.

The remaining options are both rejected on purpose: requiring WEDRA to run
elevated so a plugin runner can rewrite ACLs in the user's profile, or shipping
a Windows backend that can wedge the host or leave a stale ACE behind after a
crash. Windows therefore stays fail-closed, and an untrusted plugin cannot run
there. This also rules out a "copy the plugin into a sandbox-owned directory"
workaround, because a vault inside the profile is unreachable for the same
reason and a vault outside it cannot be made reachable without the DACL write
above.

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
