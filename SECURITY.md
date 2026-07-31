# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Seli CLI SDK, please **do NOT open a public GitHub issue**.

Email us at **security@seli.app** with:

- A description of the vulnerability
- Steps to reproduce
- Potential impact
- Your suggested fix (optional)

We will:

- Acknowledge receipt within **48 hours**
- Provide a detailed response within **7 days**
- Issue a fix and, where applicable, a CVE within **30 days** (timeline may vary by severity)

We appreciate responsible disclosure and will credit reporters in the release notes (unless you prefer to remain anonymous).

## Supported Versions

| Version | Supported |
|---------|-----------|
| 1.x     | ✅ Yes    |
| 0.x     | ❌ No     |

## Known Security Properties

**API key storage**

- Credentials are stored in `~/.seli/config.json` with `chmod 600` permissions
- The CLI never accepts the API key as a persistent CLI flag
- The `Authorization` header is redacted in `--verbose` output

**Transport security**

- HTTPS is enforced; `http://` base URLs are rejected at startup
- TLS certificate verification is not disabled

**Distribution integrity**

- Each GitHub Release includes a `checksums.txt` (SHA256) for all binaries

**Supply chain**

- All dependencies are tracked in `go.sum`
- Dependabot opens PRs for patch and minor dependency updates
- `govulncheck` runs in CI on every PR

## Out of Scope

The following are not considered vulnerabilities for this project:

- Issues that require physical access to the user's machine
- Social engineering attacks
- Vulnerabilities in the Seli platform itself (report those to security@seli.app separately)
