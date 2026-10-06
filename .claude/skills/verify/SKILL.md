---
name: verify
description: "Local CI mirror. Run all checks that GitHub Actions runs before pushing. Use before creating PRs or to debug CI failures. Triggers: 'verify', 'check', 'run CI locally', 'pre-push check'."
---

# Verify

Run every check that CI runs, locally, so you never push broken code.

## Workflow

CI runs these checks on PRs to `main` (see `.github/workflows/shiroclient-sdk-go.yml`):

### 1. Lint (golangci-lint v2.13)

```bash
golangci-lint config verify
golangci-lint run ./...
```

Rules live in `.golangci.yml` (shared with luthersystems/elps and
luthersystems/substrate). Use v2.13 locally: findings differ across minors.

### 1b. Work marker gate

```bash
make work-marker-gate
```

Fails on an upper-case work marker word that `scripts/work-markers.txt` does
not allow. Do the work, or open an issue and add a dated entry.

### 2. Full CI Test Suite

```bash
make citest
```

This runs `make plugin` (downloads substrate binary if missing) followed by `make test` (runs `go test -timeout 10m ./...`).

## Quick Verify (If Plugin Already Downloaded)

If you've already run `make plugin` in this session:

```bash
golangci-lint run ./... && make work-marker-gate && make test
```

## Key Reminders

- CI runs on `ubuntu-latest` with Go 1.25. Ensure your local Go version matches.
- There is no separate format check; formatting issues are caught by golangci-lint.
- The plugin download (`make plugin`) uses `scripts/obtain-plugin.sh` and requires network access.
- Plugin version is pinned in `common.config.mk` (`SUBSTRATE_VERSION=v2.240.0`).

## Checklist

- [ ] `golangci-lint run ./...` passes (matches CI lint step)
- [ ] `make work-marker-gate` passes
- [ ] `make citest` passes (matches CI test step)
- [ ] No untracked generated files left behind
