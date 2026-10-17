# Changelog

All notable changes to the criteria adapter wire contract are documented here.
The format is a loose Keep-a-Changelog: versioned sections, most recent first.
Version bumps follow semantic versioning against the wire contract only:
additive changes (new messages, new fields, new well-known event kinds) are
MINOR; field removals, renumbering, or semantic shifts are MAJOR and must pass
`make proto-lint`'s `buf breaking` gate against the previous release tag.

## [v0.8.0] — declared step timeout carrier (KB-282)

Additive minor over v0.7.0: one new request field. No field is removed or
renumbered; `buf breaking` against `v0.7.0` must stay clean.

### Added — proto

- `ExecuteRequest.timeout_ms` (NEW field 8, `int64`, unit: milliseconds) —
  the workflow's declared step timeout, forwarded from the engine's step
  declaration so adapters can enforce the host's deadline on their own
  execution (KB-212 leg 2 populates it; KB-283 leg 3 honors it on the
  shell adapter). It is a wire carrier only: enforcement (kill, retry,
  typed-failure reporting) stays the adapter's policy per its own
  execution capabilities. `timeout_ms = 0`/absent = unset — the adapter
  applies its own default, today's behavior byte-for-byte (the shell
  adapter otherwise kills long steps at its own 5m default; KB-95/KB-96
  evidence on KB-212).
- Conformance vectors and the pinned reference validator are unchanged:
  the carrier is request-only metadata with no response-side contract to
  pin, so the shared vectors stay byte-identical.

### Compatibility matrix

| Host \ Adapter | v0.7.x (pre-carrier) | v0.8.0 (carrier-aware) |
|---|---|---|
| **v0.7.x host** | unchanged | unchanged: the old host never populates `timeout_ms`, so the adapter keeps applying its own default |
| **v0.8.0 host** | works: the old adapter receives `timeout_ms` as an unknown field its generated code ignores and keeps applying its own default | declared timeouts reach the adapter |

### Publishing (v0.8.0 release mechanics)

- Tag `v0.8.0` triggers the existing publish flow: the Go module proxies
  this module, and the tag-derived publish job generates the TypeScript
  and Python bindings (`buf.gen.multi.yaml`).
- npm publish is skipped by design (no `NPM_TOKEN` secret exists; see the
  v0.7.0 notes). The engine pins v0.8.0 when KB-212 (leg 2) lands; the
  shell pins it when KB-283 (leg 3) lands.
- Security-lane dependency bump rides the same release (no wire-contract
  impact): `golang.org/x/net` 0.58.0 → 0.61.0 and the `go` directive
  1.26.6 → 1.26.9, clearing the GO-2026-66xx advisories the `osv-scan`
  gate flagged. Security updates bypass the 7-day cooldown by policy.

## [v0.7.0] — outcome-contract wave (KB-44)

Additive minor over v0.6.0: one new request-scoped contract system, one new
stream metadata field, one new repair backstop, two new well-known event
kinds. No field is removed or renumbered; `buf breaking` against `v0.6.0`
must stay clean.

### Added — proto

- `OutcomeContract` (NEW message) — the per-outcome payload contract:

  | Field   | Number | Type   | Meaning |
  |---------|--------|--------|---------|
  | `name`             | 1 | string | outcome name; must match an `allowed_outcomes` entry while hosts still populate it |
  | `schema_json`      | 2 | bytes  | JSON Schema for the outcome's payload; empty = no payload contract |
  | `fallback`         | 3 | bool   | no-finalize fallback outcome (host enforces at most one per step at compile time) |
  | `require_comment`  | 4 | bool   | comment is mandatory when finalizing with this outcome |
  | *reserved*         | 100–999 | | |

- `ExecuteRequest.outcome_contracts` (NEW field 6, repeated `OutcomeContract`)
  and `ExecuteRequest.rejection` (NEW field 7, `ExecutionRejection`; absent =
  normal execute). Contracts ride ExecuteRequest ONLY (they are per-step and
  Execute already carries per-step `allowed_outcomes`); Restore/Pause/Resume
  need no changes — a resumed session receives contracts on its next Execute.

- `ExecuteResult.comment` (NEW field 5, string) — the model/adapter finalize
  comment as first-class wire metadata (previously it survived only in adapter
  event payloads). Semantics are unchanged for old fields: `outcome` stays 1,
  `outputs_json` stays 4, legacy field 2 stays reserved.

- `ExecutionRejection` (NEW message) — host-rejection repair backstop:
  `outcome` (1), `issues` (2), `attempt` (3, 1-based repair counter,
  increments the previous rejection's attempt). Carried on
  `ExecuteRequest.rejection = 7`; non-empty = repair mode (skip the full step
  prompt, send a minimal repair prompt into the same live session, resubmit a
  corrected payload). Empty = normal execute.

- Well-known event kinds (registry: `docs/adapters.md`):
  `outcome.payload_invalid` `{"issues":[…]}` and `outcome.recovered`
  `{"attempt":…,"outcome":"…"}` are NEW; `outcome.finalized` (payload
  `{"outcome":…,"reason":…}`) and `outcome.reprompt` (`{"attempt":…,
  "maxAttempts":…}`) are standardized as written — existing adapters already
  emit them with those payload shapes, and their camelCase payload keys are
  preserved.

### Added — conformance

- `conformance/` ships the shared vectors (7 scenarios) and the pinned
  reference validator (`criteriav2.EvaluateOutcomeContracts`,
  `NewExecutionRejection`, `ValidatePayloadSchema`); see
  `conformance/README.md` for the vector format, the pinned algorithm, and the
  verbatim issue-string vocabulary. The engine suite and the three SDK suites
  (Go / TypeScript / Python) consume the same byte-identical files.

### Compatibility matrix

| Host \ Adapter | v0.6.x (pre-contract) | v0.7.0 (contract-aware) |
|---|---|---|
| **v0.6.x host** | unchanged | degrades to name-only validation¹ |
| **v0.7.0 host** | works (old adapter keeps name-only behavior; host still name-checks and validates payloads it receives) | full contract mode |

¹ A v0.7.0 adapter cannot emit contracts to an old host: it receives
`outcome_contracts` as unknown fields its generated code ignores, and the old
host never sends `rejection`. Contract mode simply does not activate
cross-version in that direction; `allowed_outcomes` name validation is the
only enforcement, exactly as today.

### Deprecation — ExecuteRequest.allowed_outcomes

`allowed_outcomes` (field 5) STAYS POPULATED for one deprecation cycle: it is
what makes the old-adapter/new-host row of the matrix work. In contract mode
the outcome must still be an allowed name. Hosts MAY stop populating it for
contract-mode steps after the cycle; the cycle counts from the v0.7.0
release. Empty `allowed_outcomes` in contract mode skips name-only validation
(contract membership is then the sole gate) — pinned by the conformance
vectors.

### Publishing (v0.7.0 release mechanics)

- Tag `v0.7.0` triggers the existing publish flow: the Go module proxies
  this module, and the tag-derived publish job generates the TypeScript and
  Python bindings (`buf.gen.multi.yaml`).
- npm publish is SKIPPED BY DESIGN (no `NPM_TOKEN` secret exists; do not add
  one): TypeScript consumers vendor the generated bindings or read the repo
  at the pinned tag — the TS SDK repos note this consumption path in their
  PRs.
- Four-layer bump note: the SDK/lockfile bump rides THIS repo's release; the
  engine pins v0.7.0 in a follow-up commit of the engine ticket (outcome-
  contract wave 2/5).

## [v0.6.0] — 2026-10-01

- CRI-201 (#22): `InfoResponse.state` (NEW field 18, `StateDescriptor`) —
  checkpoint declaration (mode, schema, max_bytes, granularity) for adapters
  with stateful sessions.

## [v0.5.3] / [v0.5.2] — 2026-09-15

- CRI-171 (#21): `InfoResponse.tools` (NEW field 17, repeated `ToolInfo` with
  name / description / args_schema_json) — advertised adapter tools for the
  static tool-name precheck (CRI-173 upgrade path). Both tags point at the
  same release commit (the CRI-171 release was re-tagged once).

## [v0.5.1] — 2026-06-05

- Remote-chunk helpers + full v2 test suite (reconcile pass).

## [v0.5.0] — 2026-06-05

- Protocol v2 wire contract foundation (Go bindings).