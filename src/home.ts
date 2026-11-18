import { homedir } from 'node:os';
import { posix, win32 } from 'node:path';

/**
 * Where lm keeps its private files. Mirrors Go's os.UserConfigDir() + "/lm-cli",
 * so an account saved by the rc5 binary is found by this one.
 */
export function configHome(
  env: NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
  home: string = homedir(),
): string {
  if (env.LM_HOME) return env.LM_HOME;
  if (platform === 'win32') return win32.join(env.AppData ?? win32.join(home, 'AppData', 'Roaming'), 'lm-cli');
  if (platform === 'darwin') return posix.join(home, 'Library', 'Application Support', 'lm-cli');
  return posix.join(env.XDG_CONFIG_HOME || posix.join(home, '.config'), 'lm-cli');
}
