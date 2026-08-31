// Ambient global for the build-time version constant injected by bun's
// bundler via `--define __B3TTY_VERSION__=...` in the Makefile's `client`
// target. No import/export statements here, so TS treats this as a global
// script file (not a module) — the declaration is visible everywhere under
// src/client without an import.
declare const __B3TTY_VERSION__: string;
