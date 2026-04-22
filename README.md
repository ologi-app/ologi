# ologi — Ologi Voice CLI

Talk your way through your AI conversations. macOS only in v1.

## Install

```sh
brew install ologi-app/tap/ologi
```

## Use

```sh
ologi login         # link this device
ologi voice start   # background daemon; double-tap right_option to dictate
```

See `ologi --help` for all subcommands, or read the spec at
[`docs/superpowers/specs/2026-04-21-ologi-voice-cli-design.md`](../../docs/superpowers/specs/2026-04-21-ologi-voice-cli-design.md).

## Releasing

Releases are cut by pushing a tag matching `ologi-cli-v*`:

```
git tag ologi-cli-v0.1.0
git push --tags
```

The CI workflow `.github/workflows/ologi-cli-release.yml` builds darwin
arm64 + amd64 tarballs and uploads them as GitHub Release assets.

### Homebrew tap (one-time setup)

A separate repo `ologi/homebrew-tap` holds the formula. First time:

1. Create repo at `github.com/ologi/homebrew-tap`.
2. Add `Formula/ologi.rb`:

   ```ruby
   class Ologi < Formula
     desc "Ologi — talk your way through your AI conversations"
     homepage "https://ologi.app/voice"
     version "0.1.0"

     depends_on "portaudio"
     depends_on :macos

     on_macos do
       on_arm do
         url "https://github.com/<your-gh-org>/hypertask/releases/download/ologi-cli-v0.1.0/ologi-0.1.0-darwin-arm64.tar.gz"
         sha256 "<fill-from-CI-artifact>"
       end
       on_intel do
         url "https://github.com/<your-gh-org>/hypertask/releases/download/ologi-cli-v0.1.0/ologi-0.1.0-darwin-amd64.tar.gz"
         sha256 "<fill-from-CI-artifact>"
       end
     end

     def install
       bin.install "ologi"
     end

     test do
       assert_match(/^ologi /, shell_output("#{bin}/ologi --version"))
     end
   end
   ```

3. On each new `ologi-cli-v*` release, bump `version` and the two SHAs
   using the values emitted by the workflow (they're in the `.sha256`
   files attached to the GitHub Release).

Users install with:

```
brew install ologi-app/tap/ologi
```
