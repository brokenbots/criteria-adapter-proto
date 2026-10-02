package criteriav2_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/brokenbots/criteria-adapter-proto/conformance"
	criteriav2 "github.com/brokenbots/criteria-adapter-proto/criteria/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ─── Conformance vectors ─────────────────────────────────────────────────────

type contractPhase struct {
	Request json.RawMessage   `json:"request"`
	Results []json.RawMessage `json:"results"`
	Expect  struct {
		Accepted  *bool           `json:"accepted"`
		Forwarded json.RawMessage `json:"forwarded"`
		Issues    []string        `json:"issues"`
		Rejection json.RawMessage `json:"rejection"`
	} `json:"expect"`
}

func loadContractVectors(t *testing.T) map[string][]contractPhase {
	t.Helper()
	names, err := conformance.VectorNames()
	require.NoError(t, err)
	require.Len(t, names, 7, "vector inventory changed; update the pinned tests too")

	phases := make(map[string][]contractPhase, len(names))
	for _, name := range names {
		raw, err := conformance.ReadVector(name)
		require.NoError(t, err)
		var doc struct {
			Name   string          `json:"name"`
			Phases []contractPhase `json:"phases"`
		}
		require.NoError(t, json.Unmarshal(raw, &doc), "vector %s", name)
		phases[name] = doc.Phases
	}
	return phases
}

// TestEvaluateOutcomeContracts_ConformanceVectors runs every committed
// conformance vector through the reference evaluator: same bytes, same
// algorithm the three SDK suites must reproduce.
func TestEvaluateOutcomeContracts_ConformanceVectors(t *testing.T) {
	for name, phases := range loadContractVectors(t) {
		t.Run(name, func(t *testing.T) {
			for pi, phase := range phases {
				req := &criteriav2.ExecuteRequest{}
				require.NoError(t, protojson.Unmarshal(phase.Request, req), "phase %d request", pi)

				var results []*criteriav2.ExecuteResult
				for _, raw := range phase.Results {
					r := &criteriav2.ExecuteResult{}
					require.NoError(t, protojson.Unmarshal(raw, r), "phase %d result", pi)
					results = append(results, r)
				}

				got, issues := criteriav2.EvaluateOutcomeContracts(req, results)

				require.NotNil(t, phase.Expect.Accepted, "phase %d expect.accepted", pi)
				if *phase.Expect.Accepted {
					require.Empty(t, issues, "phase %d must accept with no issues", pi)
					require.NotNil(t, got, "phase %d must forward a result", pi)
					want := &criteriav2.ExecuteResult{}
					require.NoError(t, protojson.Unmarshal(phase.Expect.Forwarded, want))
					require.True(t, proto.Equal(want, got),
						"phase %d must forward verbatim: want %v, got %v", pi, want, got)
				} else {
					require.Nil(t, got, "phase %d must not forward a rejected result", pi)
					require.Equal(t, phase.Expect.Issues, issues,
						"phase %d issue list must match exactly and in order", pi)

					wantRej := &criteriav2.ExecutionRejection{}
					require.NoError(t, protojson.Unmarshal(phase.Expect.Rejection, wantRej))
					var rejectedOutcome string
					if len(results) > 0 {
						rejectedOutcome = results[0].GetOutcome()
					} else {
						rejectedOutcome = wantRej.GetOutcome()
					}
					wantBuilt := criteriav2.NewExecutionRejection(rejectedOutcome, issues, req.GetRejection())
					require.True(t, proto.Equal(wantRej, wantBuilt),
						"phase %d rejection must be the pinned construction: want %v, got %v",
						pi, wantRej, wantBuilt)
				}
			}
		})
	}
}

// ─── EvaluateOutcomeContracts edge behavior ─────────────────────────────────

func contractModeReq(contracts []*criteriav2.OutcomeContract, allowed ...string) *criteriav2.ExecuteRequest {
	return &criteriav2.ExecuteRequest{
		SessionId:        "sess-contract",
		StepName:         "summarize",
		AllowedOutcomes:  allowed,
		OutcomeContracts: contracts,
	}
}

const contractSchema = `{"type":"object","required":["summary","exit_code"],"properties":{"summary":{"type":"string"},"exit_code":{"type":"number"}}}`

// TestEvaluateOutcomeContracts_LegacyMode pins that a nil request or an empty
// contracts list never validates anything: the step must behave byte-for-byte
// like pre-v0.7.0, forwarding results[0] untouched even when a result would
// fail every contract-mode rule.
func TestEvaluateOutcomeContracts_LegacyMode(t *testing.T) {
	wouldFail := &criteriav2.ExecuteResult{
		Outcome:     "bogus-never-allowed",
		OutputsJson: []byte(`{"summary":true}`),
	}
	for name, req := range map[string]*criteriav2.ExecuteRequest{
		"nil request":   nil,
		"no contracts":  contractModeReq(nil, "completed"),
		"empty list":    contractModeReq([]*criteriav2.OutcomeContract{}, "completed"),
		"unset allowed": contractModeReq(nil),
	} {
		t.Run(name, func(t *testing.T) {
			got, issues := criteriav2.EvaluateOutcomeContracts(req, []*criteriav2.ExecuteResult{wouldFail})
			require.Empty(t, issues)
			require.Same(t, wouldFail, got, "legacy mode must forward results[0] verbatim")

			got, issues = criteriav2.EvaluateOutcomeContracts(req, nil)
			require.Empty(t, issues)
			require.Nil(t, got, "legacy mode with no results forwards nothing")
		})
	}
}

// TestEvaluateOutcomeContracts_Fallback pins the no-finalize path: the
// fallback contract finalizes without payload or comment, and the evaluator
// picks the FIRST fallback deterministically (the host compile enforces ≤1).
func TestEvaluateOutcomeContracts_Fallback(t *testing.T) {
	first := &criteriav2.OutcomeContract{Name: "failed", Fallback: true}
	second := &criteriav2.OutcomeContract{Name: "escalated", Fallback: true, SchemaJson: []byte(contractSchema)}
	req := contractModeReq([]*criteriav2.OutcomeContract{first, second}, "completed", "failed", "escalated")

	got, issues := criteriav2.EvaluateOutcomeContracts(req, nil)
	require.Empty(t, issues)
	require.True(t, proto.Equal(&criteriav2.ExecuteResult{Outcome: "failed"}, got),
		"fallback finalize must carry only the outcome name")

	// No fallback contract → the step is rejected, not finalized.
	plain := contractModeReq([]*criteriav2.OutcomeContract{{Name: "completed"}}, "completed")
	got, issues = criteriav2.EvaluateOutcomeContracts(plain, nil)
	require.Nil(t, got)
	require.Equal(t, []string{"no_result: step ended without a finalized result"}, issues)
}

// TestEvaluateOutcomeContracts_GateShortCircuit pins the issue-order gates:
// a result failing an earlier gate is not comment- or schema-validated, so
// exactly one issue comes out per gate even with garbage payloads attached.
func TestEvaluateOutcomeContracts_GateShortCircuit(t *testing.T) {
	completed := &criteriav2.OutcomeContract{Name: "completed", SchemaJson: []byte(contractSchema), RequireComment: true}
	req := contractModeReq([]*criteriav2.OutcomeContract{completed}, "completed", "failed")

	for _, tc := range []struct {
		name   string
		result *criteriav2.ExecuteResult
		want   []string
	}{
		{
			name:   "empty outcome",
			result: &criteriav2.ExecuteResult{OutputsJson: []byte(`{`), Comment: ""},
			want:   []string{"empty_outcome: result has no outcome"},
		},
		{
			name:   "outcome not allowed",
			result: &criteriav2.ExecuteResult{Outcome: "escalated", Comment: "", OutputsJson: []byte(`{`)},
			want:   []string{`outcome_not_allowed: outcome "escalated" is not in allowed_outcomes`},
		},
		{
			name:   "outcome uncontracted",
			result: &criteriav2.ExecuteResult{Outcome: "failed", Comment: "", OutputsJson: []byte(`{`)},
			want:   []string{`outcome_uncontracted: outcome "failed" has no outcome_contracts entry`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, issues := criteriav2.EvaluateOutcomeContracts(req, []*criteriav2.ExecuteResult{tc.result})
			require.Nil(t, got)
			require.Equal(t, tc.want, issues)
		})
	}
}

// TestEvaluateOutcomeContracts_MultipleResults pins that only results[0] is
// judged: trailing stream entries (e.g. a duplicate emission) must not flip an
// accepted step into rejection.
func TestEvaluateOutcomeContracts_MultipleResults(t *testing.T) {
	valid := &criteriav2.ExecuteResult{
		Outcome:     "completed",
		Comment:     "done",
		OutputsJson: []byte(`{"summary":"ok","exit_code":0}`),
	}
	garbage := &criteriav2.ExecuteResult{Outcome: "never"}
	req := contractModeReq([]*criteriav2.OutcomeContract{{Name: "completed", SchemaJson: []byte(contractSchema)}}, "completed")

	got, issues := criteriav2.EvaluateOutcomeContracts(req, []*criteriav2.ExecuteResult{valid, garbage})
	require.Empty(t, issues)
	require.Same(t, valid, got)
}

// TestEvaluateOutcomeContracts_IssueOrdering pins that comment and payload
// issues from the same contract emit in the pinned order: missing_comment
// first, then payload-schema rules in required-array order.
func TestEvaluateOutcomeContracts_IssueOrdering(t *testing.T) {
	req := contractModeReq([]*criteriav2.OutcomeContract{{
		Name:           "completed",
		SchemaJson:     []byte(contractSchema),
		RequireComment: true,
	}}, "completed")
	bad := &criteriav2.ExecuteResult{
		Outcome:     "completed",
		OutputsJson: []byte(`{"summary":42,"exit_code":"zero"}`),
	}
	got, issues := criteriav2.EvaluateOutcomeContracts(req, []*criteriav2.ExecuteResult{bad})
	require.Nil(t, got)
	require.Equal(t, []string{
		`missing_comment: outcome "completed" requires a comment (require_comment)`,
		`payload_schema: property "summary": expected "string", got "number"`,
		`payload_schema: property "exit_code": expected "number", got "string"`,
	}, issues)
}

// TestEvaluateOutcomeContracts_MissingCommentLiteralQuotes pins that the
// parameterized issue strings use literal double quotes (NOT language-native
// quoting like Go's %q): outcome names go through byte-identical assembly in
// every SDK, so a name carrying a quote or non-ASCII rune must surface
// verbatim, never escaped.
func TestEvaluateOutcomeContracts_MissingCommentLiteralQuotes(t *testing.T) {
	req := contractModeReq([]*criteriav2.OutcomeContract{{
		Name:           `weird"name é`,
		RequireComment: true,
	}}, `weird"name é`)
	got, issues := criteriav2.EvaluateOutcomeContracts(req, []*criteriav2.ExecuteResult{
		{Outcome: `weird"name é`},
	})
	require.Nil(t, got)
	require.Equal(t, []string{
		`missing_comment: outcome "weird"name é" requires a comment (require_comment)`,
	}, issues)
}

// TestNewExecutionRejection pins the repair-backstop construction: issues are
// joined with "\n" and the attempt counter increments the prior attempt,
// starting at 1 on a fresh step (no prior rejection).
func TestNewExecutionRejection(t *testing.T) {
	issues := []string{
		`payload_schema: property "summary": expected "string", got "number"`,
		`payload_schema: property "exit_code": expected "number", got "string"`,
	}

	fresh := criteriav2.NewExecutionRejection("completed", issues, nil)
	require.True(t, proto.Equal(&criteriav2.ExecutionRejection{
		Outcome: "completed",
		Issues:  strings.Join(issues, "\n"),
		Attempt: 1,
	}, fresh))

	prior := &criteriav2.ExecutionRejection{Outcome: "completed", Attempt: 3}
	got := criteriav2.NewExecutionRejection("completed", issues, prior)
	require.True(t, proto.Equal(&criteriav2.ExecutionRejection{
		Outcome: "completed",
		Issues:  strings.Join(issues, "\n"),
		Attempt: 4,
	}, got), "attempt must increment the prior rejection's attempt")
}

// TestValidatePayloadSchema_Edges pins the pinned-schema-subset behavior the
// vectors cannot cover individually: empty/null payloads, non-object payloads,
// invalid schemas, presence-only properties, ignored leaf keywords, and every
// pinned JSON type name.
func TestValidatePayloadSchema_Edges(t *testing.T) {
	schema := []byte(contractSchema)

	for _, tc := range []struct {
		name    string
		payload []byte
		want    []string
	}{
		{
			name:    "empty payload reports every required property missing, in required order",
			payload: nil,
			want: []string{
				`payload_schema: property "summary": required property is missing`,
				`payload_schema: property "exit_code": required property is missing`,
			},
		},
		{
			name:    "literal null payload behaves like an empty object",
			payload: []byte("null"),
			want: []string{
				`payload_schema: property "summary": required property is missing`,
				`payload_schema: property "exit_code": required property is missing`,
			},
		},
		{
			name:    "array payload is not a JSON object",
			payload: []byte(`["summary"]`),
			want:    []string{"payload_schema: outputs_json does not decode to a JSON object"},
		},
		{
			name:    "string payload is not a JSON object",
			payload: []byte(`"ok"`),
			want:    []string{"payload_schema: outputs_json does not decode to a JSON object"},
		},
		{
			name:    "trailing garbage is not a JSON object",
			payload: []byte(`{} trailing`),
			want:    []string{"payload_schema: outputs_json does not decode to a JSON object"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := criteriav2.ValidatePayloadSchema(schema, tc.payload)
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("invalid schema bytes map to the single validity issue", func(t *testing.T) {
		for _, bad := range []string{
			`{`,
			`[]`,
			`"object"`,
			`{"type":"string","required":["x"]}`,
			`{"required":[1]}`,
			`{"properties":{"x":{"type":"pattern"}}}`,
			`{"properties":{"x":true}}`,
		} {
			got := criteriav2.ValidatePayloadSchema([]byte(bad), []byte(`{"x":1}`))
			require.Equal(t,
				[]string{"payload_schema: contract schema_json is not a valid schema"},
				got, "schema %s", bad)
		}
		// {"required":["x"]} with no properties entry IS a valid subset schema
		// (presence-only check), not an invalid one.
		require.Empty(t, criteriav2.ValidatePayloadSchema(
			[]byte(`{"required":["x"]}`), []byte(`{"x":1}`)))
	})

	t.Run("payload-decode failure is its own exclusive lane", func(t *testing.T) {
		got := criteriav2.ValidatePayloadSchema([]byte(`{`), []byte(`[1]`))
		require.Equal(t,
			[]string{"payload_schema: outputs_json does not decode to a JSON object"}, got,
			"when outputs_json does not decode to an object, schema and property rules do not run")
	})

	t.Run("unknown leaf keywords are ignored", func(t *testing.T) {
		withUnknown := []byte(`{"type":"object","required":["summary"],"properties":{"summary":{"type":"string","maxLength":5,"pattern":"^x"}}}`)
		require.Empty(t, criteriav2.ValidatePayloadSchema(withUnknown, []byte(`{"summary":"ok"}`)))
		require.Equal(t,
			[]string{`payload_schema: property "summary": expected "string", got "number"`},
			criteriav2.ValidatePayloadSchema(withUnknown, []byte(`{"summary":5}`)))
	})

	t.Run("required property without a declared type is presence-checked only", func(t *testing.T) {
		presenceOnly := []byte(`{"type":"object","required":["anything"],"properties":{}}`)
		require.Empty(t, criteriav2.ValidatePayloadSchema(presenceOnly, []byte(`{"anything":[1,{"a":2}]}`)))
		require.Equal(t,
			[]string{`payload_schema: property "anything": required property is missing`},
			criteriav2.ValidatePayloadSchema(presenceOnly, []byte(`{}`)))
	})

	t.Run("non-required properties are never validated", func(t *testing.T) {
		got := criteriav2.ValidatePayloadSchema(schema, []byte(`{"summary":"ok","exit_code":0,"extra":[1],"nest":{"x":true}}`))
		require.Empty(t, got)
	})

	t.Run("every pinned JSON type name is reported", func(t *testing.T) {
		typed := []byte(`{"required":["s","n","b","o","a"],"properties":{"s":{"type":"string"},"n":{"type":"number"},"b":{"type":"boolean"},"o":{"type":"object"},"a":{"type":"array"}}}`)
		payload := []byte(`{"s":1,"n":"one","b":1,"o":[],"a":{}}`)
		got := criteriav2.ValidatePayloadSchema(typed, payload)
		require.Equal(t, []string{
			`payload_schema: property "s": expected "string", got "number"`,
			`payload_schema: property "n": expected "number", got "string"`,
			`payload_schema: property "b": expected "boolean", got "number"`,
			`payload_schema: property "o": expected "object", got "array"`,
			`payload_schema: property "a": expected "array", got "object"`,
		}, got)

		got = criteriav2.ValidatePayloadSchema(typed, []byte(`{"s":null}`))
		require.Equal(t, []string{
			`payload_schema: property "s": expected "string", got "null"`,
			// s consumed; remaining required names reported missing
			`payload_schema: property "n": required property is missing`,
			`payload_schema: property "b": required property is missing`,
			`payload_schema: property "o": required property is missing`,
			`payload_schema: property "a": required property is missing`,
		}, got)
	})

	t.Run("matching types validate clean", func(t *testing.T) {
		got := criteriav2.ValidatePayloadSchema(schema, []byte(`{"summary":"ok","exit_code":2.5}`))
		require.Empty(t, got)
	})
}

// ─── Wire contract ───────────────────────────────────────────────────────────

// TestOutcomeContractMessages_WireFieldNumbers pins the v0.7.0 wire contract:
// ExecuteRequest.outcome_contracts is field 6 and rejection is field 7
// (allowed_outcomes stays 5), ExecuteResult.comment is field 5, and the new
// messages carry their pinned field numbers and kinds.
func TestOutcomeContractMessages_WireFieldNumbers(t *testing.T) {
	fd := criteriav2.File_criteria_v2_adapter_proto
	require.NotNil(t, fd, "adapter proto descriptor must be registered")

	reqDesc := fd.Messages().ByName("ExecuteRequest")
	require.NotNil(t, reqDesc)
	contracts := reqDesc.Fields().ByName("outcome_contracts")
	require.NotNil(t, contracts, "ExecuteRequest.outcome_contracts must exist on the wire")
	assert.Equal(t, protoreflect.FieldNumber(6), contracts.Number(),
		"outcome_contracts must be field 6 (allowed_outcomes stays 5)")
	assert.True(t, contracts.IsList(), "outcome_contracts must be repeated")
	assert.Equal(t, "criteria.v2.OutcomeContract", string(contracts.Message().FullName()),
		"outcome_contracts must be a repeated OutcomeContract")
	rejection := reqDesc.Fields().ByName("rejection")
	require.NotNil(t, rejection, "ExecuteRequest.rejection must exist on the wire")
	assert.Equal(t, protoreflect.FieldNumber(7), rejection.Number(),
		"rejection must be field 7 (the repair backstop)")
	assert.True(t, rejection.HasPresence(),
		"rejection must be presence-tracked: absent = normal execute")

	resDesc := fd.Messages().ByName("ExecuteResult")
	require.NotNil(t, resDesc)
	comment := resDesc.Fields().ByName("comment")
	require.NotNil(t, comment, "ExecuteResult.comment must exist on the wire")
	assert.Equal(t, protoreflect.FieldNumber(5), comment.Number(),
		"comment must be field 5 (outputs_json stays 4)")

	contractDesc := fd.Messages().ByName("OutcomeContract")
	require.NotNil(t, contractDesc)
	assert.Equal(t, protoreflect.FieldNumber(1), contractDesc.Fields().ByName("name").Number())
	schemaJSON := contractDesc.Fields().ByName("schema_json")
	require.NotNil(t, schemaJSON)
	assert.Equal(t, protoreflect.FieldNumber(2), schemaJSON.Number())
	assert.Equal(t, protoreflect.BytesKind, schemaJSON.Kind(), "schema_json must be bytes")
	assert.Equal(t, protoreflect.FieldNumber(3), contractDesc.Fields().ByName("fallback").Number())
	assert.Equal(t, protoreflect.FieldNumber(4), contractDesc.Fields().ByName("require_comment").Number())

	rejectionDesc := fd.Messages().ByName("ExecutionRejection")
	require.NotNil(t, rejectionDesc)
	assert.Equal(t, protoreflect.FieldNumber(1), rejectionDesc.Fields().ByName("outcome").Number())
	assert.Equal(t, protoreflect.FieldNumber(2), rejectionDesc.Fields().ByName("issues").Number())
	attempt := rejectionDesc.Fields().ByName("attempt")
	require.NotNil(t, attempt)
	assert.Equal(t, protoreflect.FieldNumber(3), attempt.Number())
	assert.Equal(t, protoreflect.Uint32Kind, attempt.Kind(), "attempt must be uint32")
}

// putRecord appends a length-delimited field (string/bytes/message) with
// varint-encoded tag and varint-encoded length — real protobuf wire bytes
// produced WITHOUT the v0.7.0 serializer, so the compat tests prove old-shape
// bytes decode into the new bindings.
func putRecord(buf *bytes.Buffer, no protoreflect.FieldNumber, payload []byte) {
	buf.Write(varint(uint32(no)<<3 | 2))
	buf.Write(varint(uint32(len(payload))))
	buf.Write(payload)
}

// putVarint appends a varint field with a varint-encoded tag.
func putVarint(buf *bytes.Buffer, no protoreflect.FieldNumber, v uint32) {
	buf.Write(varint(uint32(no) << 3))
	buf.Write(varint(v))
}

func varint(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			out = append(out, b|0x80)
		} else {
			return append(out, b)
		}
	}
}

// TestExecuteResult_LegacyWireCompat pins cross-version wire compatibility: a
// v0.6.0-shaped ExecuteResult (fields 1/3/4 only — outcome, chunk unset,
// outputs_json) decodes into the v0.7.0 bindings with comment empty, and a
// v0.7.0 comment decodes from its field-5 bytes.
func TestExecuteResult_LegacyWireCompat(t *testing.T) {
	var legacy bytes.Buffer
	putRecord(&legacy, 1, []byte("completed"))
	putRecord(&legacy, 4, []byte(`{"summary":"ok"}`))

	got := &criteriav2.ExecuteResult{}
	require.NoError(t, proto.Unmarshal(legacy.Bytes(), got),
		"v0.6.0 ExecuteResult wire bytes must decode into the v0.7.0 bindings")
	assert.Equal(t, "completed", got.GetOutcome())
	assert.Equal(t, `{"summary":"ok"}`, string(got.GetOutputsJson()))
	assert.Empty(t, got.GetComment(), "legacy bytes carry no comment — it must stay empty")

	var withComment bytes.Buffer
	putRecord(&withComment, 1, []byte("completed"))
	putRecord(&withComment, 5, []byte("finalize comment"))
	got = &criteriav2.ExecuteResult{}
	require.NoError(t, proto.Unmarshal(withComment.Bytes(), got))
	assert.Equal(t, "finalize comment", got.GetComment())
}

// TestExecuteRequest_NewFieldsWireCompat pins that the new ExecuteRequest
// fields decode from hand-rolled wire bytes: nested OutcomeContract messages
// on field 6, a nested ExecutionRejection on field 7, and that an unknown
// far-future field is preserved (proto3 forward safety).
func TestExecuteRequest_NewFieldsWireCompat(t *testing.T) {
	var contractMsg bytes.Buffer
	putRecord(&contractMsg, 1, []byte("completed"))
	putRecord(&contractMsg, 2, []byte(contractSchema))
	putVarint(&contractMsg, 4, 1) // require_comment = true

	var rejectionMsg bytes.Buffer
	putRecord(&rejectionMsg, 1, []byte("completed"))
	putRecord(&rejectionMsg, 2, []byte("payload_schema: property \"summary\": required property is missing"))
	putVarint(&rejectionMsg, 3, 2)

	var wire bytes.Buffer
	putRecord(&wire, 1, []byte("sess-contract"))
	putRecord(&wire, 6, contractMsg.Bytes())
	putRecord(&wire, 7, rejectionMsg.Bytes())
	putVarint(&wire, 1001, 7) // unknown far-future field

	got := &criteriav2.ExecuteRequest{}
	require.NoError(t, proto.Unmarshal(wire.Bytes(), got))
	require.Len(t, got.GetOutcomeContracts(), 1, "field 6 must decode as repeated OutcomeContract")
	assert.Equal(t, "completed", got.GetOutcomeContracts()[0].GetName())
	assert.Equal(t, contractSchema, string(got.GetOutcomeContracts()[0].GetSchemaJson()))
	assert.True(t, got.GetOutcomeContracts()[0].GetRequireComment())
	require.NotNil(t, got.GetRejection(), "field 7 must decode as the ExecutionRejection")
	assert.Equal(t, "completed", got.GetRejection().GetOutcome())
	assert.Equal(t, uint32(2), got.GetRejection().GetAttempt())

	roundTripReq := roundTrip(t, got)
	require.True(t, proto.Equal(got, roundTripReq),
		"contract-mode requests must survive a proto wire round-trip byte-faithfully")
}

// TestExecuteResult_CommentRoundTrip pins that ExecuteResult.comment survives
// a proto wire round-trip with payload and outcome intact (the end-to-end
// comment plumbing vector 06 pins at the JSON layer).
func TestExecuteResult_CommentRoundTrip(t *testing.T) {
	msg := &criteriav2.ExecuteResult{
		Outcome:     "completed",
		Comment:     "release notes drafted",
		OutputsJson: []byte(`{"summary":"ok","exit_code":0}`),
	}
	got := roundTrip(t, msg)
	require.True(t, proto.Equal(msg, got))
	assert.Equal(t, "release notes drafted", got.GetComment())
}
