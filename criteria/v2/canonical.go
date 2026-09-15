package criteriav2

// canonical.go — the canonical-JSON args-digest helper shared by the host and
// every Go SDK/adapter. This is the single algorithm behind
// PermissionRequest.args_digest and the `args_digest` key of the
// `permission.request` AdapterEvent payload (kind "adapter_tool", CRI-152):
// sha256 over the canonical JSON encoding of the args, so audit trails and
// correlation compare equal across serialisation-order differences and across
// consumers (host, criteria-go-adapter-sdk, adapters).
//
// The algorithm is the one the Criteria host implements (the host repo's
// internal/adapter/audit/canonical.go): a deterministic *subset* of RFC 8785
// (JCS — JSON Canonicalization Scheme) built on encoding/json. The subset is
// deliberately pinned, deviations included, so digests stay byte-identical
// across all consumers. Do not "fix" any of the deltas below without a
// coordinated wire change — they are part of the digest contract:
//
//   - object keys are sorted with Go's byte-wise string ordering
//     (sort.Strings), not RFC 8785's UTF-16 code-unit ordering (the two
//     differ only for characters above the BMP versus U+E000..U+FFFF);
//   - strings are encoded by encoding/json, which HTML-escapes <, >, and &
//     (RFC 8785 does not) and escapes U+2028/U+2029 (RFC 8785 does not);
//   - numbers are encoded by encoding/json (Go float formatting with its
//     two-digit-exponent cleanup), not the full RFC 8785 ES6 algorithm — in
//     particular -0 serialises as "-0" where RFC 8785 requires "0";
//   - booleans and null are literal; output has no whitespace and no
//     trailing newline.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalJSON returns the canonical JSON encoding of v, as consumed by
// ArgsDigest.
//
// v must be JSON-round-trippable: maps, slices, numbers, strings, booleans,
// and nil are all accepted. Object keys are sorted at every nesting level.
// The output has no trailing newline and no whitespace between tokens.
//
// The encoding is intentionally the host's canonical-JSON subset of RFC 8785
// (see the package-level comment on this file) rather than full RFC 8785:
// for args_digest the requirement is determinism and byte-parity between the
// host and every SDK, and this helper is the shared implementation of it.
func CanonicalJSON(v any) ([]byte, error) {
	// Round-trip through encoding/json to normalise Go-native types (e.g. int,
	// float64, struct) into a generic map/slice/scalar tree that we can
	// canonicalise recursively.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical json: marshal: %w", err)
	}

	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("canonical json: unmarshal: %w", err)
	}

	var buf bytes.Buffer
	if err := encodeCanonical(&buf, generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ArgsDigest returns the lowercase hex-encoded SHA-256 digest of the
// canonical JSON representation of v. This is the value stored in
// PermissionRequest.args_digest and in the `args_digest` key of the
// `permission.request` AdapterEvent payload for adapter tool calls; the host
// computes exactly the same bytes, so SDKs and adapters can assert parity.
func ArgsDigest(v any) (string, error) {
	b, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// encodeCanonical recursively writes the canonical JSON of node into buf.
func encodeCanonical(buf *bytes.Buffer, node any) error {
	if node == nil {
		buf.WriteString("null")
		return nil
	}
	switch v := node.(type) {
	case bool:
		encodeBool(buf, v)
	case float64:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("canonical json: encode float64: %w", err)
		}
		buf.Write(b)
	case string:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("canonical json: encode string: %w", err)
		}
		buf.Write(b)
	case []any:
		return encodeArray(buf, v)
	case map[string]any:
		return encodeObject(buf, v)
	default:
		return fmt.Errorf("canonical json: unsupported type %T", node)
	}
	return nil
}

// encodeBool writes "true" or "false" into buf.
func encodeBool(buf *bytes.Buffer, v bool) {
	if v {
		buf.WriteString("true")
	} else {
		buf.WriteString("false")
	}
}

// encodeArray writes the canonical JSON of a JSON array into buf.
func encodeArray(buf *bytes.Buffer, v []any) error {
	buf.WriteByte('[')
	for i, elem := range v {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := encodeCanonical(buf, elem); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// encodeObject writes the canonical JSON of a JSON object into buf, sorting
// keys with Go's byte-wise string ordering so the output is deterministic.
func encodeObject(buf *bytes.Buffer, v map[string]any) error {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return fmt.Errorf("canonical json: encode key: %w", err)
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		if err := encodeCanonical(buf, v[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}
