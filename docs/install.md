# Install a release bundle

Release candidate: `0.1.0-rc.1`.
Obtain the archive and `SHA256SUMS` from the
[official release](https://github.com/v1b3x0r/lm-cli/releases/tag/v0.1.0-rc.1).
Do not fetch similarly named packages from an unrelated registry.

Choose your platform:

| Machine | Bundle suffix |
| --- | --- |
| Apple Silicon Mac | darwin_arm64.tar.gz |
| Intel Mac | darwin_amd64.tar.gz |
| Linux x86-64 | linux_amd64.tar.gz |
| Linux ARM64 | linux_arm64.tar.gz |

macOS/Linux: verify the archive's SHA-256 against its exact line in `SHA256SUMS`
using `shasum -a 256 <archive>` or `sha256sum <archive>`. Extract into an empty
directory and run `./lm version`. No Go or Node runtime is needed.

To install without administrator access, from that extracted directory:

```sh
mkdir -p "$HOME/.local/bin"
test ! -e "$HOME/.local/bin/lm" && install -m 755 ./lm "$HOME/.local/bin/lm"
export PATH="$HOME/.local/bin:$PATH"
lm version
```

This refuses to overwrite an existing `lm`. If that name is already installed,
inspect the existing command first. The PATH change applies to this shell; make
it persistent in your shell configuration if desired. macOS bundles are not
Developer ID signed/notarized; if macOS blocks execution, do not bypass the
warning. A signed release or a reviewed source build is required in that case.

Windows is deferred until private credential storage is implemented and tested
with Windows ACLs. This candidate includes macOS and Linux only.

Checksums detect corruption; they are not signatures or proof of publisher
identity. Only trust artifacts received through the official publication channel.

Then follow `QUICKSTART.md`. Uninstall removes only the executable; it does not
delete remote Rooms. Keep private grants if you intend to return to them.

## Download example: Apple Silicon Mac

```sh
curl -fLO https://github.com/v1b3x0r/lm-cli/releases/download/v0.1.0-rc.1/lm-cli_0.1.0-rc.1_darwin_arm64.tar.gz
curl -fLO https://github.com/v1b3x0r/lm-cli/releases/download/v0.1.0-rc.1/SHA256SUMS
shasum -a 256 lm-cli_0.1.0-rc.1_darwin_arm64.tar.gz
```

Compare the matching checksum before extracting and installing as above.
