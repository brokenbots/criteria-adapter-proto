package criteriav2

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

// TestSmoke_MarshalRoundTrip confirms the generated descriptors initialise and
// a message round-trips — i.e. the extracted bindings work standalone.
func TestSmoke_MarshalRoundTrip(t *testing.T) {
	in := &InfoResponse{Name: "demo", Version: "0.5.0"}
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := &InfoResponse{}
	if err := proto.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.GetName() != "demo" || out.GetVersion() != "0.5.0" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
