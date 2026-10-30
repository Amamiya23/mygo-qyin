# Releasing

The app is **Lilt**. Its GitHub repository is `Amamiya23/mygo-qyin`; the application identifier stays `dev.amamiya.qqmusic` for credential and installer compatibility. Build artifacts use the `lilt` slug. `mygo.json` configures this repository as the signed update feed, and the app registers the native updater at startup.

Before the first release with updates, add the contents of the matching `mygo-update.key` as the repository Actions secret `MYGO_UPDATER_PRIVATE_KEY`. Keep a backup outside the repository and use the same key for future releases; the public key in `mygo.json` verifies their signatures. Older builds without updates need one manual installation of a build with updates enabled. Verify an installed build can update to a newer signed release; development builds cannot update themselves.

macOS builds currently use MyGo's default ad-hoc signature, without Developer ID signing or Apple notarization. The workflow needs no Apple credentials. macOS may require the user to allow the app to open on first launch. Update archives still use the update signing key on every platform.

1. Commit pending work, bring main up to date with origin, and run `go test ./internal/...`. Others push to main too, so a release cut from a stale main leaves out their work:

   ```bash
   git pull --rebase origin main
   ```

2. Bump `version` in `mygo.json`, and add a `## 0.1.6` section at the top of `CHANGELOG.md` saying, in Chinese, what changed for people using the app since the last release (`git log v0.1.5..HEAD`, after the pull). Commit both in a commit titled just the version, e.g. `0.1.6`. MyGo's updater shows that section in the app's update window, and the release page takes it as its notes; `mygo build` fails without one.
3. Push main first, and tag only once that push has gone through. Pushing both at once still pushes the tag when main is rejected, and the tag starts a release build of a commit that isn't on main:

   ```bash
   git push origin main
   git tag v0.1.6
   git push origin v0.1.6
   ```

   If main is rejected, pull again, add what came in to the changelog, amend the version commit, test, and push main again before tagging. If a tag went up on the wrong commit anyway, cancel its run, then delete the tag and the draft it made before tagging again:

   ```bash
   git push origin :refs/tags/v0.1.6
   gh release delete v0.1.6 --yes
   ```

4. `.github/workflows/release.yml` builds macOS, Windows and Linux and uploads them to a draft release. Wait for every job to pass:

   ```bash
   gh run watch "$(gh run list --workflow release.yml -L1 --json databaseId -q '.[0].databaseId')" --exit-status
   ```

   If a job fails, don't publish; fix it first.

5. Publish the draft and mark it latest:

   ```bash
   gh release edit v0.1.6 --draft=false --latest
   ```

6. Check the update feed shows the new version:

   ```bash
   curl -sL https://github.com/Amamiya23/mygo-qyin/releases/latest/download/update-darwin-arm64.json
   ```

7. Deploy the website. It reads the latest release when it's prerendered, so it has to be rebuilt after publishing:

   ```bash
   cd website && bun run deploy
   ```

   Then check `/download/` on your deployed Lilt site shows the new version. Set up your own Cloudflare deployment; the fork does not use the upstream site's address.

## The speech recognition libraries

Podcast captions run sherpa-onnx, whose libraries the app downloads with the model from the `sherpa-onnx-<version>` release, not from the app's own. They only change with sherpa-onnx's version, in `scripts/sherpa`. Then pack them again, signing the macOS ones with the app's Developer ID (the app's hardened runtime loads no others), commit the `internal/speech/natives.go` it writes, and upload them as a prerelease, which never becomes the latest that updates look for:

```bash
go run ./scripts/sherpa -sign "Developer ID Application: <your signing identity>"
gh release create sherpa-onnx-2.0.1 --prerelease --latest=false build/sherpa/*
```
