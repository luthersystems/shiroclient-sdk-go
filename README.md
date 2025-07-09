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
