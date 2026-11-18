import { describe, expect, it } from 'vitest';
import { configHome } from '../src/home.js';

describe('configHome', () => {
  it('honours LM_HOME', () => {
    expect(configHome({ LM_HOME: '/custom/lm' }, 'darwin', '/Users/a')).toBe('/custom/lm');
  });

  it('uses Application Support on macOS', () => {
    expect(configHome({}, 'darwin', '/Users/a')).toBe('/Users/a/Library/Application Support/lm-cli');
  });

  it('uses XDG_CONFIG_HOME, else ~/.config, on Linux', () => {
    expect(configHome({ XDG_CONFIG_HOME: '/x/cfg' }, 'linux', '/home/a')).toBe('/x/cfg/lm-cli');
    expect(configHome({}, 'linux', '/home/a')).toBe('/home/a/.config/lm-cli');
  });

  it('uses AppData on Windows', () => {
    expect(configHome({ AppData: 'C:\\Users\\a\\AppData\\Roaming' }, 'win32', 'C:\\Users\\a'))
      .toBe('C:\\Users\\a\\AppData\\Roaming\\lm-cli');
  });
});
