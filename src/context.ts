import type { Endpoints } from './auth.js';

/** What a command runs against. Production fills `home` only; tests point base/endpoints at fakes. */
export interface Context {
  home: string;
  base?: string;
  endpoints?: Partial<Endpoints>;
  timeoutMs?: number;
}
