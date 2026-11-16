package criteriav2_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	criteriav2 "github.com/brokenbots/criteria-adapter-proto/criteria/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ExecuteRequest.timeout_ms (KB-282: KB-212 leg 1, wire carrier) ──────────
//
// timeout_ms (field 8, int64) forwards a workflow's declared step timeout to
// adapters so long steps are not killed by an adapter's own internal default
// (e.g. the shell adapter's 5m default). These tests pin the additive
// contract: the field rides the wire as exactly field 8; unset/0 = adapter's
// own default (today's behavior byte-for-byte); old hosts and old adapters
// degrade cleanly in both directions.

// TestExecuteRequest_TimeoutMs_RoundTrip asserts the carrier contract: a
// populated declared timeout survives the proto wire unchanged, and an unset
// timeout decodes the same as an absent one (0) — the "0/absent = unset"
// rule that keeps the pre-v0.8.0 behavior byte-for-byte.
func TestExecuteRequest_TimeoutMs_RoundTrip(t *testing.T) {
	msg := &criteriav2.ExecuteRequest{
		SessionId: "sess-1",
		StepName:  "pair_programming_loop:ci_gate",
		Input:     map[string]string{"prompt": "run the gate"},
		// 60m declared step timeout (KB-95/KB-96 evidence shape).
		TimeoutMs: int64((60 * time.Minute) / time.Millisecond),
	}
	got := roundTrip(t, msg)
	require.True(t, proto.Equal(msg, got))
	assert.EqualValues(t, 60*time.Minute/time.Millisecond, got.GetTimeoutMs(),
		"the declared step timeout must survive the wire unchanged")

	absent := roundTrip(t, &criteriav2.ExecuteRequest{SessionId: "sess-2", StepName: "step-a"})
	assert.Zero(t, absent.GetTimeoutMs(),
		"an unset timeout must stay unset (0) — adapters apply their own default")
}

// TestExecuteRequest_TimeoutMs_UnsetOmitsJSON asserts the JSON surface: an
// unset timeout is omitted from protojson output entirely, so hosts/SDKs
// speaking JSON see byte-identical payloads to pre-v0.8.0.
func TestExecuteRequest_TimeoutMs_UnsetOmitsJSON(t *testing.T) {
	legacy := &criteriav2.ExecuteRequest{SessionId: "sess-1", StepName: "step-a"}
	legacyJSON, err := protojson.Marshal(legacy)
	require.NoError(t, err)
	assert.NotContains(t, string(legacyJSON), "timeoutMs",
		"an unset timeout must not appear in JSON payloads")

	carrier := &criteriav2.ExecuteRequest{
		SessionId: "sess-1",
		StepName:  "step-a",
		TimeoutMs: 1,
	}
	carrierJSON, err := protojson.Marshal(carrier)
	require.NoError(t, err)
	assert.Contains(t, string(carrierJSON), "timeoutMs",
		"a set timeout must appear in JSON payloads so adapters see the declaration")
	// protojson renders int64 scalars as JSON strings per its int64 spec.
	var decoded criteriav2.ExecuteRequest
	require.NoError(t, protojson.Unmarshal(carrierJSON, &decoded))
	assert.EqualValues(t, 1, decoded.GetTimeoutMs(),
		"a JSON-carried timeout must decode back to the declared value")
}

// TestExecuteRequest_TimeoutMs_WireFieldNumbers pins the wire contract:
// timeout_ms is field 8 — the next free number after rejection=7 — and a
// plain (non-presence) scalar so 0 and absent stay indistinguishable. The
// neighboring fields must not collide: an earlier draft of the card placed
// timeout_ms on 6, which outcome_contracts already owns.
func TestExecuteRequest_TimeoutMs_WireFieldNumbers(t *testing.T) {
	fd := criteriav2.File_criteria_v2_adapter_proto
	require.NotNil(t, fd, "adapter proto descriptor must be registered")

	msgDesc := fd.Messages().ByName("ExecuteRequest")
	require.NotNil(t, msgDesc)

	f := msgDesc.Fields().ByName("timeout_ms")
	require.NotNil(t, f, "ExecuteRequest.timeout_ms must exist on the wire")
	assert.Equal(t, protoreflect.FieldNumber(8), f.Number(),
		"timeout_ms must be field 8 — the next free number after rejection=7")
	assert.Equal(t, protoreflect.Int64Kind, f.Kind(), "timeout_ms must be an int64 carrier")
	assert.False(t, f.HasPresence(),
		"timeout_ms must be a plain scalar: 0 (absent) = unset, not a presence-tracked optional")

	contracts := msgDesc.Fields().ByName("outcome_contracts")
	require.NotNil(t, contracts)
	assert.Equal(t, protoreflect.FieldNumber(6), contracts.Number(),
		"outcome_contracts must stay field 6 (no renumber/retag)")
	rejection := msgDesc.Fields().ByName("rejection")
	require.NotNil(t, rejection)
	assert.Equal(t, protoreflect.FieldNumber(7), rejection.Number(),
		"rejection must stay field 7 (no renumber/retag)")
}

// TestExecuteRequest_TimeoutMs_WireEncoding asserts the on-the-wire shape of
// a populated timeout: exactly one field-8 varint carrying the declared
// value, and none at all when unset — the byte-for-byte gate on the legacy
// payload.
func TestExecuteRequest_TimeoutMs_WireEncoding(t *testing.T) {
	unset, err := proto.Marshal(&criteriav2.ExecuteRequest{
		SessionId:       "sess-1",
		StepName:        "step-a",
		Input:           map[string]string{"prompt": "hello"},
		AllowedOutcomes: []string{"done"},
	})
	require.NoError(t, err)
	recs, err := findWireRecords(unset, 8)
	require.NoError(t, err)
	assert.Empty(t, recs,
		"a pre-v0.8.0-shaped request must carry zero field-8 records on the wire")

	set, err := proto.Marshal(&criteriav2.ExecuteRequest{
		SessionId: "sess-1",
		StepName:  "step-a",
		TimeoutMs: 3600000,
	})
	require.NoError(t, err)
	recs, err = findWireRecords(set, 8)
	require.NoError(t, err)
	require.Len(t, recs, 1, "a set timeout must emit exactly one field-8 record")
	require.Equal(t, protowire.VarintType, recs[0].typ)
	assert.EqualValues(t, 3600000, recs[0].varint)
}

// TestExecuteRequest_TimeoutMs_OldHostBytesDegrade asserts the old-host row
// of the additive matrix: bytes a pre-v0.8.0 host produces (no field 8 at
// all) decode on a v0.8.0 adapter with timeout_ms left unset — the adapter
// then applies its own default, today's behavior byte-for-byte.
func TestExecuteRequest_TimeoutMs_OldHostBytesDegrade(t *testing.T) {
	legacyPayload := &criteriav2.ExecuteRequest{
		SessionId:       "sess-1",
		StepName:        "step-a",
		Input:           map[string]string{"prompt": "hello"},
		AllowedOutcomes: []string{"done"},
	}
	legacyBytes, err := proto.Marshal(legacyPayload)
	require.NoError(t, err)

	recs, err := findWireRecords(legacyBytes, 8)
	require.NoError(t, err)
	require.Empty(t, recs, "old-host bytes must not carry field 8")

	decoded := &criteriav2.ExecuteRequest{}
	require.NoError(t, proto.Unmarshal(legacyBytes, decoded))
	assert.Zero(t, decoded.GetTimeoutMs(),
		"old-host bytes must decode to an unset timeout on a v0.8.0 adapter")
	require.True(t, proto.Equal(legacyPayload, decoded),
		"legacy payload must decode identically (no semantic drift)")
}

// TestExecuteRequest_TimeoutMs_OldAdapterIgnoresUnknownField asserts the
// new-host row: a v0.8.0 host's bytes (field 8 populated) decode cleanly on
// a pre-v0.8.0 adapter. The legacy adapter decode is simulated with the real
// file descriptor minus the timeout_ms field, driven through dynamicpb —
// the same unknown-field tolerance generated bindings rely on.
func TestExecuteRequest_TimeoutMs_OldAdapterIgnoresUnknownField(t *testing.T) {
	legacyFD := protodesc.ToFileDescriptorProto(criteriav2.File_criteria_v2_adapter_proto)
	stripped := false
	for _, md := range legacyFD.MessageType {
		if md.GetName() != "ExecuteRequest" {
			continue
		}
		kept := make([]*descriptorpb.FieldDescriptorProto, 0, len(md.Field))
		for _, f := range md.Field {
			if f.GetName() == "timeout_ms" {
				stripped = true
				continue
			}
			kept = append(kept, f)
		}
		md.Field = kept
	}
	require.True(t, stripped, "descriptor surgery must have removed timeout_ms")

	legacyFile, err := protodesc.NewFile(legacyFD, protoregistry.GlobalFiles)
	require.NoError(t, err, "legacy (pre-v0.8.0) descriptor must compile")
	reqDesc := legacyFile.Messages().ByName("ExecuteRequest")
	require.NotNil(t, reqDesc)
	require.Nil(t, reqDesc.Fields().ByName("timeout_ms"),
		"legacy descriptor must not know timeout_ms")

	newHostBytes, err := proto.Marshal(&criteriav2.ExecuteRequest{
		SessionId:       "sess-1",
		StepName:        "step-a",
		AllowedOutcomes: []string{"done"},
		TimeoutMs:       3600000,
	})
	require.NoError(t, err)

	oldAdapterMsg := dynamicpb.NewMessage(reqDesc)
	require.NoError(t, proto.Unmarshal(newHostBytes, oldAdapterMsg),
		"a pre-v0.8.0 adapter decode must tolerate the unknown field 8")
	assert.Equal(t, "sess-1",
		oldAdapterMsg.Get(reqDesc.Fields().ByName("session_id")).Interface())
	assert.Equal(t, "step-a",
		oldAdapterMsg.Get(reqDesc.Fields().ByName("step_name")).Interface())
}

// wireRecord is one decoded top-level field record of a message's wire
// encoding.
type wireRecord struct {
	num    protowire.Number
	typ    protowire.Type
	varint uint64
}

// findWireRecords walks the top-level records of a message's wire encoding
// and returns every record matching fieldNum. Map entries and nested
// messages (fields 1/2 of the inner encoding) exist only as their own
// record numbers at the top level here, so the scan is exact for the
// ExecuteRequest payloads above.
func findWireRecords(data []byte, fieldNum protowire.Number) ([]wireRecord, error) {
	var found []wireRecord
	for len(data) > 0 {
		num, typ, tagLen := protowire.ConsumeTag(data)
		if tagLen < 0 {
			return nil, protowire.ParseError(tagLen)
		}
		data = data[tagLen:]
		valLen := protowire.ConsumeFieldValue(num, typ, data)
		if valLen < 0 {
			return nil, protowire.ParseError(valLen)
		}
		if num == fieldNum {
			record := wireRecord{num: num, typ: typ}
			if typ == protowire.VarintType {
				v, n := protowire.ConsumeVarint(data[:valLen])
				if n < 0 {
					return nil, protowire.ParseError(n)
				}
				record.varint = v
			}
			found = append(found, record)
		}
		data = data[valLen:]
	}
	return found, nil
}
