# Security

## Reporting a vulnerability

Please report security issues privately via GitHub's **"Report a vulnerability"**
flow (Security → Advisories) on this repository, or email security@brokenbots.net.
Do not open a public issue for an undisclosed vulnerability.

## Supply-chain controls

This repo is the Criteria adapter **wire contract** (protocol v2): the `.proto`
sources plus generated bindings published as a **Go module**, an **npm package**
(`@criteria/...`), and a **PyPI package**. It ships no executable binary.

Dependency hygiene is enforced in CI and documented in
[docs/dependency-policy.md](docs/dependency-policy.md):

- **`osv-scan`** — osv-scanner runs on every PR/push as a **blocking** gate over
  the Go module; no shipping known vulnerabilities. Exceptions are documented +
  dated in [`osv-scanner.toml`](osv-scanner.toml).
- **`deps-report`** — non-blocking Go dependency-freshness report.
- **Dependabot** — routine minor/patch updates for all four ecosystems (Go, npm,
  pip, GitHub Actions) with a 7-day supply-chain cooldown (security fixes exempt).

Reproduce the Go vulnerability gate locally with `make vuln-scan` and check
freshness with `make deps-outdated`.
