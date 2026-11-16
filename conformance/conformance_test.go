package conformance_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/brokenbots/criteria-adapter-proto/conformance"
	criteriav2 "github.com/brokenbots/criteria-adapter-proto/criteria/v2"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// pinnedVectors is the complete expected vector inventory. A vector added to
// or removed from conformance/vectors must update this list, keeping the
// README inventory table and the shared fixtures in lockstep.
var pinnedVectors = []string{
	"01_contract_roundtrip_valid.json",
	"02_contract_payload_invalid.json",
	"03_require_comment_missing.json",
	"04_fallback_fires.json",
	"05_no_contracts_legacy.json",
	"06_comment_end_to_end.json",
	"07_rejection_repair_context.json",
}

type vectorPhase struct {
	Request json.RawMessage   `json:"request"`
	Results []json.RawMessage `json:"results"`
	Expect  struct {
		Accepted  *bool           `json:"accepted"`
		Forwarded json.RawMessage `json:"forwarded"`
		Issues    []string        `json:"issues"`
		Rejection json.RawMessage `json:"rejection"`
	} `json:"expect"`
}

type vectorDoc struct {
	Name   string        `json:"name"`
	Phases []vectorPhase `json:"phases"`
}

func loadVectors(t *testing.T) map[string]vectorDoc {
	t.Helper()
	names, err := conformance.VectorNames()
	require.NoError(t, err)
	require.ElementsMatch(t, pinnedVectors, names)

	docs := make(map[string]vectorDoc, len(names))
	for _, name := range names {
		raw, err := conformance.ReadVector(name)
		require.NoError(t, err)
		var doc vectorDoc
		require.NoError(t, json.Unmarshal(raw, &doc), "vector %s", name)
		require.NotEmpty(t, doc.Name, "vector %s must carry a human label", name)
		require.NotEmpty(t, doc.Phases, "vector %s", name)
		docs[name] = doc
	}
	return docs
}

func unmarshalProto(t *testing.T, raw json.RawMessage, m proto.Message) {
	t.Helper()
	if len(raw) == 0 {
		return
	}
	require.NoError(t, protojson.Unmarshal(raw, m), "proto3 JSON parse of %.60s", raw)
}

// TestVectorsParseAsProto3JSON pins that every vector document parses into the
// generated v0.7.0 bindings via plain proto3 canonical JSON — the
// compatibility the three SDK suites rely on when they consume the same bytes.
func TestVectorsParseAsProto3JSON(t *testing.T) {
	for name, doc := range loadVectors(t) {
		t.Run(name, func(t *testing.T) {
			for pi, phase := range doc.Phases {
				unmarshalProto(t, phase.Request, &criteriav2.ExecuteRequest{})
				for _, raw := range phase.Results {
					unmarshalProto(t, raw, &criteriav2.ExecuteResult{})
				}
				if phase.Expect.Forwarded != nil {
					unmarshalProto(t, phase.Expect.Forwarded, &criteriav2.ExecuteResult{})
				}
				if phase.Expect.Rejection != nil {
					unmarshalProto(t, phase.Expect.Rejection, &criteriav2.ExecutionRejection{})
				}
				require.NotNil(t, phase.Expect.Accepted, "phase %d expect.accepted", pi)
			}
		})
	}
}

// TestVectorRejectionLinkage pins the rejection-repair loop contract between
// phases: the next phase's request.rejection is proto-equal to the previous
// phase's expect.rejection, and a rejection's issues text is exactly its
// ordered issue list joined with "\n".
func TestVectorRejectionLinkage(t *testing.T) {
	for name, doc := range loadVectors(t) {
		t.Run(name, func(t *testing.T) {
			var pending *criteriav2.ExecutionRejection
			for pi, phase := range doc.Phases {
				var rejection *criteriav2.ExecutionRejection
				if phase.Expect.Rejection != nil {
					rejection = &criteriav2.ExecutionRejection{}
					unmarshalProto(t, phase.Expect.Rejection, rejection)
					require.Equal(t, strings.Join(phase.Expect.Issues, "\n"), rejection.GetIssues(),
						"phase %d rejection issues must be its ordered issue list joined with \\n", pi)
				}
				if pending != nil {
					req := &criteriav2.ExecuteRequest{}
					unmarshalProto(t, phase.Request, req)
					require.True(t, proto.Equal(pending, req.GetRejection()),
						"phase %d request.rejection must carry phase %d expect.rejection verbatim", pi, pi-1)
				}
				pending = rejection
			}
		})
	}
}
