#!/usr/bin/env python3
"""Exercise installer failure boundaries without touching the owner's shell config."""
import hashlib, io, os, pathlib, subprocess, tarfile, tempfile, unittest
SCRIPT = pathlib.Path(__file__).with_name('install.sh').resolve()
VERSION = '0.1.0-rc.1'
class Installer(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='lm-installer-test-')
        self.base = pathlib.Path(self.tmp.name)
        self.root = self.base / "home with 'quote"
        self.mock = self.base / 'tools'; self.mock.mkdir()
        self.env = dict(os.environ, PATH=str(self.mock)+':/usr/bin:/bin:/usr/sbin:/sbin',
                        SHELL='/bin/zsh', LM_INSTALL_HOME=str(self.root), ZDOTDIR=str(self.root),
                        FIXTURES=str(self.base), MOCK_OS='Darwin', MOCK_ARCH='arm64')
        self.tool('uname', '#!/bin/sh\ncase "$1" in -s) echo "$MOCK_OS";; -m) echo "$MOCK_ARCH";; esac\n')
        self.tool('curl', '''#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in https://*) url=$1;; -o) shift; dest=$1;; esac
 shift
done
cp "$FIXTURES/${url##*/}" "$dest"
''')
        for system, arch in [('darwin','arm64'), ('darwin','amd64'), ('linux','arm64'), ('linux','amd64')]:
            name=f'lm-cli_{VERSION}_{system}_{arch}.tar.gz'
            data=b'#!/bin/sh\necho 0.1.0-rc.1\n'
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
    def test_install_repeat_and_new_zsh(self):
        self.run_install(); self.run_install()
        profile=self.root/'.zshrc'
        self.assertEqual(profile.read_text().count('# Living Memory CLI'),1)
        p=subprocess.run(['/bin/zsh','-ic','lm version'],env=self.env,text=True,capture_output=True)
        self.assertEqual(p.returncode,0,p.stderr); self.assertIn(VERSION,p.stdout)
    def test_bad_checksum_writes_nothing(self):
        (self.base/'SHA256SUMS').write_text('0'*64+f'  lm-cli_{VERSION}_darwin_arm64.tar.gz\n')
        self.run_install(False); self.assertFalse(self.root.exists())
    def test_existing_binary_preserved(self):
        p=self.root/'.local/bin/lm'; p.parent.mkdir(parents=True); p.write_text('keep me')
        self.run_install(False); self.assertEqual(p.read_text(),'keep me')
        self.assertFalse((self.root/'.zshrc').exists())
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
