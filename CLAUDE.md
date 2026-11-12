# CLAUDE.md

Guidance for Claude Code when working in this repository.

b3tty is a browser-based terminal emulator: a Go HTTP server bridges a pty to the browser over WebSockets, and xterm.js renders it. The Go side is a Cobra CLI (`cmd/`) plus the server (`src/`); the client is TypeScript bundled by bun (`src/client/`).

## Commands

```bash
make setup          # bun install in src/client (first time)
make client         # bundle src/client → src/dist (gitignored); prerequisite of build/test/test-race
make build          # client + ./b3tty binary (version from VERSION)
make test           # Go tests (./...) + bun tests
make test-race      # Go tests with -race (separate CI job, not part of `make test`)
make format         # prettier, in place (format-check for CI)
make lint           # tsc --noEmit on client TS (there is no Go lint target)
make design-system  # component bundle for the design-system artifact → build/design-system
go test -v ./src -run TestName   # single Go test
```

`src/dist` must exist before Go compiles — the `//go:embed dist` directive requires it — so run `make client` before `go test` on a fresh checkout.

## Layout

- `main.go` → `cmd.Execute()`
- `cmd/` — CLI: `root.go` (config loading, shared state), `start.go` (server), `profile.go`, `settings.go`, `theme.go`. Only `cmd/settings_test.go` has tests.
- `src/` — server, one concern per file:
  - `serve.go` — `TerminalServer`, `Serve`, `newMux`, `apiRoutes`, CSP headers, embeds
  - handlers: `display_handler.go` (`/`, `/background`, auth helpers), `terminal_handler.go` (`/ws`), `theme_handler.go`, `theme_manage_handler.go` (import/copy/delete/reload), `profile_handler.go`, `settings_handler.go` (and WebSocket broadcasts), `setup_handler.go` (first-run page)
  - themes: `theme_catalog.go` (lookup across sources, `LoadThemeState`), `theme_package.go` (`.b3theme` export/import), `plugin_store.go` (installed packages on disk), `theme_ops.go` (operations shared by the handlers and the CLI)
  - `config.go` (typed config + all config-writing functions), `models.go` (data types), `defaults.go` (constants), `utils.go`, `lock.go`, `logger.go`, `asset_compression.go`
  - `templates/` (`terminal.tmpl`, `setup.tmpl`), `default_themes/*.json` (built-in themes), `assets/` (hand-authored static files, committed), `dist/` (bun output, gitignored)
- `src/client/` — `terminal.ts` (entry: xterm setup, WebSocket, event wiring), `api.ts` (all HTTP calls), `types.ts`, `validators.ts`, `design.ts` (design tokens and shared CSS), `command-search.ts`, `components/` (web components), `design-system/` (design-system artifact build)

## Server

**Config.** Viper only *locates* the config file (`~/.config/b3tty/conf.yaml` or `/etc/b3tty/`). Values are read by `src.LoadConfig`, one typed `yaml.v3` decode with `KnownFields(true)`, so unknown keys and wrong types fail with a line number. Precedence (CLI flag > config file > default) works by pre-seeding the `Config` with flag values before decoding, since yaml.v3 leaves absent keys untouched; each setting then needs only a `!startCmd.Flags().Changed(...)` guard. `ConfigKeys` exists only to tell an explicit zero from an absent key. A decode error is a warning for most commands but fatal for `start`.

**Writing config.** Every writer in `config.go` goes through `loadConfigMap`/`writeConfigMap` (preserving other top-level keys) and holds `configFileMu` — which only guards within one process, not a CLI command racing the server. YAML keys are hyphenated (`working-directory`, `selection-background`) and must match the struct `yaml:` tags, or the next start rejects the file. Store structs directly rather than hand-built maps so new fields can't be dropped.

**Shared state.** `TerminalServer` is shared by every request goroutine. `StateMu` guards the fields handlers mutate (`Client`, `Theme`, `Profiles`, `Themes`, `InstalledThemes`, `AvailableThemes`, `DefaultTheme`, `ActiveTheme`, `ProfileName`); hold it only long enough to copy values out — never across template execution, JSON encoding, or file I/O. Fields set before `Serve()` and never changed (`Server`, `ConfigFile`, `Plugins`, `NoBrowser`, `ShowMenubar`, `Version`, …) need no lock.

**Restart-only settings.** Server settings (port, no-auth, no-browser, show-menubar) are persisted by `POST /settings` and `b3tty settings set` but apply only on restart. Terminal settings (fonts, ligatures, rows/columns, auto-resize) are applied live and broadcast to open sessions.

**Adding an API endpoint.** Add it to `apiRoutes()` in `serve.go`, not directly to `newMux`: that wraps it in `requireToken` and `TestNewMuxRequiresTokenOnEveryAPIRoute` covers it. Mutating handlers must also call `requireSameOrigin`. Add the client call to `api.ts` (via `apiFetch`) and the same export to `design-system/mock-api.ts`. Return validation failures as a 400 with a plain-text reason (`http.Error`); the client shows that text.

**WebSocket protocol (`/ws?cols=&rows=&token=`).** pty output → binary messages; keyboard input → text; resize → JSON `{type:"resize",cols,rows}`. Server → browser control messages are JSON `{type:"theme"|"settings"|"themes",…}`, sent by `broadcastTheme`/`broadcastSettings`/`broadcastThemeLists`. Every write to a connection holds that `wsClient`'s mutex.

**Session teardown.** End sessions with `terminateSession()` (kill the child, then close the pty), never a bare `ptmx.Close()`: on macOS closing the master doesn't unblock a concurrent `Read()`, which leaks the goroutine and its `WSClients` entry.

**Lock file.** `~/.config/b3tty/b3tty.lock` (port, pid, token, protocol; mode 0600) is how CLI commands find and authenticate to a running server (`postToRunningServer`), which they notify best-effort after changing config.

## Themes

- Three sources with precedence **built-in > installed > user**. Always look names up through `ResolveTheme` / `(ts).resolveTheme`.
- Built-ins (`default_themes/*.json`) and installed themes are never copied into `ts.Themes` or conf.yaml's `themes:`, which hold user themes only. Only user themes can be edited or exported; any theme can be copied into a new user theme.
- The Themes menu (`available-themes:` / `ts.AvailableThemes`) is a separate ordered list of names from any source. With no `available-themes` key it is seeded from the `themes:` names.
- With no resolvable `theme:`, b3tty falls back to `default-theme:`, then to no theme (xterm.js defaults).
- A user theme gains a `package:` block (its stable UUID and metadata) on first export; config writers carry it over from disk, and it never appears in JSON.
- `.b3theme` packages are untrusted zip input: size/entry limits, entries read by name and never extracted, `DisallowUnknownFields`, colors and image type validated, https-only `repo`. Installed packages live in `~/.config/b3tty/plugins` (`installed.json` + `themes/<sha256>/package.b3theme`); `LoadThemes(true)` re-hashes them (server start, `/reload-themes`), other CLI commands skip that.
- Background images are validated on every request (extension *and* sniffed content type; SVG excluded). `/background?theme=<name>` is per-theme because browsers reuse an already-loaded image for the same URL; the client also adds `v=<hash of path>` for edited paths.

## Client

**Components** (`src/client/components/`, one `<b3tty-*>` element per file; `index.ts` defines all of them and re-exports their interfaces and type guards):

- Only the class and `customElements.define` go inside the file's `typeof HTMLElement !== "undefined"` guard, so the module imports under `bun test` (which has no DOM).
- Because components can't be tested, any component with real state keeps it in a DOM-free `<component>-state.ts` reducer with its own `.test.ts`. The pattern: listeners and server responses dispatch actions through `#dispatch`; the reducer returns the *same object* for an action that doesn't apply (callers use that to decide whether to send a request); `#render()` sets every input, button, and message from state, writing input values only when they differ.
- Static shadow-DOM structure is built once from template strings of hard-coded values; user data is set later via `textContent`, input values, and properties — never interpolated into markup.
- Shared helpers: one owned by a component lives in and is exported from that component's file (e.g. palette-card helpers); one with no owner gets a small module named for its concern (`overlay.ts`, `prompt-layer.ts`, `theme-file-dialog.ts`). No catch-all utils module.
- Styles: every component starts its style block with `${DESIGN_TOKENS}${BASE_STYLES}` from `design.ts` and uses tokens rather than raw values (layout dimensions excepted). Shared CSS belongs in `design.ts`.
- Overlays close only through their own `close()`, which ends with `closeOverlay` so `terminal.ts` refocuses the terminal; dialogs stacked on another overlay (theme import/export) use `closeStackedOverlay`. Dialogs close via their buttons — the command palette (Escape/backdrop) is the exception.
- Overlays are wired outside the menu-bar block in `terminal.ts`, because the menu bar can be disabled and the command palette (Ctrl+Shift+;) must still reach them. Command ids starting `open:` leave focus with the opened overlay.
- When adding a component or preview card, update `index.ts` and the `COMPONENTS` list in `design-system/build.ts`.

**terminal.ts:**
- Keep testable logic in exported pure functions (tested in `terminal.test.ts`).
- All runtime theme changes go through `applyResolvedTheme`. A background image is shown only after an `Image` probe confirms it loads.
- `allowTransparency` is always on so switching to an image theme needs no reload.
- Ligatures are pure CSS (`font-feature-settings` on `.xterm-rows`); no character joiner or addon is needed with the DOM renderer.
- The version comes from bun's `--define __B3TTY_VERSION__` (declared in `global.d.ts`), not from `window.B3TTY`.

## Security

- `terminal.tmpl`/`setup.tmpl` use `html/template`; `ConfigJSON` is deliberately `template.JS`.
- API routes require `Authorization: Bearer <token>`. `/`, `/ws`, and `/background` take `?token=` and check it themselves, `/ws` before upgrading. Only `/assets/` and `/dist/` are public; no-auth mode leaves the token empty.
- Mutating handlers also enforce `Sec-Fetch-Site: same-origin`. A missing header passes, so this stops other websites only — the token is what stops local programs.
- Token comparisons are constant-time; `/` adds exponential backoff on failures. Logged URLs go through `redactedURL`.

## Logging and debug

Use `src.Info/Warn/Error/Fatal/Debug` in `src` and `start.go`; other CLI subcommands use `cmdLog` (no timestamps). `b3tty start --debug` enables `[DEBUG]` server logs and client console timing.

## CI

`ci.yml` runs test, test-race, format-check, and lint on PRs to `main`; `tag.yml` tags `v<VERSION>` on push to `main` when `VERSION` changed.

## Keeping docs current

Update README.md and this file when a change alters behavior they describe. Keep this file at the level above: conventions, invariants, and non-obvious reasons. Don't add per-function or per-field details that the code already shows.
