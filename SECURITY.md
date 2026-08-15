# Security Policy

## Supported Versions

Security fixes are provided for the current public beta line:

| Version | Supported |
| --- | --- |
| `v0.1.x` | Yes |
| Older or untagged builds | Unsupported |

## Reporting a Vulnerability

Please report security vulnerabilities privately through [GitHub Security Advisories](https://github.com/wenn-id/devparity/security/advisories/new). Do not open a public issue, pull request, or discussion containing an unpatched vulnerability, exploit instructions, credentials, or personal data.

Include only the smallest reproducible details needed to validate the report: affected version or commit, operating system, reproduction steps, impact, and a proposed mitigation if known. Please redact tokens, keys, repository contents, and other secrets before submitting.

The maintainers will acknowledge a report as soon as practical, investigate it privately, and coordinate disclosure after a fix or mitigation is available. Reports that cannot be reproduced may require additional information through the private advisory.

## Security Boundaries

DevParity is local-first. Static commands do not execute repository code, access the network, or write to the target repository. Host documentation execution is unsandboxed and requires an explicit trust flag on every invocation. Container execution is the preferred execution mode for reviewed Docker/Podman environments, defaults to no network, and runs in a temporary copied workspace.
