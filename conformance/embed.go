// Package conformance embeds the outcome-contract conformance vectors
// (conformance/README.md) so the Go SDK, the engine suite, and any other Go
// consumer validate against byte-identical fixtures at the pinned release.
//
// The non-Go SDKs (criteria-python-adapter-sdk,
// criteria-typescript-adapter-sdk) read the same files from the same tag;
// this package exists so the Go side never drifts from the committed bytes.
package conformance

import (
	"embed"
	"io/fs"
	"sort"
)

//go:embed vectors/*.json
var vectorFS embed.FS

// VectorNamespace is the directory the vector files live in, relative to the
// package root.
const VectorNamespace = "vectors"

// VectorFS exposes the embedded vector files rooted at the package root so
// callers can fs.ReadDir(VectorFS, "vectors") or fs.ReadFile(VectorFS,
// vectors/<file>.json) without importing this package's helpers.
var VectorFS fs.FS = vectorFS

// VectorNames returns the sorted base names of every vector file ("01_….json",
// …). Test suites iterate these so a vector added or removed here forces a
// conscious test update rather than silent skew.
func VectorNames() ([]string, error) {
	entries, err := fs.ReadDir(vectorFS, VectorNamespace)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// ReadVector returns the exact committed bytes of one vector file. The name
// is the base name as returned by VectorNames ("01_contract_roundtrip_valid.json").
func ReadVector(name string) ([]byte, error) {
	return fs.ReadFile(vectorFS, VectorNamespace+"/"+name)
}
