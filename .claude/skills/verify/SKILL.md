---
name: verify
description: "Local CI mirror. Run all checks that GitHub Actions runs before pushing. Use before creating PRs or to debug CI failures. Triggers: 'verify', 'check', 'run CI locally', 'pre-push check'."
---

# Verify

Run every check that CI runs, locally, so you never push broken code.

## Workflow

CI runs these checks on PRs to `main` (see `.github/workflows/shiroclient-sdk-go.yml`,
and the shared Quality workflow in `.github/workflows/quality.yml`):

### 1. Lint (golangci-lint v2.15.0)

```bash
golangci-lint config verify
golangci-lint run ./...
```

Rules live in `.golangci.yml` (shared with luthersystems/elps and
luthersystems/substrate). Use v2.15.0 locally, the shared Quality workflow's pin: findings differ
across minors.

### 1b. Work markers

The shared Quality workflow runs the work marker gate. A work marker (one of
the five upper-case words) must name an open issue, as `MARKER(#N)`. The
rules are in docs/quality-workflow.md in luthersystems/.github.

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

- CI runs on `ubuntu-latest` with the Go version in go.mod. Ensure your local Go version matches.
- There is no separate format check; formatting issues are caught by golangci-lint.
- The plugin download (`make plugin`) uses `scripts/obtain-plugin.sh` and requires network access.
- Plugin version is pinned in `common.config.mk` (`SUBSTRATE_VERSION=v2.240.0`).

## Checklist

- [ ] `golangci-lint run ./...` passes (matches CI lint step)
- [ ] `make citest` passes (matches CI test step)
- [ ] No untracked generated files left behind
