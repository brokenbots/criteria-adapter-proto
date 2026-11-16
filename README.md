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

## Capability vocabulary

Adapters declare free-form capability strings in `InfoResponse.capabilities`
at the Info handshake; the host gates behavior on them, and unknown values
are ignored for forward-compatibility. Well-known values:

- `parallel_safe` — the adapter's `Execute` is safe to run concurrently with
  other steps.
- `adapter_tools` — the adapter speaks the adapter tool-call flow: it emits
  `permission.request` AdapterEvents with payload kind `adapter_tool` and
  consumes `PermissionEvent.tool_call_result` answers (see the
  `PermissionEvent` comment in [`adapter.proto`](proto/criteria/v2/adapter.proto)).

`capabilities` is distinct from `supported_features`, which lists the optional
lifecycle features an adapter implements — `pause`, `resume`, `snapshot`,
`restore`, `inspect`.

## Tool-call versioning matrix

Adapter tool calls ride the Permissions stream: the caller emits a
`permission.request` AdapterEvent (payload kind `adapter_tool`) and the host
answers with `PermissionEvent.tool_call_result` (success or typed failure) or
`cancel` (deny). Behavior across host/adapter version combinations:

|                 | **New host**                 | **Old host**                   |
| --------------- | ---------------------------- | ------------------------------ |
| **New adapter** | full flow                    | `call_error: host_unsupported` |
| **Old adapter** | unknown oneof member ignored | unchanged behavior             |

- **New adapter + new host** — the full flow: `permission.request` payload
  kind `adapter_tool` → `ToolCallResult`.
- **New adapter + old host** — the old host's permission interceptor treats
  the tool_call request as a plain permission request (policy allow/deny).
  If **allowed**, the old host never sends a result, so the new adapter sees
  a bare allow-grant (`PermissionEvent.request`) with no following
  `tool_call_result` within its call deadline — the SDK maps this
  deterministic signature to the typed `call_error: host_unsupported` and
  **caches it per session**, so subsequent calls fail fast without sending.
  If **denied**, `cancel` arrives and the call is denied as usual. The SDK
  helper must enforce a bounded deadline so an old host can never hang a
  call (CRI-164).
- **Old adapter + new host** — old adapters never send tool_call requests; an
  unexpected `tool_call_result` (unknown oneof member) is ignored by their
  generated code — harmless.
- **Old adapter + old host** — unchanged behavior.

Host gating on the `adapter_tools` declaration is **per-call, not at session
open**: a `tool_call` request from an adapter whose handshake
`InfoResponse.capabilities` lacked `adapter_tools` is answered with the typed
`call_error: capability_missing` — a missing declaration never aborts the
session (deterministic enforcement is specified in CRI-159).

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
