---
name: pr
description: "Ship changes via pull request. Runs verification, pushes, and creates a PR targeting main. Triggers: 'create PR', 'open PR', 'ship it', 'submit for review'."
---

# PR

Ship changes from branch to merged pull request.

## Workflow

### 1. Verify

Run the `verify` skill first to ensure all CI checks pass locally:

```bash
golangci-lint run ./... && make citest
```

Do NOT proceed if verification fails.

### 2. Create Branch (If Not Already On One)

Branch naming convention (from repo history):

- Features: `feature/short-description` or `sam-at-luther/Feature_name`
- Fixes: `fix/short-description`
- Docs: `docs/short-description`
- Refactors: `refactor/short-description`

```bash
git checkout -b feature/my-change main
```

### 3. Commit Changes

Stage and commit with a descriptive message. PR titles from repo history use these patterns:

- `Add <thing>` for new features/packages
- `Expose <thing>` for making internal functionality public
- `Fix <thing>` or `Properly handle <thing>` for bug fixes
- `Bump deps` or `Update <dep>` for dependency changes
- `Refactor <thing>` for structural changes

### 4. Push

```bash
git push -u origin HEAD
```

### 5. Create PR

Target branch is always `main`. Use `gh` CLI:

```bash
gh pr create --base main --title "Short descriptive title" --body "Description of changes"
```

Keep PR titles short (under 70 characters). Use the body for details.

### 6. Wait for CI

```bash
gh pr checks --watch
```

CI runs golangci-lint + `make citest`. If checks fail, fix locally and push again.

## Key Reminders

- Always target `main` branch
- Never push directly to `main`
- Keep PRs focused: one logical change per PR
- Large changes should have a GitHub issue filed first for discussion (see CONTRIBUTING.md)
- Contributors must be listed in AUTHORS and CONTRIBUTORS files

## Checklist

- [ ] `verify` skill passes locally
- [ ] Branch is pushed to origin
- [ ] PR created targeting `main`
- [ ] CI checks pass (`gh pr checks`)
- [ ] PR title is descriptive and under 70 characters
