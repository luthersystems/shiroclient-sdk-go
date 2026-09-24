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

## ⚛️ Atomic Multi-Call (`CallBatch` and `QueryBatch`)

`shiroclient.CallBatch` runs several phylum methods as **one** transaction,
all or nothing. Requests run in order and each sees the writes of the ones
before it. `shiroclient.QueryBatch` takes the same requests and runs them
the same way, but never commits.

|                         | `CallBatch`                                      | `QueryBatch`                                       |
|-------------------------|--------------------------------------------------|----------------------------------------------------|
| Purpose                 | Write: commit several requests together          | Read or simulate several requests together         |
| Every request must succeed | Yes: the first failure stops the batch        | Yes: the first failure stops the batch             |
| Commits                 | The writes, once, as one transaction             | Never; nothing is ordered                          |
| Later requests see earlier writes | Yes                                    | Yes, the simulated writes, then discarded          |
| Write-then-discard requests (`private_decode`) | Refused: they fail the batch with `CodeForcedNoCommit` | Allowed: they belong here  |
| Result on failure       | `CallBatchResponse` + `*CallBatchError`          | `CallBatchResponse` + `*CallBatchError`            |
| `Committed` / `TxID`    | Set when the batch wrote state                   | Always `false` / empty                             |

Build each request like a `Call`, including its own transient data, and hand
the list to the batch. Configs passed to the batch itself apply to the whole
transaction.

```go
aliceMXF, err := private.WithTransientMXF(&private.EncodeRequest{Message: alice, Transforms: transforms})
if err != nil { return err }
bobMXF, err := private.WithTransientMXF(&private.EncodeRequest{Message: bob, Transforms: transforms})
if err != nil { return err }
seed, err := private.WithSeed() // one CSPRNG seed per transaction
if err != nil { return err }

resp, err := shiroclient.CallBatch(ctx, client, []shiroclient.CallBatchRequest{
  {Method: "create_profile", ID: "alice", Configs: aliceMXF},
  {Method: "create_profile", ID: "bob", Configs: bobMXF},
  {Method: "deposit", Params: []interface{}{aliceDeposit}, ID: "deposit",
    Configs: []shiroclient.Config{shiroclient.WithTransientData("secret", aliceSecret)}},
}, seed) // batch level: the seed, and options such as WithMSPFilter
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
  A batch that only reads is not committed and has no `TxID`. `QueryBatch`
  fails the same way, since results after a failed request rest on a
  simulation that went wrong.
- **Timeouts are not failures.** A timeout (`IsTimeoutError`) means the
  batch may have committed. Never run it again, as a new batch or as
  separate calls, without reconciling against the ledger first. A gateway
  that includes
  [luthersystems/substrate#515](https://github.com/luthersystems/substrate/pull/515)
  also reports `ErrOutcomeUnknown` with the transaction ID
  (`OutcomeUnknownTxID`). An older gateway sends only the timeout.
- **Per-request transient data.** Put a request's transient data in its
  `CallBatchRequest.Configs`: `WithTransientData`, `WithTransientDataMap`,
  or helpers built on them such as `private.WithTransientMXF`. The batch
  sends each request's data with that request only, so two requests can
  each carry their own `"mxf"` or `"secret"`. This gives **correct
  per-request routing, not isolation from phylum code**: the phylum is
  trusted, and Fabric sends the whole transient map to every endorser, as
  for `Call`.
- **Batch-level configs apply to the whole transaction.** Pass them to
  `CallBatch` / `QueryBatch` itself. Their transient data may hold only the
  transaction-wide keys `csprng_seed_private` (`private.WithSeed`),
  `timestamp_override`, `traceparent` and `tracestate`. Any other key is
  refused, with an error pointing to the request's `Configs`, and nothing is
  sent.
- **The seed is transaction-wide.** `private.WithTransientMXF` bundles a
  seed with the `mxf` data. In a request's `Configs` that seed yields to
  the batch's, so pass `private.WithSeed()` to the batch; without it the
  batch is refused.
- **What a request's `Configs` may set.** Allowed: transient data
  (`WithTransientData`, `WithTransientDataMap`, `private.WithTransientMXF`,
  `private.WithSeed` as above) and no-op configs. Refused before anything is
  sent, naming the option: `WithParams` and `WithID` (use the request's
  `Params` and `ID`), `WithEndpoint`, `WithHeader`, `WithAuthToken`,
  `WithHTTPClient`, `WithLog`, `WithLogField`, `WithLogrusFields`,
  `WithMSPFilter`, `WithTargetEndpoints`, `WithoutTargetEndpoints`,
  `WithMinEndorsers`, `WithCreator`, `WithDependentTxID`,
  `WithDependentBlock`, `WithPhylumVersion`, `WithDisableWritePolling`,
  `WithTimestampGenerator`, `WithCCFetchURLProxy`,
  `WithCCFetchURLDowngrade`, `WithResponse`, `WithResponseReceiver` and
  `WithUnsafeDebug`. A request's transient keys must also be non-empty, must
  not be one of the four transaction-wide keys, and must not start with
  `$batch/`, which is reserved (also in `Call`).
- **Requirements.** The gateway must include
  [luthersystems/substrate#521](https://github.com/luthersystems/substrate/pull/521),
  which also carries the per-request transient routing. An older gateway
  answers "method not found", and `CallBatch` returns an error matching
  `shiroclient.ErrCallBatchNotSupported` (`QueryBatch`:
  `shiroclient.ErrQueryBatchNotSupported`) without running anything. The
  mock client does not support batches yet (the substrate plugin has no
  batch method) and returns the same errors. Neither function ever falls
  back to separate `Call`s, which would not be atomic.
- **Compatibility.** `CallBatch` and `QueryBatch` are package functions over
  the optional `shiroclient.CallBatcher` and `shiroclient.QueryBatcher`
  interfaces, so the `ShiroClient` interface is unchanged.

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
