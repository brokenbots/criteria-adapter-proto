# Outcome-contract conformance vectors

Shared, byte-identical fixture vectors for the v0.7.0 per-outcome payload
contract (KB-44 outcome-contract wave). Consumers: the Criteria engine suite
and the three SDK repositories —

- [criteria-go-adapter-sdk](https://github.com/brokenbots/criteria-go-adapter-sdk)
- [criteria-python-adapter-sdk](https://github.com/brokenbots/criteria-python-adapter-sdk)
- [criteria-typescript-adapter-sdk](https://github.com/brokenbots/criteria-typescript-adapter-sdk)

— which run the SAME bytes through their own test suites. The files are
committed here (the proto contract is the single source of truth) and are
pinned per release: consumers read them from the repo at the tagged version
they pin (the Go package embeds them — see `embed.go`).

## Vector format

One JSON document per scenario under `vectors/`, encoded in proto3 canonical
JSON so any proto3-JSON implementation (Go `protojson`, protobuf-es,
`json_format`) parses it without adapters:

- message fields carry their lowerCamel JSON names
  (`sessionId`, `outcomeContracts`, `schemaJson`, `requireComment`,
  `outputsJson`, `allowedOutcomes`, `stepName`, …);
- `bytes` fields (`schema_json`, `outputs_json`) are standard base64:
  `schemaJson` decodes to JSON text (see the pinned schema subset below),
  `outputsJson` decodes to the exact payload bytes that the adapter returned;
- `uint32` values are plain JSON numbers.
- Key order inside a document is not load-bearing — consumers parse, they do
  not diff file text. The committed files are generated with sorted keys for
  stable diffing.

Document shape:

```json
{
  "name": "<short human label>",
  "phases": [
    {
      "request":  { "//": "ExecuteRequest (proto3 JSON)" },
      "results":  [ { "//": "ExecuteResult(s) that arrived on the stream, in arrival order" } ],
      "expect": {
        "accepted":  true,
        "forwarded": { "//": "ExecuteResult forwarded / effective after contract mode — present iff accepted" },
        "issues":    [ "// exact, ordered issue strings — present iff rejected" ],
        "rejection": { "//": "ExecutionRejection the NEXT Execute must carry — present iff rejected" }
      }
    }
  ]
}
```

Multi-phase vectors ("phases") express a rejection-repair interaction:
phase N+1's `request.rejection` is phase N's `expect.rejection`, exactly as
the host retry loop re-sends it.

## Pinned validation algorithm (contract mode)

The reference implementation is `criteriav2.EvaluateOutcomeContracts` in
[.../criteria/v2/outcome_contract.go](../criteria/v2/outcome_contract.go).
Every SDK mirrors it; vectors pin the behavior below. Inputs: the
`ExecuteRequest` (`req`) and the results that arrived on the stream in arrival
order (`results`). Output: the effective `ExecuteResult` (`nil` when
rejected) plus an ordered issue list (empty = accepted).

1. **Legacy mode** — `req` is nil, or `req.outcome_contracts` is empty:
   behavior is byte-for-byte pre-v0.7.0. The algorithm returns `results[0]`
   (or `nil` when no result arrived) and NO issues. Name-only
   `allowed_outcomes` validation is the legacy host behavior and is exercised
   by the `no_contracts` vector (a result that contract mode would reject is
   accepted here).
2. **Fallback finalize** — contract mode with `len(results) == 0`: the step
   ended without a finalization. If a contract has `fallback: true`, the host
   finalizes with it and the effective result is `{outcome: <fallback name>}`
   — no comment, no payload. If there is no fallback contract the step is
   rejected with issue `no_result: step ended without a finalized result`.
3. **Result validation** — contract mode with `len(results) >= 1`: validate
   `results[0]` (`R`) and collect issues IN ORDER. Each gate is a
   SHORT-CIRCUIT: a result failing an earlier gate (empty outcome,
   not-allowed outcome, uncontracted outcome) stops there and is NOT
   comment- or schema-validated, since the gates below depend on locating its
   contract:
   1. `R.outcome` empty → `empty_outcome: result has no outcome`.
   2. `R.outcome` not in `req.allowed_outcomes` (only when that list is
      non-empty) → `outcome_not_allowed: outcome "<name>" is not in allowed_outcomes`.
   3. No contract with `name == R.outcome` →
      `outcome_uncontracted: outcome "<name>" has no outcome_contracts entry`.
   4. Contract has `require_comment: true` and `R.comment` is empty →
      `missing_comment: outcome "<name>" requires a comment (require_comment)`.
   5. Contract has non-empty `schemaJson` → append the payload-schema issues
      (below), in the order the rules emit them.
4. **Accept or reject** — zero issues: `R` is forwarded VERBATIM (same bytes
   for `outputsJson`, same `comment`). Any issue: nothing is forwarded, and
   the rejection-repair backstop constructs the next-attempt
   `ExecutionRejection` via
   `criteriav2.NewExecutionRejection(rejectedOutcome, issues, priorRejection)`:
   `outcome = R.outcome`, `issues = strings.Join(issues, "\n")`, `attempt =
   priorRejection.attempt + 1` (a request with no prior rejection → 1).

Contract mode ignores results beyond `results[0]`; stream integrity
(zero-or-one terminal result) is enforced by the engine's attempt loop, not by
this algorithm.

## Pinned payload-schema subset

`schemaJson` in the vectors is RESTRICTED to the deterministic subset below
so every SDK implements byte-identical validation without a JSON Schema
dependency. Full JSON Schema validation remains a host compile-time choice —
the wire contract does not mandate it.

- The schema is a JSON object: optional top-level `"type": "object"` (must be
  `"object"` when present), optional `"required"` (array of unique strings),
  optional `"properties"` (object of `{"<name>": {"type": "<t>"}}`).
- Leaf `"type"` ∈ `string | number | boolean | object | array`. No other leaf
  keywords appear in vector schemas (no `pattern`, `format`, nesting, …) —
  implementations IGNORE unknown leaf keywords to stay subset-stable across
  schema generations.
- `null` or absent `outputsJson` is treated as an empty object `{}`.
- `outputsJson` MUST decode to a JSON object when the schema exists; any
  other top-level JSON value →
  `payload_schema: outputs_json does not decode to a JSON object`.
- A schema that does not parse as the subset →
  `payload_schema: contract schema_json is not a valid schema` (exact text;
  do NOT include the parser error — it differs per language).

Issue rules — the three payload rules are EXCLUSIVE LANES, so the issue list
of a schema check is exactly one of: (1) the object-decode issue, (2) the
schema-validity issue, or (3) one issue per `"required"` entry in the schema's
`required`-array order. Within lane 3, presence issues come before type
issues per property, and a missing property yields only the presence issue:

- outputs_json does not decode to a JSON object (array, string, number, or
  malformed bytes; not reached for nil/empty/null payloads, which are an
  empty object) →
  `payload_schema: outputs_json does not decode to a JSON object`
  — property rules do NOT run against a payload that is not an object, and
  the schema-validity rule does not run either (single failing lane).
- schema_json does not parse as the pinned subset →
  `payload_schema: contract schema_json is not a valid schema`
  — property rules do not run without a valid schema.
- required property absent →
  `payload_schema: property "<name>": required property is missing`
- required property present with a different JSON type than
  `properties[<name>].type` →
  `payload_schema: property "<name>": expected "<type>", got "<actual>"`
  where `actual` ∈ `string | number | boolean | object | array | null`
  (JSON object → `object`, array → `array`, `true`/`false` → `boolean`,
  JSON number → `number`, string → `string`, `null` → `null`).
- a required property with NO declared `properties` type is presence-checked
  only (JSON Schema semantics); non-required properties are never validated,
  and extra properties are forwarded unchanged.

Complete issue-string vocabulary pinned here, verbatim:

```
no_result: step ended without a finalized result
empty_outcome: result has no outcome
outcome_not_allowed: outcome "<name>" is not in allowed_outcomes
outcome_uncontracted: outcome "<name>" has no outcome_contracts entry
missing_comment: outcome "<name>" requires a comment (require_comment)
payload_schema: outputs_json does not decode to a JSON object
payload_schema: contract schema_json is not a valid schema
payload_schema: property "<name>": required property is missing
payload_schema: property "<name>": expected "<type>", got "<actual>"
```

## Vector inventory

| File | Exercises |
|---|---|
| `01_contract_roundtrip_valid.json` | contract round-trip: valid payload forwarded verbatim |
| `02_contract_payload_invalid.json` | invalid payload rejected with issue list |
| `03_require_comment_missing.json` | `require_comment` with missing comment rejected |
| `04_fallback_fires.json` | fallback fires on never-finalized |
| `05_no_contracts_legacy.json` | no-contract step behaves exactly as pre-v0.7.0 |
| `06_comment_end_to_end.json` | `ExecuteResult.comment` plumbed end to end (wire round-trip) |
| `07_rejection_repair_context.json` | rejection-context Execute consumed correctly (repair path) |

## Consuming

Go: `github.com/brokenbots/criteria-adapter-proto/conformance` embeds the
files (`conformance.VectorFS`, `conformance.VectorNames`,
`conformance.ReadVector`) — same bytes at the pinned tag.

TypeScript / Python: read the raw files under `vectors/` from the same tag
(the vector files are committed in this repo; the npm/PyPI packages are
generated at publish time and do not ship them, so the SDK repos check out /
download this repo's vectors for their suites).