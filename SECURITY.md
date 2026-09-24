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
