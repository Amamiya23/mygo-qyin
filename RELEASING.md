# Releasing

1. Commit pending work and run `go test ./internal/...`.
2. Bump `version` in `mygo.json`, and add a `## 0.1.8` section at the top of `CHANGELOG.md` saying, in Chinese, what changed for people using the app since the last release. Commit both in a commit titled just the version, e.g. `0.1.8`. MyGo's updater shows that section in the app's update window, and the release page takes it as its notes; `mygo build` fails without one.
3. Tag it `v<version>` and push main and the tag:

   ```bash
   git tag v0.1.8
   git push origin main v0.1.8
   ```

4. `.github/workflows/release.yml` builds macOS, Windows and Linux and uploads them to a draft release. Wait for every job to pass:

   ```bash
   gh run watch "$(gh run list --workflow release.yml -L1 --json databaseId -q '.[0].databaseId')" --exit-status
   ```

   If a job fails, don't publish; fix it first.

5. Publish the draft and mark it latest:

   ```bash
   gh release edit v0.1.8 --draft=false --latest
   ```

6. Check the update feed shows the new version:

   ```bash
   curl -sL https://github.com/egoist/mygo-cloudmusic/releases/latest/download/update-darwin-arm64.json
   ```

7. Deploy the website. It reads the latest release when it's prerendered, so it has to be rebuilt after publishing:

   ```bash
   cd website && bun run deploy
   ```

   Then check https://mygo-cloudmusic.egoist.workers.dev/download/ shows the new version.

## The speech recognition libraries

Podcast captions run sherpa-onnx, whose libraries the app downloads with the model from the `sherpa-onnx-<version>` release, not from the app's own. They only change with sherpa-onnx's version, in `scripts/sherpa`. Then pack them again, signing the macOS ones with the app's Developer ID (the app's hardened runtime loads no others), commit the `internal/speech/natives.go` it writes, and upload them as a prerelease, which never becomes the latest that updates look for:

```bash
go run ./scripts/sherpa -sign "Developer ID Application: xinyu zeng (GJE9R5VE87)"
gh release create sherpa-onnx-2.0.1 --prerelease --latest=false build/sherpa/*
```
