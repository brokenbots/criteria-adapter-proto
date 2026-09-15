package criteriav2_test

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	criteriav2 "github.com/brokenbots/criteria-adapter-proto/criteria/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzUnmarshalAdapterMessages feeds random bytes to proto.Unmarshal for each
// top-level wire message type, guarding against panics from malformed inputs
// received from networked adapters (WS20).
func FuzzUnmarshalAdapterMessages(f *testing.F) {
	// Seed corpus from well-formed messages so the fuzzer starts from valid
	// states before exploring mutations.
	seeds := []proto.Message{
		&criteriav2.InfoRequest{},
		&criteriav2.InfoResponse{Name: "seed"},
		&criteriav2.OpenSessionRequest{SessionId: "s", Secrets: map[string]string{"k": "v"}},
		&criteriav2.OpenSessionResponse{},
		&criteriav2.ExecuteRequest{SessionId: "s", StepName: "step"},
		&criteriav2.ExecuteEvent{},
		&criteriav2.LogRequest{SessionId: "s"},
		&criteriav2.LogEvent{},
		&criteriav2.PermissionEvent{},
		&criteriav2.PermissionDecision{},
		&criteriav2.PauseRequest{SessionId: "s"},
		&criteriav2.PauseResponse{},
		&criteriav2.ResumeRequest{SessionId: "s"},
		&criteriav2.ResumeResponse{},
		&criteriav2.SnapshotRequest{SessionId: "s"},
		&criteriav2.SnapshotResponse{State: []byte("state"), SchemaVersion: 1},
		&criteriav2.RestoreRequest{SessionId: "s", State: []byte("state")},
		&criteriav2.RestoreResponse{},
		&criteriav2.InspectRequest{SessionId: "s"},
		&criteriav2.InspectResponse{CurrentStep: "step-a"},
		&criteriav2.CloseSessionRequest{SessionId: "s"},
		&criteriav2.CloseSessionResponse{},
		&criteriav2.SnapshotVersionMismatch{Have: 1, Want: 2},
		&criteriav2.Heartbeat{StreamName: "execute"},
		&criteriav2.Chunk{Seq: 0, Total: 1, Final: true},
	}

	// PermissionEvent oneof members (CRI-154): every member must unmarshal
	// panic-free, including the tool_call_result variants.
	for _, ev := range permissionEventOneofSeeds() {
		seeds = append(seeds, ev)
	}

	for _, seed := range seeds {
		b, err := proto.Marshal(seed)
		if err != nil {
			f.Fatalf("failed to marshal seed %T: %v", seed, err)
		}
		f.Add(b)
	}

	// Unmarshal targets — one per top-level message type.
	targets := []func([]byte) error{
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.InfoRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.InfoResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.OpenSessionRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.OpenSessionResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.ExecuteRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.ExecuteEvent{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.LogRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.LogEvent{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.PermissionEvent{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.PermissionDecision{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.PauseRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.PauseResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.ResumeRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.ResumeResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.SnapshotRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.SnapshotResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.RestoreRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.RestoreResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.InspectRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.InspectResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.CloseSessionRequest{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.CloseSessionResponse{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.SnapshotVersionMismatch{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.Heartbeat{}) },
		func(b []byte) error { return proto.Unmarshal(b, &criteriav2.Chunk{}) },
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, target := range targets {
			// We only care that no panic occurs; unmarshal errors are expected.
			_ = target(data)
		}
	})
}

// ─── PermissionEvent oneof contract fuzzing (CRI-154) ────────────────────────

// seedArgsDigest is a well-formed 64-hex sha256 value for the PermissionRequest
// seed; the digest value itself is irrelevant to the oneof contract under test
// (parity with the host's canonicalisation is pinned in canonical_test.go).
const seedArgsDigest = "a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4"

// permissionEventOneofSeeds returns one PermissionEvent per oneof member and
// per contract edge the tool_call_result member can hit on the wire: valid
// framing, malformed chunk sequences, an empty request_id, and call_error set
// alongside result data. The list seeds both fuzz harnesses in this file.
func permissionEventOneofSeeds() []*criteriav2.PermissionEvent {
	return []*criteriav2.PermissionEvent{
		{Event: &criteriav2.PermissionEvent_Request{Request: &criteriav2.PermissionRequest{
			RequestId: "req-1", Tool: "create_issue", ArgsDigest: seedArgsDigest,
			ArgsPreview: `{"title":"Found a bug"}`,
		}}},
		{Event: &criteriav2.PermissionEvent_Cancel{Cancel: &criteriav2.PermissionCancel{
			RequestId: "req-1", Reason: "operator denied",
		}}},
		// Well-formed single-fragment result: the exact shape the reassembler
		// must accept.
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-1", Outcome: "success",
			Chunk:       &criteriav2.Chunk{Seq: 0, Total: 1, Final: true},
			OutputsJson: []byte(`{"number":1}`),
		}}},
		// Malformed chunk sequences.
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-1", Outcome: "success",
			Chunk: &criteriav2.Chunk{Seq: 1, Total: 2, Final: true}, // seq gap: does not start at 0
		}}},
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-1", Outcome: "success",
			Chunk: &criteriav2.Chunk{Seq: 0, Total: 2, Final: true}, // declares 2 chunks, delivers 1
		}}},
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-1", Outcome: "success",
			Chunk: &criteriav2.Chunk{Seq: 0, Total: 3, Final: false}, // never final
		}}},
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-1", Outcome: "success",
			Chunk: &criteriav2.Chunk{Seq: 0, Total: 0, Final: true}, // declares zero total
		}}},
		// Empty request_id: the joiner must reject correlation-less fragments.
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "", Outcome: "success",
			Chunk:       &criteriav2.Chunk{Seq: 0, Total: 1, Final: true},
			OutputsJson: []byte(`{}`),
		}}},
		// Unchunked result without chunk metadata: not reassemblable input.
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-2", Outcome: "success", OutputsJson: []byte(`{}`),
		}}},
		// call_error set alongside result data (call_error wins by contract);
		// unchunked and chunked shapes.
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-3", Outcome: "success",
			OutputsJson: []byte(`{"k":"v"}`), CallError: "callee_timeout",
		}}},
		{Event: &criteriav2.PermissionEvent_ToolCallResult{ToolCallResult: &criteriav2.ToolCallResult{
			RequestId: "call-4", CallError: "callee_crash",
			Chunk:       &criteriav2.Chunk{Seq: 0, Total: 1, Final: true},
			OutputsJson: []byte(`{"k":"v"}`),
		}}},
	}
}

// permissionEventMember names which PermissionEvent oneof member is set,
// using the same identifiers as the proto's event members.
func permissionEventMember(ev *criteriav2.PermissionEvent) string {
	switch ev.GetEvent().(type) {
	case *criteriav2.PermissionEvent_Request:
		return "request"
	case *criteriav2.PermissionEvent_Cancel:
		return "cancel"
	case *criteriav2.PermissionEvent_ToolCallResult:
		return "tool_call_result"
	default:
		return ""
	}
}

// toolCallResultVerdict resolves a ToolCallResult to its contract verdict: a
// present call_error wins over any concurrently set outcome/outputs data
// (typed failure), a result without call_error is a success, and no result
// member means no verdict.
func toolCallResultVerdict(res *criteriav2.ToolCallResult) string {
	switch {
	case res == nil:
		return "none"
	case res.GetCallError() != "":
		return "typed_failure"
	default:
		return "success"
	}
}

// wellFormedSingleFragment reports whether res is a complete, correctly framed
// single-chunk ToolCallResult: the exact shape JoinToolCallResultOutputs
// accepts when handed one fragment (non-empty request_id, chunk metadata with
// seq 0, total 1, final set).
func wellFormedSingleFragment(res *criteriav2.ToolCallResult) bool {
	return res.GetRequestId() != "" &&
		res.GetChunk() != nil &&
		res.GetChunk().GetSeq() == 0 &&
		res.GetChunk().GetTotal() == 1 &&
		res.GetChunk().GetFinal()
}

// FuzzPermissionEventOneofContract fuzzes the PermissionEvent oneof with focus
// on the tool_call_result member (CRI-154): malformed chunk sequences, an
// empty request_id, and call_error set alongside result data. It asserts the
// wire contract, not just panic-freedom:
//
//   - the result reassembler accepts exactly the well-formed single-fragment
//     shape, reproduces its outputs byte-for-byte, and never panics on
//     chunk-metadata lies (wrong seq, lying total, missing final flag);
//   - the binary and JSON decoders agree on which oneof member is set and on
//     the call_error-wins verdict when a result is ambiguous.
func FuzzPermissionEventOneofContract(f *testing.F) {
	for _, seed := range permissionEventOneofSeeds() {
		b, err := proto.Marshal(seed)
		if err != nil {
			f.Fatalf("failed to marshal seed %T: %v", seed, err)
		}
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		ev := &criteriav2.PermissionEvent{}
		if err := proto.Unmarshal(data, ev); err != nil {
			return // undecodable bytes are not a contract violation
		}

		// Framing oracle: a decoded ToolCallResult handed to the reassembler
		// alone must be accepted exactly when it is a well-formed single
		// fragment, and the joined outputs must equal the sent ones.
		if res := ev.GetToolCallResult(); res != nil {
			joined, err := criteriav2.JoinToolCallResultOutputs([]*criteriav2.ToolCallResult{res})
			if wellFormedSingleFragment(res) {
				if err != nil {
					t.Fatalf("joiner rejected a well-formed single fragment: %v (fragment: %+v)", err, res)
				}
				if !bytes.Equal(joined, res.GetOutputsJson()) {
					t.Fatalf("joiner altered outputs: sent %q, got %q", res.GetOutputsJson(), joined)
				}
			} else if err == nil {
				t.Fatalf("joiner accepted a malformed fragment: %+v", res)
			}
		}

		// Decoder-agreement oracle: the JSON surface must decode the same
		// oneof member and the same call_error-wins verdict as the binary
		// surface. protojson.Marshal rejects invalid UTF-8 strings; such
		// bytes are a transport concern, not a contract violation.
		jb, err := protojson.Marshal(ev)
		if err != nil {
			return
		}
		evJSON := &criteriav2.PermissionEvent{}
		if err := protojson.Unmarshal(jb, evJSON); err != nil {
			t.Fatalf("protojson.Unmarshal rejected protojson.Marshal output: %v (json: %s)", err, jb)
		}
		if got, want := permissionEventMember(evJSON), permissionEventMember(ev); got != want {
			t.Fatalf("decoders disagree on oneof member: binary=%q json=%q", want, got)
		}
		if got, want := toolCallResultVerdict(evJSON.GetToolCallResult()), toolCallResultVerdict(ev.GetToolCallResult()); got != want {
			t.Fatalf("decoders disagree on call_error-wins verdict: binary=%q json=%q", want, got)
		}
	})
}

// TestPermissionEvent_CallErrorAndResultBothSet_DecodersAgree pins the
// call_error-wins resolution deterministically (the fuzz target above asserts
// the same invariant over arbitrary decodable bytes): a ToolCallResult
// carrying call_error together with success-shaped result data is a typed
// failure under both wire decoders, and the conflicting result data is still
// preserved on the wire for audit.
func TestPermissionEvent_CallErrorAndResultBothSet_DecodersAgree(t *testing.T) {
	ambiguous := &criteriav2.PermissionEvent{Event: &criteriav2.PermissionEvent_ToolCallResult{
		ToolCallResult: &criteriav2.ToolCallResult{
			RequestId:   "call-1",
			Outcome:     "success",
			OutputsJson: []byte(`{"issue_url":"https://github.com/octocat/hello-world/issues/1"}`),
			CallError:   "callee_timeout",
		},
	}}

	viaBinary := roundTrip(t, ambiguous)

	jb, err := protojson.Marshal(ambiguous)
	require.NoError(t, err)
	viaJSON := &criteriav2.PermissionEvent{}
	require.NoError(t, protojson.Unmarshal(jb, viaJSON))

	for name, decoded := range map[string]*criteriav2.PermissionEvent{"binary": viaBinary, "json": viaJSON} {
		res := decoded.GetToolCallResult()
		require.NotNilf(t, res, "%s decoder dropped the tool_call_result member", name)
		assert.Equalf(t, "tool_call_result", permissionEventMember(decoded), "%s oneof member", name)
		assert.Equalf(t, "callee_timeout", res.GetCallError(), "%s decoder must preserve call_error", name)
		assert.Equalf(t, "typed_failure", toolCallResultVerdict(res),
			"%s decoder must reach the call_error-wins verdict", name)
		// The conflicting result data is still on the wire: the contract —
		// not lossy decoding — is what makes call_error authoritative.
		assert.Equalf(t, "success", res.GetOutcome(), "%s decoder must preserve the conflicting outcome", name)
		assert.JSONEqf(t, `{"issue_url":"https://github.com/octocat/hello-world/issues/1"}`,
			string(res.GetOutputsJson()), "%s decoder must preserve the conflicting outputs_json", name)
	}

	assert.Equal(t,
		toolCallResultVerdict(viaBinary.GetToolCallResult()),
		toolCallResultVerdict(viaJSON.GetToolCallResult()),
		"binary and JSON decoders must agree that call_error wins")
}
