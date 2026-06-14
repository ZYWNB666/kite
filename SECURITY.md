# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.5.x   | :white_check_mark: |
| 1.4.x   | :white_check_mark: |
| < 1.4.0 | :x:                |

## Reporting a Vulnerability

To report a vulnerability, please immediately let us know by emailing kite@zzde.me.

## Known Security Issues

See [SECURITY_AUDIT_REPORT.md](SECURITY_AUDIT_REPORT.md) for the full audit report and [SECURITY_TEST_REPORT.md](SECURITY_TEST_REPORT.md) for live deployment test results.

### Critical Issues (Unfixed)
- #1: `CreateSuperUser`/`ImportClusters` endpoints lack authentication
- #5: Feishu callback signature verification is optional
- #8: Login endpoints have no rate limiting

### Fixed Issues
- #3: OAuth redirect URL no longer trusts `X-Forwarded-*` headers
- #14: `/metrics` endpoint now requires authentication
- #16: Login error messages no longer leak usernames
