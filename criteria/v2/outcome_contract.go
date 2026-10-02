package criteriav2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// This file is the REFERENCE implementation of the pinned outcome-contract
// validation algorithm (conformance/README.md, v0.7.0 / KB-44). Every SDK
// implementation mirrors it and the committed conformance vectors pin its
// behavior byte-for-byte — do not change it without regenerating the vectors
// and updating the README in the same change.

// Static members of the pinned issue-string vocabulary. Parameterized issues
// (outcome_not_allowed, outcome_uncontracted, missing_comment, and the
// payload_schema property rules) are formatted at their emit sites with the
// exact templates pinned in conformance/README.md.
const (
	issueNoResult      = "no_result: step ended without a finalized result"
	issueEmptyOutcome  = "empty_outcome: result has no outcome"
	issueNotJSONObject = "payload_schema: outputs_json does not decode to a JSON object"
	issueBadSchema     = "payload_schema: contract schema_json is not a valid schema"
)

// EvaluateOutcomeContracts applies the per-step outcome contracts carried on
// an ExecuteRequest to the results a step produced, returning the effective
// result to forward (nil when rejected) and the ordered issue list (nil when
// accepted). It is called with the ExecuteRequest that produced results and
// those results in arrival order — at most one terminal result is expected;
// extra entries are ignored (stream integrity is the engine's attempt loop's
// job, not this algorithm's).
//
// Mode rules (proto contract, v0.7.0):
//
//   - Legacy mode: req is nil or carries no outcome_contracts → behavior is
//     byte-for-byte pre-v0.7.0. results[0] is forwarded as-is with no issues;
//     name-only allowed_outcomes validation remains the host's attempt-loop
//     concern.
//   - Contract mode: req.outcome_contracts non-empty → the result is validated
//     against its contract (allowed_outcomes membership still applies when
//     the list is populated, per the one-deprecation-cycle compat matrix). A
//     valid result is forwarded VERBATIM — outputs_json must already be the
//     model-submitted, schema-validated payload, never a session-state
//     assembly. A zero-result step finalizes with the fallback contract (no
//     payload, no comment) or is rejected with no_result.
//
// Issues are emitted in the pinned order — outcome gates first, then
// missing_comment, then payload-schema rules — so rejections are comparable
// across SDKs. When issues are non-empty the host constructs the next
// attempt's ExecutionRejection with NewExecutionRejection and re-executes;
// the adapter's repair-mode handling lives in the ExecutionRejection docs.
func EvaluateOutcomeContracts(req *ExecuteRequest, results []*ExecuteResult) (*ExecuteResult, []string) {
	if req == nil || len(req.GetOutcomeContracts()) == 0 {
		// Legacy mode: forward verbatim, validate nothing here.
		if len(results) == 0 {
			return nil, nil
		}
		return results[0], nil
	}

	if len(results) == 0 {
		if fb := fallbackContract(req.GetOutcomeContracts()); fb != nil {
			return &ExecuteResult{Outcome: fb.GetName()}, nil
		}
		return nil, []string{issueNoResult}
	}

	r := results[0]
	name := r.GetOutcome()
	if name == "" {
		return nil, []string{issueEmptyOutcome}
	}
	if allowed := req.GetAllowedOutcomes(); len(allowed) > 0 && !slices.Contains(allowed, name) {
		return nil, []string{
			fmt.Sprintf("outcome_not_allowed: outcome \"%s\" is not in allowed_outcomes", name),
		}
	}
	contract := contractFor(req.GetOutcomeContracts(), name)
	if contract == nil {
		return nil, []string{
			fmt.Sprintf("outcome_uncontracted: outcome \"%s\" has no outcome_contracts entry", name),
		}
	}

	var issues []string
	if contract.GetRequireComment() && r.GetComment() == "" {
		issues = append(issues, fmt.Sprintf(
			"missing_comment: outcome \"%s\" requires a comment (require_comment)", name))
	}
	if schema := contract.GetSchemaJson(); len(schema) > 0 {
		issues = append(issues, ValidatePayloadSchema(schema, r.GetOutputsJson())...)
	}
	if len(issues) == 0 {
		return r, nil
	}
	return nil, issues
}

// NewExecutionRejection builds the ExecutionRejection the host attaches to the
// NEXT Execute after a rejected result. attempt is the 1-based repair counter:
// it increments the prior rejection's attempt, with no prior rejection (a
// fresh step) starting at 1. issues are joined with "\n" into the diagnostic
// text adapters quote into the repair prompt (they must not parse it).
func NewExecutionRejection(outcome string, issues []string, prior *ExecutionRejection) *ExecutionRejection {
	var priorAttempt uint32
	if prior != nil {
		priorAttempt = prior.GetAttempt()
	}
	return &ExecutionRejection{
		Outcome: outcome,
		Issues:  strings.Join(issues, "\n"),
		Attempt: priorAttempt + 1,
	}
}

// contractFor returns the first contract governing name. Duplicate names in
// one request are a host compile error; on malformed input the first entry
// wins deterministically.
func contractFor(contracts []*OutcomeContract, name string) *OutcomeContract {
	for _, c := range contracts {
		if c.GetName() == name {
			return c
		}
	}
	return nil
}

// fallbackContract returns the single no-finalize fallback contract. The host
// enforces at most one at compile time; on malformed input (zero or several)
// nil / the first respectively wins deterministically.
func fallbackContract(contracts []*OutcomeContract) *OutcomeContract {
	for _, c := range contracts {
		if c.GetFallback() {
			return c
		}
	}
	return nil
}

// ValidatePayloadSchema validates outputs_json against one outcome's
// schema_json, restricted to the pinned JSON Schema subset in
// conformance/README.md (no external schema dependency, byte-identical
// behavior across SDKs).
//
// Pins (pinned): the three payload rules are exclusive lanes — exactly one of
// them produces the issue list, in this priority: (1) outputs_json must decode
// to a JSON object (nil/empty payloads and the literal null are treated as an
// empty object, i.e. they do not take this lane); (2) schema_json must parse
// as the pinned subset; (3) otherwise one rule per required property in the
// schema's required-array order (presence before type, wrong-typed values
// reported, missing values reported). A required property declared without a
// properties type is presence-checked only; non-required properties are never
// validated. Unknown leaf keywords are ignored so schemas stay subset-stable
// across generations.
func ValidatePayloadSchema(schemaJSON, outputsJSON []byte) []string {
	payload, payloadIssue := decodeJSONObject(outputsJSON)
	if payloadIssue != "" {
		return []string{payloadIssue}
	}

	schema, schemaIssue := parseOutcomeSchema(schemaJSON)
	if schemaIssue != "" {
		return []string{schemaIssue}
	}

	var issues []string
	for _, prop := range schema.Required {
		raw, present := payload[prop]
		if !present {
			issues = append(issues, fmt.Sprintf(
				"payload_schema: property \"%s\": required property is missing", prop))
			continue
		}
		want, declared := schema.Properties[prop]
		if !declared || want == "" {
			continue // presence-only property
		}
		if got := jsonTypeName(raw); got != want {
			issues = append(issues, fmt.Sprintf(
				"payload_schema: property \"%s\": expected \"%s\", got \"%s\"", prop, want, got))
		}
	}
	return issues
}

// decodeJSONObject decodes outputs_json into its top-level object map.
// Missing, empty, and literal-null payloads are an empty object (JSON Schema
// "absent = {}" semantics for a required-property presence pass, which then
// reports the required properties as missing). Anything else that does not
// decode to exactly one JSON object yields the pinned vocabulary issue.
func decodeJSONObject(outputsJSON []byte) (map[string]json.RawMessage, string) {
	payload := bytes.TrimSpace(outputsJSON)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return map[string]json.RawMessage{}, ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, issueNotJSONObject
	}
	return obj, ""
}

// outcomeSchema is the parsed pinned-schema subset: the required-name list in
// declared order and the declared property types (absent = presence-only).
type outcomeSchema struct {
	Required   []string
	Properties map[string]string
}

// schemaProperty carries the leaf keywords the subset understands; unknown
// leaf keywords (maxLength, pattern, …) are deliberately ignored.
type schemaProperty struct {
	Type *string `json:"type"`
}

// parseOutcomeSchema parses schema_json into the pinned subset, mapping every
// shape outside the subset — unparseable bytes, non-object root, a root type
// other than "object", a malformed required list or property set, or a leaf
// type outside the pinned enum — to the single validity issue (the parser
// error itself is language-specific and is never part of the issue text).
func parseOutcomeSchema(schemaJSON []byte) (*outcomeSchema, string) {
	payload := bytes.TrimSpace(schemaJSON)
	if len(payload) == 0 {
		return nil, issueBadSchema
	}

	var doc struct {
		Type       *string                   `json:"type"`
		Required   *[]string                 `json:"required"`
		Properties map[string]schemaProperty `json:"properties"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, issueBadSchema
	}
	if doc.Type != nil && *doc.Type != "object" {
		return nil, issueBadSchema
	}

	schema := &outcomeSchema{}
	if doc.Required != nil {
		schema.Required = *doc.Required
	}
	if doc.Properties != nil {
		schema.Properties = make(map[string]string, len(doc.Properties))
		for name, prop := range doc.Properties {
			if prop.Type != nil && !slices.Contains(schemaLeafTypes, *prop.Type) {
				return nil, issueBadSchema
			}
			schema.Properties[name] = orEmpty(prop.Type)
		}
	}
	return schema, ""
}

// schemaLeafTypes is the pinned leaf type enum of the schema subset.
var schemaLeafTypes = []string{"string", "number", "boolean", "object", "array"}

func orEmpty(t *string) string {
	if t == nil {
		return ""
	}
	return *t
}

// jsonTypeName maps a JSON value to the pinned type vocabulary used in
// property type issues: object, array, boolean, number, string, null.
func jsonTypeName(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "invalid" // unreachable for properties of a decoded object
	}
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "invalid"
	}
}
