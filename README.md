# Shiroclient Golang SDK

This repository contains a Go-based JSON-RPC client for interacting with the **shiroclient gateway**, a component of the Luther Platform that mediates access to _common operations scripts (phylum)_ running on a distributed system (fabric).

📦 GoDoc: [https://pkg.go.dev/github.com/luthersystems/shiroclient-sdk-go/shiroclient](https://pkg.go.dev/github.com/luthersystems/shiroclient-sdk-go/shiroclient)

Argument configuration is identical to the shiroclient Java [implementation](https://github.com/luthersystems/shiroclient-sdk-java).

---

## Features

- Execute requests and receive responses from logic written in [ELPS](https://github.com/luthersystems/elps) and run by the substrate runtime
- Schedule and process batch jobs
- Encode and decode private data with GDPR-style purging and export
- Track and update multiple phylum versions
- Mock backend for local and CI testing

---

## 🏗️ Transaction Flow

Shiroclient manages transactions to the distributed system (fabric) using peer discovery and ordering services.

1. **Peer Discovery**: Queries the network for peer and org membership using the configured MSP.
2. **Simulation & Endorsement**: Sends the transaction to suitable peers for simulation.
3. **Commitment**: If required, registers for block commit events and submits the transaction to the orderer.

Write transactions require a peer from the same org as the gateway. Ensure availability with at least two peers.

More details: [Fabric Transaction Flow](https://hyperledger-fabric.readthedocs.io/en/release-2.2/txflow.html)

---

## 🧪 Mock Mode

For testing, the SDK includes a mock implementation that simulates a fabric peer and ledger in memory.

```go
client, err := shiroclient.NewMock(nil)
err = client.Init(ctx, shiroclient.EncodePhylumBytes(testPhylum))
```

You can restore mock clients from snapshots and bootstrap them with config.

### Tracing in Mock Mode

The substrate plugin supports OTLP trace export. Set the `SUBSTRATE_OTLP_ENDPOINT` environment variable to enable end-to-end distributed tracing when running with the mock client:

```bash
SUBSTRATE_OTLP_ENDPOINT=http://localhost:4318 go test ./...
```

Because the SDK launches the plugin as a child process that inherits the parent's environment, no additional configuration is needed. The SDK's existing trace context propagation via transient data connects your application-layer spans to substrate-layer spans, giving you a unified view across the entire call chain.

---

## ⚛️ Atomic Multi-Call (`CallBatch`)

`shiroclient.CallBatch` runs several phylum methods as **one** transaction,
all or nothing. Requests run in order and each sees the writes of the ones
before it.

```go
resp, err := shiroclient.CallBatch(ctx, client, []shiroclient.CallBatchRequest{
  {Method: "deposit", Params: []interface{}{aliceDeposit}, ID: "alice",
    Transient: map[string][]byte{"secret": aliceSecret}},
  {Method: "deposit", Params: []interface{}{bobDeposit}, ID: "bob",
    Transient: map[string][]byte{"secret": bobSecret}},
}, shiroclient.WithTransientData("key", sharedKey)) // shared by both
var batchErr *shiroclient.CallBatchError
switch {
case errors.As(err, &batchErr):
  // Nothing was committed. batchErr.Index / batchErr.ID name the failed
  // request and batchErr.Err is its error; resp.Responses holds every
  // request's response (the others carry a "batch aborted" error).
case shiroclient.IsTimeoutError(err):
  // The batch MAY have committed. Reconcile before running it again.
  // With a gateway that includes substrate#515 the error also matches
  // shiroclient.ErrOutcomeUnknown and carries the tx ID to look for.
  txID, ok := shiroclient.OutcomeUnknownTxID(err) // ok is false on older gateways
  reconcile(txID, ok)
case err != nil:
  // e.g. shiroclient.ErrCallBatchNotSupported: nothing ran.
default:
  // resp.Committed, resp.TxID: one transaction for every request.
}
```

- **All or nothing.** If any request fails, nothing is committed, and
  `CallBatch` returns the `CallBatchResponse` together with a `*CallBatchError`.
  A batch that only reads is not committed and has no `TxID`.
- **Timeouts are not failures.** A timeout (`IsTimeoutError`) means the
  batch may have committed. Never run it again, as a new batch or as
  separate calls, without reconciling against the ledger first. A gateway
  that includes
  [luthersystems/substrate#515](https://github.com/luthersystems/substrate/pull/515)
  also reports `ErrOutcomeUnknown` with the transaction ID
  (`OutcomeUnknownTxID`). An older gateway sends only the timeout.
- **Shared options.** Configs apply to the whole batch, as for `Call`:
  transient data from `WithTransientData` is shared by every request, and
  every request runs against the same phylum version.
- **Per-request transient data.** `CallBatchRequest.Transient` is seen only
  by its own request: a transient read there finds the request's key first,
  then the shared one, so two deposits can each carry their own `"secret"`.
  This isolates the requests from each other. It does **not** hide the data
  from endorsing peers: Fabric sends all of it to every endorser, as for
  `Call`. Keys starting with `$batch/` are reserved and rejected (also in
  `Call`), as is an empty per-request key; nothing is sent. Per-request
  transient data needs a substrate release with luthersystems/substrate#521.
  Against an older chaincode the requests do not see their own keys, the
  phylum usually fails, and the batch commits nothing.
- **Requirements.** The gateway must include
  [luthersystems/substrate#521](https://github.com/luthersystems/substrate/pull/521).
  An older gateway answers "method not found", and `CallBatch` returns an
  error matching `shiroclient.ErrCallBatchNotSupported` without running
  anything. The mock client does not support batches yet (the substrate
  plugin has no batch method) and returns the same error. `CallBatch` never
  falls back to separate `Call`s, which would not be atomic.
- **Compatibility.** `CallBatch` is a package function over the optional
  `shiroclient.CallBatcher` interface, so the `ShiroClient` interface is
  unchanged.

---

## 🔁 Batch Driver

The `batch` package allows polling for time-based requests from ELPS _common operations scripts_.

```go
driver := batch.NewDriver(client)
driver.Register(ctx, "my_batch", 1*time.Minute, func(batchID, reqID string, msg json.RawMessage) (json.RawMessage, error) {
  return processMessage(msg)
})
```

This enables workflows triggered by timers or async scheduling logic in your _common operations script_.

---

## 🔐 Private Data Utilities

The `private` package supports encoding, decoding, export, and purge of sensitive data.

```go
enc, err := private.Encode(ctx, client, message, transforms)
err = private.Decode(ctx, client, enc, &output)
```

You can also use `WrapCall` to automatically encode/decode data and inject encryption metadata.

---

## 🧬 Phylum Version Management

The `update` package can install, enable, disable, and list phylum versions on the distributed system (fabric).

```go
err := update.Install(ctx, client, "v1.2.3", myPhylumBytes)
err = update.Enable(ctx, client, "v1.2.3")
```

Use this to upgrade logic in production networks.

---

## 🛠 Building the Plugin

To obtain the substrate plugin (required for `NewMock()`):

```bash
make plugin
```

This downloads the platform-specific plugin into `build/`.

---

## 🔍 Health Checks

```go
health, err := shiroclient.RemoteHealthCheck(ctx, client, []string{"phylum", "fabric_peer"})
```

Use this to validate connectivity across system components.
