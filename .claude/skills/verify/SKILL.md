---
name: verify
description: "Local CI mirror. Run all checks that GitHub Actions runs before pushing. Use before creating PRs or to debug CI failures. Triggers: 'verify', 'check', 'run CI locally', 'pre-push check'."
---

# Verify

Run every check that CI runs, locally, so you never push broken code.

## Workflow

CI runs two steps on PRs to `main` (see `.github/workflows/shiroclient-sdk-go.yml`):

### 1. Lint (golangci-lint v1.63)

```bash
golangci-lint run ./...
```

No custom config file exists; uses golangci-lint defaults with version v1.63.

### 2. Full CI Test Suite

```bash
make citest
```

This runs `make plugin` (downloads substrate binary if missing) followed by `make test` (runs `go test -timeout 10m ./...`).

## Quick Verify (If Plugin Already Downloaded)

If you've already run `make plugin` in this session:

```bash
golangci-lint run ./... && make test
```

## Key Reminders

- CI runs on `ubuntu-latest` with Go 1.23. Ensure your local Go version matches.
- There is no separate format check; formatting issues are caught by golangci-lint.
- The plugin download (`make plugin`) uses `scripts/obtain-plugin.sh` and requires network access.
- Plugin version is pinned in `common.config.mk` (`SUBSTRATE_VERSION=v2.205.0`).

## Checklist

- [ ] `golangci-lint run ./...` passes (matches CI lint step)
- [ ] `make citest` passes (matches CI test step)
- [ ] No untracked generated files left behind
