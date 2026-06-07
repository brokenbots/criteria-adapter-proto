# criteria-adapter-proto

The **adapter wire contract** for [Criteria](https://github.com/brokenbots/criteria) —
protocol **v2** `.proto` sources and generated bindings. This is the single
source of truth for the host↔adapter wire; the host and every SDK consume it as
a versioned dependency so no single project can drift the contract.

## Layout

```
proto/criteria/v2/   # .proto sources (adapter.proto, options.proto)
criteria/v2/         # generated Go bindings + helpers (package criteriav2)
npm/                 # @criteria/adapter-proto package (TS bindings, generated at publish)
python/              # criteria-adapter-proto package (Python bindings, generated at publish)
buf.gen.multi.yaml   # TS + Python codegen config (protobuf-es, protoc python/grpc)
```

The TypeScript and Python bindings are generated from the `.proto` sources at
publish time (`buf generate --template buf.gen.multi.yaml`); they are not
committed (only the package manifests + the hand-written `npm/src/index.ts` are).

## Go

```bash
go get github.com/brokenbots/criteria-adapter-proto@latest
```

```go
import criteriav2 "github.com/brokenbots/criteria-adapter-proto/criteria/v2"
```

## TypeScript

```bash
npm add @criteria/adapter-proto
```

Generated with [protobuf-es](https://github.com/bufbuild/protobuf-es); runtime
dependency `@bufbuild/protobuf`.

## Python

```bash
pip install criteria-adapter-proto
```

```python
from criteria.v2 import adapter_pb2, adapter_pb2_grpc
```

## Versioning

The package follows **SemVer**, and all language artifacts (Go module, npm,
PyPI) are released at the **same version** from a single `vX.Y.Z` tag:

- **major** — breaking wire changes (field removals, type changes, renumbering,
  removing/renaming an RPC).
- **minor** — backward-compatible additions (new RPCs, new optional fields, new
  messages, additive helpers).
- **patch** — bug fixes in generated code or helpers; no wire-surface change.

Consumers and their pinned versions are tracked in
[DEPENDENCIES.md](DEPENDENCIES.md).

## Publishing

Tagging `vX.Y.Z`:

- **Go** — no step needed; the tag is resolvable via the module proxy.
- **npm / PyPI** — [`publish-langs.yml`](.github/workflows/publish-langs.yml)
  generates, builds, and publishes the TS + Python packages. Publishing is
  **gated on credentials** (`NPM_TOKEN` + the `@criteria` scope; `PYPI_API_TOKEN`):
  until those repository secrets are set, generation + build are still verified
  on each tag but the publish step is skipped.

## Security & dependencies

Supply-chain controls and the dependency-freshness policy are documented in
[SECURITY.md](SECURITY.md) and [docs/dependency-policy.md](docs/dependency-policy.md).
CI runs a **blocking** osv-scanner gate over the Go module plus a non-blocking
freshness report; Dependabot covers all four ecosystems (Go, npm, pip, GitHub
Actions) with a 7-day cooldown. Reproduce locally:

```bash
make vuln-scan      # osv-scanner — known-vulnerability gate (WS49)
make deps-outdated  # go-mod-outdated — freshness report (WS50)
make deps-majors    # gomajor — available major (/vN) upgrades
```
