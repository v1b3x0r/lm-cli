#!/bin/sh
# Install the official lm release without administrator access.
set -eu
main() {
VERSION=0.1.0-rc.4
RELEASE_TAG=v0.1.0-rc.4
fail() { printf 'lm install: %s\n' "$*" >&2; exit 1; }
for tool in curl tar awk mktemp install cmp mv; do
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
base=https://github.com/v1b3x0r/lm-cli/releases/download/${RELEASE_TAG}
printf 'Downloading lm %s for %s/%s…\n' "$VERSION" "$platform" "$arch"
curl --proto '=https' --tlsv1.2 -fsSL --connect-timeout 15 --max-time 180 "$base/$archive" -o "$work/$archive"
curl --proto '=https' --tlsv1.2 -fsSL --connect-timeout 15 --max-time 60 "$base/SHA256SUMS" -o "$work/SHA256SUMS"
expected=$(awk -v name="$archive" '$2 == name { print $1; n++ } END { if (n != 1) exit 1 }' "$work/SHA256SUMS") || fail 'Release checksum entry missing or duplicated.'
[ "${#expected}" -eq 64 ] || fail 'Invalid release checksum.'
case "$expected" in *[!0-9a-f]*) fail 'Invalid release checksum.' ;; esac
file_hash() {
  if [ "$hash_tool" = shasum ]; then shasum -a 256 "$1" | awk '{print $1}'
  else sha256sum "$1" | awk '{print $1}'; fi
}
actual=$(file_hash "$work/$archive")
[ "$actual" = "$expected" ] || fail 'Checksum did not match. Nothing installed.'
# Extract only the executable, never documentation or arbitrary archive paths.
tar -xzf "$work/$archive" -C "$work" lm
[ -f "$work/lm" ] && [ ! -L "$work/lm" ] || fail 'Release does not contain a regular lm executable.'
chmod 755 "$work/lm"
[ "$("$work/lm" version)" = "$VERSION" ] || fail 'Downloaded executable could not report the expected version.'
mkdir -p "$bindir"
install_lock=$bindir/.lm-install.lock
lock_owned=0
stage=
backup_dir=
cleanup() {
  # A signal may arrive after the old entry moves but before RC4 is linked.
  if [ -n "$backup_dir" ] && { [ -e "$backup_dir/lm" ] || [ -L "$backup_dir/lm" ]; }; then
    if [ ! -e "$bindir/lm" ] && [ ! -L "$bindir/lm" ]; then
      if ln -P "$backup_dir/lm" "$bindir/lm" 2>/dev/null; then
        rm -f "$backup_dir/lm"
        rmdir "$backup_dir"
      fi
    fi
  fi
  [ -z "$stage" ] || rm -f "$stage"
  [ "$lock_owned" -eq 0 ] || rmdir "$install_lock" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT
# Ignore catchable signals only across lock acquisition and ownership assignment.
trap '' HUP INT TERM
mkdir -m 700 "$install_lock" 2>/dev/null || fail 'Another lm installation is running, or its lock remains. Nothing changed.'
lock_owned=1
trap 'exit 1' HUP INT TERM
if [ -e "$bindir/lm" ] || [ -L "$bindir/lm" ]; then
  [ -f "$bindir/lm" ] && [ ! -L "$bindir/lm" ] || fail "A different file exists at $bindir/lm. Nothing overwritten."
  if cmp -s "$work/lm" "$bindir/lm"; then
    printf 'lm %s is already installed.\n' "$VERSION"
  else
    previous_hash=$(file_hash "$bindir/lm")
    # Hashes of the extracted official release binaries, grouped by version.
    case "$previous_hash" in
      5f4edb126971308fd3bd84c685c9db35ea7c1c6ce96ac2ae1cbc5a96482b3a69|\
      5d9c3260bc7bc2ffcf0555c5f5e83e9488398d3a9fadbf0bf115afc74a6ff720|\
      fe06daf508ef6939e664f37efb4b34aa840eaf7f79b23236ad44fa7a3e521012|\
      5bad3ef05963128724efa40c1b91cb00430f06b978ce43059812186633459f7d) previous=0.1.0-rc.1 ;;
      8c610c8dccccfd4b41bcd0e59f1d86df480b246a15b6feee00fe8fa7411665d2|\
      94f65d7bd017d34730742bcbcc0bb3dbdeafb4ef9261b62aa183fcff20bf5848|\
      fa7ebdc97b98c6866006565402ac27172a98e446c869ccdd0208a3af5354d924|\
      10aa94b20d6fb626c84f71f77f0010ecc6362760bbe9dfcc1606ff68b075a40f) previous=0.1.0-rc.2 ;;
      305b20ec7bbee5e69318f34b933a9ee83cf7b1b9751ffbc16ecbb149948219f5|\
      003207f3ac307e5f14065d8a7b3286ebb63a5e0f093f742c7ac4eb1291c07d9b|\
      7ce80da1c04283ba5ec1641df0a4684175190421855feef3ff77cc92e855c40d|\
      35908c5e6640a0cc6d726b023e527df7d5ff56390aff01572390deadfddb43ac) previous=0.1.0-rc.3 ;;
      *) fail "An unrecognized lm exists at $bindir/lm. Nothing overwritten." ;;
    esac
    [ -f "$bindir/lm" ] && [ ! -L "$bindir/lm" ] &&
      [ "$(file_hash "$bindir/lm")" = "$previous_hash" ] || fail 'Existing lm changed during installation. Nothing overwritten.'
    stage=$(mktemp "$bindir/.lm-install.XXXXXX")
    install -m 755 "$work/lm" "$stage" || { rm -f "$stage"; fail 'Could not stage lm upgrade.'; }
    backup_dir=$(mktemp -d "$bindir/.lm-previous.XXXXXX")
    restore_or_preserve() {
      rm -f "$stage"
      if { [ -e "$backup_dir/lm" ] || [ -L "$backup_dir/lm" ]; } &&
         [ ! -e "$bindir/lm" ] && [ ! -L "$bindir/lm" ] &&
         ln -P "$backup_dir/lm" "$bindir/lm" 2>/dev/null; then
        rm -f "$backup_dir/lm"
        rmdir "$backup_dir"
        fail "$1 Previous executable restored."
      fi
      fail "$1 Displaced executable preserved at $backup_dir/lm."
    }
    # Move first, then verify the displaced entry; never overwrite a new arrival.
    if ! mv "$bindir/lm" "$backup_dir/lm"; then
      if [ -e "$backup_dir/lm" ] || [ -L "$backup_dir/lm" ]; then
        restore_or_preserve 'Could not move existing lm.'
      fi
      rm -f "$stage"
      rmdir "$backup_dir"
      fail 'Could not move existing lm. Nothing overwritten.'
    fi
    [ -f "$backup_dir/lm" ] && [ ! -L "$backup_dir/lm" ] &&
      [ "$(file_hash "$backup_dir/lm")" = "$previous_hash" ] || restore_or_preserve 'Existing lm changed during installation.'
    ln "$stage" "$bindir/lm" || restore_or_preserve 'Another executable appeared during installation.'
    rm -f "$stage"
    cmp -s "$work/lm" "$bindir/lm" || fail "Installed path changed; previous executable preserved at $backup_dir/lm."
    printf 'Upgraded lm from %s to %s.\n' "$previous" "$VERSION"
    printf 'Previous executable saved at %s/lm.\n' "$backup_dir"
  fi
else
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
