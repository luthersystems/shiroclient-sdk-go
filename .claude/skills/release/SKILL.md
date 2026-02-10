---
name: release
description: "Create a new release with a semver tag and GitHub release. Triggers: 'release', 'cut a release', 'tag a new version', 'create release'."
---

# Release

Tag and publish a new release. This repo uses semantic versioning (v0.x.y).

## Workflow

### 1. Determine Version

Check the latest tag:

```bash
git tag --sort=-v:refname | head -5
```

Follow semver:
- **Patch** (v0.13.x): Bug fixes, doc updates, minor improvements
- **Minor** (v0.x.0): New features, new sub-packages, API additions
- **Major** (vX.0.0): Breaking API changes (hasn't happened yet; repo is pre-1.0)

### 2. Ensure Main is Clean

```bash
git checkout main
git pull origin main
golangci-lint run ./... && make citest
```

All checks must pass before tagging.

### 3. Tag the Release

```bash
git tag v<VERSION>
git push origin v<VERSION>
```

### 4. Create GitHub Release

```bash
gh release create v<VERSION> --title "v<VERSION>" --generate-notes
```

The `--generate-notes` flag auto-generates release notes from merged PRs since the last tag.

### 5. Verify

```bash
gh release view v<VERSION>
```

Confirm the release appears on GitHub with correct notes.

## Key Reminders

- Tags are created on `main` branch only
- No CHANGELOG file exists; release notes are auto-generated from PR titles
- Pre-release versions use `-SNAPSHOT.N` suffix (e.g., `v0.12.0-SNAPSHOT.0`)
- The Go module proxy (`proxy.golang.org`) will index the new version automatically after tagging
- Substrate plugin version is pinned in `common.config.mk` and is independent of SDK releases

## Checklist

- [ ] All CI checks pass on `main`
- [ ] Version follows semver convention
- [ ] Tag pushed to origin
- [ ] GitHub release created with notes
- [ ] Release visible at `gh release list`
