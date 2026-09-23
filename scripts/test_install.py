#!/usr/bin/env python3
"""Exercise installer failure boundaries without touching the owner's shell config."""
import hashlib, io, os, pathlib, shlex, shutil, subprocess, tarfile, tempfile, unittest
SCRIPT = pathlib.Path(__file__).with_name('install.sh').resolve()
VERSION = '0.1.0-rc.4'
RELEASE_TAG = 'v0.1.0-rc.4'
RELEASE_BASE = f'https://github.com/v1b3x0r/lm-cli/releases/download/{RELEASE_TAG}'
class Installer(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='lm-installer-test-')
        self.base = pathlib.Path(self.tmp.name)
        self.root = self.base / "home with 'quote"
        self.mock = self.base / 'tools'; self.mock.mkdir()
        self.env = dict(os.environ, PATH=str(self.mock)+':/usr/bin:/bin:/usr/sbin:/sbin',
                        SHELL='/bin/zsh', LM_INSTALL_HOME=str(self.root), ZDOTDIR=str(self.root),
                        FIXTURES=str(self.base), MOCK_OS='Darwin', MOCK_ARCH='arm64',
                        EXPECTED_RELEASE_BASE=RELEASE_BASE, EXPECTED_VERSION=VERSION)
        self.tool('uname', '#!/bin/sh\ncase "$1" in -s) echo "$MOCK_OS";; -m) echo "$MOCK_ARCH";; esac\n')
        self.tool('curl', '''#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in https://*) url=$1;; -o) shift; dest=$1;; esac
 shift
done
case "$MOCK_OS" in Darwin) platform=darwin;; Linux) platform=linux;; *) exit 22;; esac
case "$MOCK_ARCH" in arm64|aarch64) arch=arm64;; x86_64|amd64) arch=amd64;; *) exit 22;; esac
case "$url" in
 "$EXPECTED_RELEASE_BASE/lm-cli_${EXPECTED_VERSION}_${platform}_${arch}.tar.gz"|"$EXPECTED_RELEASE_BASE/SHA256SUMS") ;;
 *) echo "Unexpected release URL: $url" >&2; exit 22;;
esac
printf '%s\\n' "$url" >> "$FIXTURES/requested-urls"
cp "$FIXTURES/${url##*/}" "$dest"
''')
        for system, arch in [('darwin','arm64'), ('darwin','amd64'), ('linux','arm64'), ('linux','amd64')]:
            name=f'lm-cli_{VERSION}_{system}_{arch}.tar.gz'
            data=f'#!/bin/sh\necho {VERSION}\n'.encode()
            with tarfile.open(self.base/name,'w:gz') as tf:
                info=tarfile.TarInfo('lm'); info.mode=0o755; info.size=len(data)
                tf.addfile(info, io.BytesIO(data))
        self.checksums()
    def tearDown(self): self.tmp.cleanup()
    def tool(self,name,text):
        p=self.mock/name; p.write_text(text); p.chmod(0o755)
    def checksums(self):
        (self.base/'SHA256SUMS').write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in self.base.glob('*.tar.gz')))
    def run_install(self,success=True):
        p=subprocess.run(['/bin/sh',str(SCRIPT)],env=self.env,text=True,capture_output=True)
        self.assertEqual(p.returncode==0,success,p.stdout+p.stderr)
        return p
    def trust_fixture_as_rc3(self,p, official_hash='003207f3ac307e5f14065d8a7b3286ebb63a5e0f093f742c7ac4eb1291c07d9b'):
        (self.base/'old-lm').write_bytes(p.read_bytes())
        real_shasum=shutil.which('shasum')
        if not real_shasum: self.skipTest('shasum is unavailable')
        self.tool('shasum',f'''#!/bin/sh
if cmp -s "$3" "$FIXTURES/old-lm"; then
  printf '%s  %s\\n' '{official_hash}' "$3"
else
  exec {shlex.quote(real_shasum)} "$@"
fi
''')
    def test_install_repeat_and_new_zsh(self):
        self.run_install(); self.run_install()
        profile=self.root/'.zshrc'
        self.assertEqual(profile.read_text().count('# Living Memory CLI'),1)
        p=subprocess.run(['/bin/zsh','-ic','lm version'],env=self.env,text=True,capture_output=True)
        self.assertEqual(p.returncode,0,p.stderr); self.assertIn(VERSION,p.stdout)
    def test_exact_release_urls(self):
        self.run_install()
        self.assertEqual((self.base/'requested-urls').read_text().splitlines(), [
            f'{RELEASE_BASE}/lm-cli_{VERSION}_darwin_arm64.tar.gz',
            f'{RELEASE_BASE}/SHA256SUMS',
        ])
    def test_bad_checksum_writes_nothing(self):
        (self.base/'SHA256SUMS').write_text('0'*64+f'  lm-cli_{VERSION}_darwin_arm64.tar.gz\n')
        self.run_install(False); self.assertFalse(self.root.exists())
    def test_existing_binary_preserved(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True); p.write_text('keep me')
        self.run_install(False); self.assertEqual(p.read_text(),'keep me')
        self.assertFalse((self.root/'.zshrc').exists())
    def test_upgrade_known_rc3_binary(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        old=b'#!/bin/sh\necho 0.1.0-rc.3\n'
        p.write_bytes(old); p.chmod(0o755)
        self.trust_fixture_as_rc3(p)
        result=self.run_install()
        self.assertIn(f'Upgraded lm from 0.1.0-rc.3 to {VERSION}', result.stdout)
        self.assertEqual(subprocess.check_output([str(p),'version'],text=True).strip(),VERSION)
        self.assertEqual(list(p.parent.glob('.lm-install.*')),[])
        backups=list(p.parent.glob('.lm-previous.*/lm'))
        self.assertEqual(len(backups),1)
        self.assertEqual(backups[0].read_bytes(),old)
        self.assertIn(str(backups[0]),result.stdout)
    def test_upgrade_prior_binary_from_another_architecture(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        p.write_text('#!/bin/sh\necho 0.1.0-rc.3\n'); p.chmod(0o755)
        self.trust_fixture_as_rc3(p, '7ce80da1c04283ba5ec1641df0a4684175190421855feef3ff77cc92e855c40d')
        self.run_install()
        self.assertEqual(subprocess.check_output([str(p),'version'],text=True).strip(),VERSION)
    def test_late_in_place_update_keeps_displaced_inode(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        p.write_text('#!/bin/sh\necho 0.1.0-rc.3\n'); p.chmod(0o755)
        self.trust_fixture_as_rc3(p)
        self.tool('ln','''#!/bin/sh
/bin/ln "$@" || exit
if [ "$2" = "$LM_INSTALL_HOME/.local/bin/lm" ]; then
  for old in "$LM_INSTALL_HOME"/.local/bin/.lm-previous.*/lm; do
    printf 'concurrent\\n' >> "$old"
  done
fi
''')
        self.run_install()
        backups=list(p.parent.glob('.lm-previous.*/lm'))
        self.assertEqual(len(backups),1)
        self.assertTrue(backups[0].read_bytes().endswith(b'concurrent\n'))
        self.assertEqual(subprocess.check_output([str(p),'version'],text=True).strip(),VERSION)
    def test_changed_entry_before_move_is_restored(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        p.write_text('#!/bin/sh\necho 0.1.0-rc.3\n'); p.chmod(0o755)
        self.trust_fixture_as_rc3(p)
        self.tool('mv','''#!/bin/sh
if [ "$1" = "$LM_INSTALL_HOME/.local/bin/lm" ]; then printf 'concurrent\\n' > "$1"; fi
exec /bin/mv "$@"
''')
        self.run_install(False)
        self.assertEqual(p.read_text(),'concurrent\n')
        self.assertEqual(list(p.parent.glob('.lm-previous.*')),[])
    def test_symlink_replacement_before_move_is_restored(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        p.write_text('#!/bin/sh\necho 0.1.0-rc.3\n'); p.chmod(0o755)
        self.trust_fixture_as_rc3(p)
        target=self.base/'replacement'; target.write_text('concurrent\n')
        self.env['REPLACEMENT_TARGET']=str(target)
        self.tool('mv','''#!/bin/sh
if [ "$1" = "$LM_INSTALL_HOME/.local/bin/lm" ]; then
  rm -f "$1"
  ln -s "$REPLACEMENT_TARGET" "$1"
fi
exec /bin/mv "$@"
''')
        self.run_install(False)
        self.assertTrue(p.is_symlink())
        self.assertEqual(p.resolve(),target.resolve())
        self.assertEqual(list(p.parent.glob('.lm-previous.*')),[])
    def test_new_entry_after_move_is_not_overwritten(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        old=b'#!/bin/sh\necho 0.1.0-rc.3\n'; p.write_bytes(old); p.chmod(0o755)
        self.trust_fixture_as_rc3(p)
        self.tool('ln','''#!/bin/sh
if [ "$2" = "$LM_INSTALL_HOME/.local/bin/lm" ]; then printf 'concurrent\\n' > "$2"; fi
exec /bin/ln "$@"
''')
        self.run_install(False)
        self.assertEqual(p.read_text(),'concurrent\n')
        backups=list(p.parent.glob('.lm-previous.*/lm'))
        self.assertEqual(len(backups),1)
        self.assertEqual(backups[0].read_bytes(),old)
    def test_signal_after_move_restores_previous_binary(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        old=b'#!/bin/sh\necho 0.1.0-rc.3\n'; p.write_bytes(old); p.chmod(0o755)
        self.trust_fixture_as_rc3(p)
        self.tool('mv','''#!/bin/sh
/bin/mv "$@" || exit
kill -TERM "$PPID"
''')
        self.run_install(False)
        self.assertEqual(p.read_bytes(),old)
        self.assertEqual(list(p.parent.glob('.lm-previous.*')),[])
        self.assertFalse((p.parent/'.lm-install.lock').exists())
    def test_version_spoof_is_not_executed_or_overwritten(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        data=b'#!/bin/sh\ntouch "$FIXTURES/executed"\necho 0.1.0-rc.3\n'
        p.write_bytes(data); p.chmod(0o755)
        self.run_install(False)
        self.assertEqual(p.read_bytes(),data)
        self.assertFalse((self.base/'executed').exists())
    def test_existing_install_lock_preserves_binary(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True)
        p.write_text('keep me')
        (p.parent/'.lm-install.lock').mkdir()
        self.run_install(False)
        self.assertEqual(p.read_text(),'keep me')
    def test_signal_during_lock_acquisition_leaves_no_lock(self):
        self.tool('mkdir','''#!/bin/sh
/bin/mkdir "$@" || exit
case "$*" in *'.lm-install.lock'*) kill -TERM "$PPID";; esac
''')
        self.run_install()
        self.assertFalse((self.root/'.local/bin/.lm-install.lock').exists())
    def test_other_lm_on_path(self):
        self.tool('lm','#!/bin/sh\nexit 0\n'); self.run_install(False)
        self.assertFalse(self.root.exists())
    def test_supported_platform_mapping(self):
        for system,arch in [('Darwin','x86_64'),('Linux','aarch64'),('Linux','x86_64')]:
            self.env.update(MOCK_OS=system,MOCK_ARCH=arch)
            self.run_install()
    def test_bash_existing_login_profile_preserved(self):
        self.root.mkdir(); p=self.root/'.profile'; p.write_text('# existing settings\n')
        self.env['SHELL']='/bin/bash'; self.run_install(); self.run_install()
        self.assertFalse((self.root/'.bash_profile').exists())
        self.assertEqual(p.read_text().count('# Living Memory CLI'),1)
        self.assertTrue(p.read_text().startswith('# existing settings'))
        result=subprocess.run(['/bin/bash','--noprofile','--rcfile',str(self.root/'.bashrc'),'-ic','lm version'],env=self.env,text=True,capture_output=True)
        self.assertEqual(result.returncode,0,result.stderr)
    def test_failed_download(self):
        self.tool('curl','#!/bin/sh\nexit 22\n'); self.run_install(False)
        self.assertFalse(self.root.exists())
    def test_unsupported_platform(self):
        self.env['MOCK_OS']='Windows'; self.run_install(False)
        self.assertFalse(self.root.exists())
if __name__ == '__main__': unittest.main(verbosity=2)
