# Adapters: well-known event kinds

`AdapterEvent.event_kind` is a dot-separated identifier. Adapters may emit any
kind; kinds listed here are **well-known**: the host (and the conformance
suites) know their payload shape, and the contract below is stable across
versions. Unknown kinds are forwarded unchanged (see
`AdapterEvent` in [proto/criteria/v2/adapter.proto](../proto/criteria/v2/adapter.proto)).

Payload rules for ALL event kinds:

- The payload is a `google.protobuf.Struct`; chunked events reassemble
  `payload_json` fragments in `Chunk.seq` order before decoding (see the
  proto message docs) — the shapes below are the decoded value.
- Payload keys are lowerCamel and additive: new keys may appear over time
  without a version bump; consumers must ignore unknown keys. Existing keys
  are never renamed or re-typed (this is why `outcome.reprompt` keeps its
  camelCase keys while the proto contract itself is snake_case).
- `emitted_at` is advisory metadata; it never carries ordering semantics
  (ordering is the stream order).

## Registry

| Kind                   | Since | Payload shape | Notes |
|------------------------|-------|---------------|-------|
| `permission.request`   | v0.5  | `{"kind":"adapter_tool", …}` — full payload contract on `PermissionEvent` | Blocking permission / adapter-tool-call request; answered on the Permissions stream. |
| `outcome.finalized`    | v0.5¹ | `{"outcome":"<name>","reason":"<text>"}` | The adapter finalized the step. Standardized in v0.7.0 (¹ de-facto shape: emitted by existing production adapters, e.g. the GitHub Copilot coding agent adapter, with `reason` as the finalize comment). |
| `outcome.reprompt`     | v0.5² | `{"attempt":<int>,"maxAttempts":<int>}` | The adapter is retrying the step. Standardized in v0.7.0 (² de-facto shape: emitted by existing adapters, e.g. the Claude agent adapter). |
| `outcome.payload_invalid` | v0.7.0 | `{"issues":["<issue>", …]}` | NEW. Adapter-side contract rejection (see below). |
| `outcome.recovered`    | v0.7.0 | `{"attempt":<int>,"outcome":"<name>"}` | NEW. Adapter-side repair acknowledgment (see below). |

## outcome.finalized

The adapter finalized the step. Payload:

- `outcome` — the finalized outcome name (string; matches an
  `allowed_outcomes` entry).
- `reason` — the finalize comment (string). This is the same text that rides
  the `ExecuteResult.comment` wire field as of v0.7.0: the comment is
  first-class wire metadata, and the event is its point-in-time notification
  to operators. Contract mode (`OutcomeContract.require_comment`) validates
  `ExecuteResult.comment`, not this payload.

Emitted at most once per terminal result, on the Execute stream, alongside
the terminal `ExecuteEvent.result`.

## outcome.reprompt

The adapter is retrying its internal loop (model did not produce a usable
result and will be prompted again inside the same execution). Payload:

- `attempt` — the retry number that is about to start (1-based; first
  re-try is 1).
- `maxAttempts` — the adapter's configured maximum number of attempts
  (informational; the HOST attempt loop bound is negotiated separately and
  is not reported here).

Not a terminal event: the step may still finalize with any outcome afterward.

## outcome.payload_invalid (NEW, v0.7.0)

The adapter's own outcome-contract check rejected the model-submitted payload
BEFORE finalizing — the in-session half of the two-sided enforcement split
(the host-side half is the post-finalize validation that rejects a bad
`ExecuteResult` and re-executes with `ExecutionRejection`). Payload:

- `issues` — the issue list, verbatim in the conformance-vector format
  (`conformance/README.md` pins the issue-string vocabulary and ordering:
  outcome gates first, then `missing_comment`, then payload-schema rules in
  required-array order).

After emitting, the adapter repairs in-session (re-prompts the model for a
contract-satisfying payload) and finalizes with the same outcome — or, if it
cannot satisfy the contract, finalizes with a fallback outcome if allowed,
otherwise the step ends without a finalization and the host's no-finalize
path (`OutcomeContract.fallback` / `no_result` issue) governs. A rejected
payload is never silently forwarded as `outputs_json`.

## outcome.recovered (NEW, v0.7.0)

The adapter completed a host-rejection repair: a previous Execute for this
step was rejected (`ExecuteRequest.rejection` non-empty) and the adapter has
now resubmitted a corrected result. It marks the repair loop as working —
observability for the `ExecutionRejection` backstop. Payload:

- `attempt` — the rejection attempt count that was repaired (the
  `ExecutionRejection.attempt` value the adapter received).
- `outcome` — the outcome of the corrected, accepted submission.

Emitted on the Execute stream just before the corrected terminal
`ExecuteEvent.result`.

## Registration policy

A kind becomes well-known by landing in this table with a payload shape and a
"since" version in the same release that ships consumers. The proto comments
are the wire source of truth; this registry standardizes semantics and payload
shapes. Keep per-repo serialization doctrine: changes here ride this repo's
release (CHANGELOG lists the registry diff), and SDK conformance vectors
reference the standardized kinds.