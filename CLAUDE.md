# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go SDK for the Luther Platform's Shiroclient gateway. Provides a JSON-RPC client for blockchain-based smart contract execution using ELPS (a LISP dialect) as the phylum language. Includes both RPC and mock (in-process via HashiCorp go-plugin) implementations.

**Module**: `github.com/luthersystems/shiroclient-sdk-go`
**Go version**: 1.23

## Build & Test Commands

```bash
# Download substrate plugin (required before running tests)
make plugin

# Run all tests
make test

# CI pipeline (downloads plugin + runs tests)
make citest

# Run a single test
go test -timeout 10m -run TestName ./shiroclient/...

# Lint (matches CI)
golangci-lint run ./...
```

The mock client requires the substrate plugin binary. Tests will fail without it. Run `make plugin` first.

## Architecture

### Core Interface

`internal/types/types.go` defines `ShiroClient` — the central interface with methods: `Seed`, `ShiroPhylum`, `Init`, `Call`, `QueryInfo`, `QueryBlock`. Two implementations exist:

- **RPC** (`internal/rpc/`) — HTTP JSON-RPC 2.0 client for production use
- **Mock** (`internal/mock/`) — in-process ledger via HashiCorp go-plugin for testing

### Public API Pattern

The `shiroclient/` package re-exports internal types as type aliases (`ShiroClient = types.ShiroClient`, etc.) to provide a stable public API while keeping implementation details internal. Constructors: `NewRPC(configs)` and `NewMock(configs, opts...)`.

### Configuration Pattern

Functional options via `Config` interface. All operations accept variadic `...Config` params. Base configs are set at client creation; per-call configs override them. Builder functions live in `shiroclient/configs.go` (e.g., `WithEndpoint`, `WithParams`, `WithHeader`, `WithTransientData`). Configs are applied via `types.ApplyConfigs()` which builds a `RequestOptions` struct.

### Sub-packages

- `shiroclient/batch/` — polling driver for batch request processing
- `shiroclient/private/` — AES-256 encryption for PII (GDPR export/purge support)
- `shiroclient/phylum/` — high-level phylum client wrapping ShiroClient with protobuf helpers
- `shiroclient/update/` — phylum version management (install, enable, disable)
- `x/` — unstable internal packages, not for external consumption

### Testing Patterns

- Tests embed LISP phylum code via `//go:embed *_test.lisp` directives
- Mock client setup: `client, err := shiroclient.NewMock(nil)` with `t.Cleanup(client.Close())`
- Uses `testify` (`require`/`assert`) for assertions
- Snapshot/restore for ledger state: `client.Snapshot(writer)` / `mock.WithSnapshotReader(reader)`

### Key Dependencies

- `buf.build/gen/go/luthersystems/protos` — generated protobuf types
- `github.com/luthersystems/svc` — Luther service utilities (txctx for transaction context)
- `github.com/hashicorp/go-plugin` — plugin system for mock substrate
- Uses deprecated `github.com/golang/protobuf/jsonpb` for backwards compatibility (suppressed via `//nolint:staticcheck`)

## CI

GitHub Actions on PRs to `main`: golangci-lint v1.63, then `make citest`. See `.github/workflows/shiroclient-sdk-go.yml`.

## Skills

| Skill | Purpose |
|-------|---------|
| `implement` | Core dev loop — edit, lint, build, test |
| `verify` | Local CI gate — run all checks before pushing |
| `pr` | Ship changes — verify, push, create PR |
| `pickup-issue` | Full lifecycle — issue to branch to PR |
| `release` | Tag and publish a new semver release |
