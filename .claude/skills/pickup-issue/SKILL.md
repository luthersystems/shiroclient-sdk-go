---
name: pickup-issue
description: "Full lifecycle from GitHub issue to merged PR. Reads the issue, creates a branch, implements the fix/feature, verifies, and opens a PR. Triggers: 'pickup issue #N', 'work on issue', 'grab an issue'."
---

# Pickup Issue

Complete lifecycle: read issue, branch, implement, verify, PR.

## Workflow

### 1. Read the Issue

```bash
gh issue view <NUMBER>
```

Understand the requirements, acceptance criteria, and any linked issues or discussions.

### 2. Create a Branch

Derive the branch name from the issue:

```bash
git checkout -b feature/issue-<NUMBER>-short-description main
```

Use `fix/` prefix for bug reports, `feature/` for enhancements.

### 3. Implement

Follow the `implement` skill:

1. Make changes following project conventions
2. Write/update tests with embedded LISP phylum and testify assertions
3. Lint: `golangci-lint run ./...`
4. Test: `make test`

### 4. Verify

Run the `verify` skill to mirror CI:

```bash
golangci-lint run ./... && make citest
```

### 5. Commit and Push

```bash
git add <changed-files>
git commit -m "Description matching issue intent"
git push -u origin HEAD
```

### 6. Create PR Linking the Issue

```bash
gh pr create --base main \
  --title "Short title describing the change" \
  --body "Fixes #<NUMBER>

Description of what was changed and why."
```

Using `Fixes #N` in the body auto-closes the issue when the PR merges.

### 7. Monitor CI

```bash
gh pr checks --watch
```

## Key Reminders

- Always read the full issue before starting implementation
- Link the PR to the issue using `Fixes #N` or `Closes #N`
- For large changes, comment on the issue with your approach before implementing (per CONTRIBUTING.md)
- Run `make plugin` if the substrate binary isn't already present

## Checklist

- [ ] Issue requirements understood
- [ ] Branch created from `main`
- [ ] Implementation follows project conventions
- [ ] Tests written/updated
- [ ] `verify` skill passes
- [ ] PR created and linked to issue
- [ ] CI checks pass
