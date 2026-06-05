# criteria-adapter-proto

The **adapter wire contract** for [Criteria](https://github.com/brokenbots/criteria) —
protocol **v2** `.proto` sources and generated bindings. This is the single
source of truth for the host↔adapter wire; the host and every SDK consume it as
a versioned dependency so no single project can drift the contract.

## Layout

```
proto/criteria/v2/   # .proto sources (adapter.proto, options.proto)
criteria/v2/         # generated Go bindings + helpers (package criteriav2)
```

## Go

```
go get github.com/brokenbots/criteria-adapter-proto@latest
```

```go
import criteriav2 "github.com/brokenbots/criteria-adapter-proto/criteria/v2"
```

## Status

Extraction in progress. Go bindings are live and tested here; the criteria host
and Go SDK switchover (and TS/Python package publishing) are tracked in
[RECONCILE.md](RECONCILE.md). Versioned to track the criteria release line.
