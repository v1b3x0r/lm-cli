#!/bin/sh
# Install the official lm release without administrator access.
set -eu
main() {
VERSION=0.1.0-rc.2
fail() { printf 'lm install: %s\n' "$*" >&2; exit 1; }
for tool in curl tar awk mktemp install cmp; do
  command -v "$tool" >/dev/null 2>&1 || fail "Required command missing: $tool"
done
case "$(uname -s)" in Darwin) platform=darwin ;; Linux) platform=linux ;; *) fail 'Supported systems: macOS and Linux.' ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) fail 'Supported processors: Apple Silicon/ARM64 and x86-64.' ;; esac
# LM_INSTALL_HOME permits an isolated installation without changing HOME.
root=${LM_INSTALL_HOME:-$HOME}
case "$root" in /*) ;; *) fail 'Installation home must be an absolute path.' ;; esac
case "$root" in *'
'*) fail 'Installation home cannot contain a newline.' ;; esac
bindir=$root/.local/bin
shell_name=${SHELL:-}
shell_name=${shell_name##*/}
case "$shell_name" in
  zsh) profiles="${ZDOTDIR:-$root}/.zshrc" ;;
  bash) profiles="$root/.bashrc" ;;
  *) fail 'Automatic setup supports zsh and bash. Use the manual release download for your shell.' ;;
esac
existing=$(command -v lm 2>/dev/null || true)
if [ -n "$existing" ] && [ "$existing" != "$bindir/lm" ]; then
  fail "An lm command already exists at $existing. Nothing changed."
fi
if command -v shasum >/dev/null 2>&1; then hash_tool=shasum
elif command -v sha256sum >/dev/null 2>&1; then hash_tool=sha256sum
else fail 'SHA-256 verification needs shasum or sha256sum.'; fi
work=$(mktemp -d "${TMPDIR:-/tmp}/lm-install.XXXXXX")
trap 'rm -rf "$work"' EXIT
trap 'exit 1' HUP INT TERM
archive=lm-cli_${VERSION}_${platform}_${arch}.tar.gz
base=https://github.com/v1b3x0r/lm-cli/releases/download/v${VERSION}
printf 'Downloading lm %s for %s/%s…\n' "$VERSION" "$platform" "$arch"
curl --proto '=https' --tlsv1.2 -fsSL --connect-timeout 15 --max-time 180 "$base/$archive" -o "$work/$archive"
curl --proto '=https' --tlsv1.2 -fsSL --connect-timeout 15 --max-time 60 "$base/SHA256SUMS" -o "$work/SHA256SUMS"
expected=$(awk -v name="$archive" '$2 == name { print $1; n++ } END { if (n != 1) exit 1 }' "$work/SHA256SUMS") || fail 'Release checksum entry missing or duplicated.'
[ "${#expected}" -eq 64 ] || fail 'Invalid release checksum.'
case "$expected" in *[!0-9a-f]*) fail 'Invalid release checksum.' ;; esac
if [ "$hash_tool" = shasum ]; then actual=$(shasum -a 256 "$work/$archive" | awk '{print $1}')
else actual=$(sha256sum "$work/$archive" | awk '{print $1}'); fi
[ "$actual" = "$expected" ] || fail 'Checksum did not match. Nothing installed.'
# Extract only the executable, never documentation or arbitrary archive paths.
tar -xzf "$work/$archive" -C "$work" lm
[ -f "$work/lm" ] && [ ! -L "$work/lm" ] || fail 'Release does not contain a regular lm executable.'
chmod 755 "$work/lm"
[ "$("$work/lm" version)" = "$VERSION" ] || fail 'Downloaded executable could not report the expected version.'
if [ -e "$bindir/lm" ] || [ -L "$bindir/lm" ]; then
  [ -f "$bindir/lm" ] && [ ! -L "$bindir/lm" ] && cmp -s "$work/lm" "$bindir/lm" || fail "A different file exists at $bindir/lm. Nothing overwritten."
  printf 'lm %s is already installed.\n' "$VERSION"
else
  mkdir -p "$bindir"
  # A hard link publishes a complete file and refuses a concurrent overwrite.
  stage=$(mktemp "$bindir/.lm-install.XXXXXX")
  if install -m 755 "$work/lm" "$stage" && ln "$stage" "$bindir/lm"; then rm -f "$stage"
  else rm -f "$stage"; fail 'Could not install lm without overwriting an existing file.'; fi
fi
quoted=$(printf '%s' "$bindir" | sed "s/'/'\\\\''/g")
path_line="export PATH='$quoted':\"\$PATH\""
add_path() {
  profile=$1
  if [ -e "$profile" ] && [ ! -f "$profile" ]; then fail "Not a regular shell profile: $profile"; fi
  mkdir -p "$(dirname "$profile")"
  if ! grep -Fqx "$path_line" "$profile" 2>/dev/null; then
    printf '\n# Living Memory CLI\n%s\n' "$path_line" >> "$profile"
    printf 'Added lm to PATH in %s\n' "$profile"
  fi
}
add_path "$profiles"
if [ "$shell_name" = bash ]; then
  # Bash reads only the first existing login profile; do not shadow .profile.
  if [ -f "$root/.bash_profile" ]; then login=$root/.bash_profile
  elif [ -f "$root/.bash_login" ]; then login=$root/.bash_login
  elif [ -f "$root/.profile" ]; then login=$root/.profile
  else login=$root/.bash_profile; fi
  add_path "$login"
fi
printf '\nInstalled lm %s. Open a new Terminal tab, then run:\n\n  lm version\n  lm create my-room --room\n\n' "$VERSION"

}
main "$@"
