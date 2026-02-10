---
name: implement
description: "Core development loop for making code changes. Use when implementing features, fixing bugs, or refactoring. Triggers: 'implement', 'make a change', 'add feature', 'fix bug'."
---

# Implement

Foundation for any code change in shiroclient-sdk-go. Covers the full edit-lint-build-test loop.

## Workflow

### 1. Ensure Plugin is Available

The mock client tests require the substrate plugin binary. Before running any tests:

```bash
make plugin
```

This downloads platform-specific binaries to `build/`. Skip if already present.

### 2. Make Your Changes

Follow these conventions:

- **Public API**: Expose via type aliases in `shiroclient/` package (see `shiroclient/shiroclient.go`)
- **Internal logic**: Lives in `internal/types/`, `internal/rpc/`, or `internal/mock/`
- **Configuration**: Add new options as `Config` builders in `shiroclient/configs.go`
- **New sub-packages**: Follow the pattern of `shiroclient/batch/`, `shiroclient/private/`, etc.
- **Deprecated APIs**: Suppress staticcheck with `//nolint:staticcheck` where backward compat requires it (e.g., `github.com/golang/protobuf/jsonpb`)

### 3. Write or Update Tests

- Embed LISP phylum code via `//go:embed *_test.lisp` directives
- Use `testify` (`require`/`assert`) for assertions
- Mock client setup pattern:
  ```go
  client, err := shiroclient.NewMock(nil)
  require.NoError(t, err)
  t.Cleanup(client.Close())
  ```
- Use snapshot/restore for ledger state tests: `client.Snapshot(writer)` / `mock.WithSnapshotReader(reader)`

### 4. Lint

```bash
golangci-lint run ./...
```

This matches CI (golangci-lint v1.63, default config). Fix any issues before proceeding.

### 5. Run Tests

Scoped test (recommended during development):

```bash
go test -timeout 10m -run TestName ./shiroclient/...
```

Full suite:

```bash
make test
```

### 6. Verify Build

```bash
go build ./...
```

Ensures all packages compile cleanly.

## Key Reminders

- Tests WILL FAIL without the substrate plugin. Run `make plugin` first.
- The `SUBSTRATEHCP_FILE` env var is set automatically by the Makefile to point to the platform binary.
- If you add a new `.lisp` test file, ensure the `//go:embed` directive in the corresponding `_test.go` file picks it up.
- No `.golangci.yml` exists; linting uses golangci-lint defaults.
- Go version is 1.23 (see `go.mod`).

## Checklist

- [ ] Plugin binary exists (`make plugin`)
- [ ] `golangci-lint run ./...` passes
- [ ] `go build ./...` compiles cleanly
- [ ] `make test` passes
- [ ] New/changed behavior has test coverage
