# [P2] Release engineering: first tag, changelog, compatibility gates, CI matrix

Labels: P2, release

- Cut v0.1.0 with `scripts/release.sh v0.1.0 --push` (tags the root, `all/` and every `extension/*` module in lockstep); the `Release` workflow then installs every module from a fresh consumer.
- Add CHANGELOG.md and release notes.
- `apidiff`/`gorelease` on PRs after the first tag.
- The `go 1.26.0` directive excludes users on older toolchains, yet the core builds and vets with Go 1.23; lower it (e.g. `go 1.24`) and test `oldstable` + `stable` instead of pinning `1.26.0` (which also misses patch-release security fixes).
- Add Windows (CRLF paths) and `GOARCH=386` (int overflow) jobs.
