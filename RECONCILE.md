# Reconciliation notes (proto extraction — switchover TODO)

This repo was seeded from the **live SDK copy** (`sdk/pb/criteria/v2`, 27 in-tree
importers). The criteria host had a **second, diverged copy** at
`proto/criteria/v2`. Before the host switches to depend on this module, reconcile:

- **`chunking.go`** — the host copy exports `SendChunks` / `AssembleChunks` that
  the SDK copy (used here) does not. Take the **union** of both before deleting
  the in-tree copies, or the host build will break on the missing symbols.
- **`adapter_grpc.pb.go`** — differed between host and SDK copies (regenerate
  from `proto/criteria/v2/*.proto` with a single buf config to get one canonical
  output).
- Re-run the host's `contract_test.go` / `fuzz_test.go` / `proto_test.go`
  against this module's bindings during the switchover.

Switchover steps (separate effort):
1. Reconcile the helpers above into this repo.
2. `go get github.com/brokenbots/criteria-adapter-proto@vX` in the criteria host
   and the Go SDK; repoint imports; delete in-tree `proto/criteria/v2` + `sdk/pb`.
3. Generate + publish TS (`@criteria/adapter-proto`) and Python
   (`criteria-adapter-proto`) bindings for those SDKs.
