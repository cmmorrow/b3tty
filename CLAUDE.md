# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
make client         # bundles and minifies src/client/terminal.mjs → src/assets/terminal.min.js (requires bun)
make build          # runs make client, then builds ./b3tty binary (reads version from VERSION file)
make build-linux    # cross-compile for linux/amd64
make build-mac      # cross-compile for darwin/amd64

# Test
make test           # runs all tests with verbose output
go test -v ./src -run TestFunctionName  # run a single test

# Lint / format
make format         # format client JS/TS source files with prettier (writes in place)
make format-check   # check formatting without writing (non-zero exit if files differ; useful for CI)

# Other
make clean          # remove binary and build artifacts
make tidy           # go mod tidy
```

## Architecture

b3tty is a browser-based terminal emulator. It runs a Go HTTP server that bridges a pseudo-terminal (pty) to a browser via WebSockets, with xterm.js rendering the terminal in the browser.

**Package layout:**
- `main.go` — entry point; calls `cmd.Execute()`
- `cmd/` — CLI layer using Cobra + Viper
  - `root.go` — root command; loads config from `~/.config/b3tty/conf.yaml` (or `/etc/b3tty/`); populates `profiles` map
  - `start.go` — `b3tty start` subcommand; runs config, theme, and port validation, constructs a `src.TerminalServer` directly, then calls `src.Serve()`
  - `config.go` — typed YAML config structs and `validateConfig`; decodes the config file with `KnownFields(true)` to reject unknown keys and wrong types; intentionally separate from `src/models.go` to avoid coupling the runtime model to the config file shape
- `src/` — core server logic
  - `serve.go` — `TerminalServer` struct (all fields exported), `Serve(*TerminalServer, bool, bool)`, `GetCSPHeaders()`, `buildUIUrl`, `logProfileURLs`, `assets` embed; `TerminalServer.ConfigFile` is set by `cmd/start.go` from `viper.ConfigFileUsed()` and passed to all config-writing functions
  - `display_handler.go` — `displayTermHandler`, `setSizeHandler`, `backgroundHandler`; auth/backoff helpers (`authBackoffDelay`, `backoffBase`, `backoffMax`); `parseSizeParams`, `resolveProfileName`, `buildConfigJSON`; `templates/terminal.tmpl` embed
  - `theme_handler.go` — `themePaletteHandler`, `themeConfigHandler`, `addThemeHandler`, `editThemeHandler`; `builtinThemes` map and all `default_themes/*.json` embeds; `defaultDarkTheme`, `defaultLightTheme`
  - `profile_handler.go` — `profileConfigHandler`, `editProfileHandler`, `deleteProfileHandler`; `nonDefaultProfileNames` helper; all three handlers enforce same-origin CSRF; `editProfileHandler` defaults an empty shell to `DEFAULT_SHELL` so the pty command never receives an empty string
  - `setup_handler.go` — `renderSetupPage`, `themeSelectHandler`, `saveConfigHandler`; `templates/setup.tmpl` and `templates/theme-select.tmpl` embeds
  - `terminal_handler.go` — `terminalHandler`, `upgrader`, `parseResizeMessage`, `formatCommand`
  - `defaults.go` — named constants shared across `cmd/` and `src/`: default values (`DEFAULT_SHELL`, `DEFAULT_URI`, `DEFAULT_ROWS`, `DEFAULT_COLS`, etc.) and server constants (`BUFFER_SIZE`, `TOKEN_LENGTH`, `MAX_REQUEST_BODY_SIZE`)
  - `models.go` — data structs: `Client`, `Server`, `TLS`, `Profile`, `Theme`, `TermConfig`, `CSPHeader`, `CSPHeaders`, `themePaletteResponse`, `themeConfigResponse`, `profileConfigResponse`, `editProfileResponse`; `Theme.toColorMap()` converts the struct back to a hyphenated key map for use with `UpdateThemeInConfig`/`SaveThemeToConfig`, skipping `BackgroundImage`
  - `config.go` — `WriteDefaultConfig`, `buildConfigYAML`, `UpdateThemeInConfig`, `SaveThemeToConfig`, `ReadThemeNames`, `SaveProfileToConfig`, `DeleteProfileFromConfig`; `UpdateThemeInConfig` adds a theme to `themes:` only if absent (used for activation); `SaveThemeToConfig` always overwrites the entry (used for editing) while preserving `background-image` from the existing YAML entry; `ReadThemeNames` reads theme names directly from YAML (not Viper) to preserve exact key casing; `SaveProfileToConfig` upserts a named profile in the `profiles:` section using hyphenated YAML keys; `DeleteProfileFromConfig` removes a named entry from `profiles:`, no-oping if absent
  - `utils.go` — helpers: token generation, browser open, field name conversion, theme color validation, `ValidatePortNumber`
  - `logger.go` — leveled, color-aware logger used throughout `src` and called from `cmd` via `src.Info/Warn/Error/Fatal/Debug`
- `src/client/` — frontend source (TypeScript, bundled by bun → `terminal.min.js`)
  - `terminal.ts` — main module; reads `window.B3TTY` for all config, initializes xterm.js, manages WebSocket lifecycle, handles menu bar and theme-picker events
  - `components.ts` — seven web components: `<b3tty-palette-card>`, `<b3tty-dialog>`, `<b3tty-theme-selector>`, `<b3tty-menu-bar>`, `<b3tty-theme-picker>`, `<b3tty-theme-editor>`, `<b3tty-profile-editor>`; exports TypeScript interfaces and `isB3tty*` type guards; all class definitions guarded by `typeof HTMLElement !== "undefined"` for bun test compatibility; `B3ttyPaletteCardImpl` is defined first so it is available when other components call `document.createElement("b3tty-palette-card")`; shared module-level constants `BUTTON_STYLES` and `PALETTE_CARD_VARS` (CSS template strings) and `fetchPaletteCards(themeNames)` helper (returns `Array<{ card, name, palette }>`) are used by both `B3ttyThemePickerImpl` and `B3ttyThemeEditorImpl`
  - `api.ts` — client-side HTTP helpers for all server endpoints (`/size`, `/theme`, `/theme-config`, `/add-theme`, `/save-config`, `/edit-theme`, `/profile-config`, `/edit-profile`, `/delete-profile`)
  - `validators.ts` — input validation: `isValidHttpProtocol`, `isValidWsProtocol`, `isValidPort`, `isValidUri`
  - `types.ts` — shared TypeScript interfaces: `TermConfig`, `ThemeActivateResponse`, `ClientConfig`, `ThemeConfig`, `ProfileConfig`, `EditProfileResponse`, DOM/socket stubs; `isThemeActivateResponse` and `isEditProfileResponse` runtime type guards; `Window` augmented with `B3TTY?: TermConfig`
  - `package.json` / `bun.lock` — bun project; dependencies are `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-web-links`, `@xterm/addon-image`
- `src/assets/` — static files embedded into the binary at compile time
  - `terminal.min.js` — bundled, minified output of `src/client/terminal.ts` (generated by `make client`)
  - `terminal.css` — page-level styles (layout, bell, profile label)
  - `xterm.6.0.0.min.css` — xterm.js stylesheet (vendored)
- `src/default_themes/` — embedded JSON files for all built-in themes; keys use the hyphenated form expected by `MapToTheme` (e.g. `bright-red`, `selection-background`)
  - Dark themes: `b3tty_dark.json`, `catppuccin-mocha.json`, `solarized-dark.json`, `tokyo-night.json`, `dracula.json`
  - Light themes: `b3tty_light.json`, `solarized-light.json`, `one-light.json`, `gruvbox-light.json`, `catppuccin-latte.json`

**Logging:**
- All log output goes through `src/logger.go` with level prefixes: cyan `[INFO ]`, yellow `[WARN ]`, red `[ERROR]`, bold-red `[FATAL]`, magenta `[DEBUG]`; colors only when stdout is an interactive terminal
- `NewWarnLogger()` is used as `http.Server.ErrorLog` so TLS/HTTP-layer errors carry the `[WARN ]` prefix
- `SetDebug(bool)` gates `Debug`/`Debugf`; `cmd/` calls `src.Info/Warn/Error/Fatalf` directly

**Debug mode:**
- Enabled with `b3tty start --debug`; calls `src.SetDebug(true)` before any other startup work
- Server-side: `[DEBUG]` lines cover startup flags, request metadata, mutex operations, PTY dimensions, resize events, and buffer reads
- Frontend: when `config.debug` is `true`, the browser console logs `[b3tty] keypress round-trip: Xms` for every keypress — measured from just before `socket.send` to the `term.write` completion callback

**Request flow:**
1. `GET /` → `displayTermHandler`; returns 404 for any path other than `/`; validates `?token=`; if `ts.FirstRun` is true renders `setup.tmpl` (first-run flow) and returns; otherwise selects the active profile, builds `TermConfig` (sorted `themeNames`, `allThemeNames`, `profileNames`, `activeTheme`), and renders `terminal.tmpl`
2. Static assets served from `/assets/` via the embedded directory
3. `GET /background` → serves the configured background image file; returns 404 when none is configured
4. `GET /theme?name=<n>` → `themePaletteHandler`; returns `{ bg, fg, selBg, cursor, normal[8], bright[8] }`; checks `ts.Themes` first (user-defined colors take priority), then falls back to `builtinThemes`; color arrays follow ANSI display order (black, red, yellow, green, cyan, blue, magenta, white)
5. `GET /theme-config?name=<n>` → returns `themeConfigResponse` (all `Theme` fields + `hasBackgroundImage`) without side effects; also resolves built-in themes not yet in `ts.Themes`; `POST` additionally activates the theme server-side, updates `ts.Client.Theme` and `ts.ActiveTheme`, and persists to `conf.yaml` via `UpdateThemeInConfig`; requires same-origin `Sec-Fetch-Site`
6. `POST /add-theme` → `addThemeHandler`; accepts `{ "theme": "<name>" }`; resolves from `builtinThemes` then `ts.Themes`; if built-in and not yet in `ts.Themes`, adds it in memory; persists via `UpdateThemeInConfig`; returns `themeConfigResponse` extended with `themeNames []string` (sorted `ts.Themes` keys after addition) so the client can refresh the Themes menu immediately
7. `POST /edit-theme` → `editThemeHandler`; accepts `{ "name": "<name>", "theme": {...Theme color fields...} }`; creates or overwrites a user-defined theme and activates it; preserves `BackgroundImage` from the existing in-memory theme (field is `json:"-"` so it is never in the request body); persists via `SaveThemeToConfig` which also preserves `background-image` in the YAML; returns `themeConfigResponse` with `HasBackgroundImage` set; requires same-origin `Sec-Fetch-Site`
8. `GET /profile-config?name=<n>` → `profileConfigHandler`; returns `profileConfigResponse` (shell, workingDirectory, title, root, commands) from `ts.Profiles`; rejects `name=""` or `name="default"` with 400; 404 if name not found
9. `POST /edit-profile` → `editProfileHandler`; accepts `{ "name": "<name>", "profile": {...profileConfigResponse fields...} }`; creates or overwrites a user-defined profile in `ts.Profiles` and persists via `SaveProfileToConfig`; empty shell defaults to `DEFAULT_SHELL`; empty command strings are discarded; profile is saved but NOT activated (active terminal session is unaffected); returns `editProfileResponse` with sorted non-default `profileNames`; requires same-origin `Sec-Fetch-Site`
10. `POST /delete-profile` → `deleteProfileHandler`; accepts `{ "name": "<name>" }`; removes from `ts.Profiles` and persists via `DeleteProfileFromConfig`; rejects `"default"`; no-ops if name is absent; returns `editProfileResponse` with updated `profileNames`; requires same-origin `Sec-Fetch-Site`
11. `POST /save-config` → `saveConfigHandler`; first-run only (returns 404 when `ts.FirstRun` is false); accepts `{ "theme": "b3tty-dark" | "b3tty-light" | "skip" }`; writes `conf.yaml` and sets `ts.FirstRun = false`; requires same-origin `Sec-Fetch-Site`
12. Page loads `terminal.min.js` as an ES module; module calls `fitAddon.fit()`, `await`s `POST /size` to size the pty, then opens `WS /ws` — size-before-websocket ordering is critical
13. `WS /ws` → `terminalHandler`; forks a pty sized with `OrgCols`/`OrgRows`; two goroutines bridge pty↔WebSocket; a `done` channel closed via `sync.Once` lets the input goroutine distinguish clean PTY exit from unexpected WebSocket error; handler blocks on `<-done` to prevent goroutine leaks

**WebSocket message protocol (`/ws`):**
- **pty output → browser:** binary messages; write deadline of 10s per message so a stalled browser cannot block the pty; decoded client-side with a persistent `TextDecoder("utf-8", { stream: true })` to handle split multi-byte sequences
- **keyboard input → pty:** plain text messages written directly to the pty
- **resize → pty:** JSON `{ type: "resize", cols: N, rows: N }`; server distinguishes from keyboard input by attempting JSON unmarshal; zero-dimension resize requests are discarded with a `[WARN ]` log

**Frontend (`src/client/terminal.ts`):**
- `THEME_KEYS` is the canonical list of every xterm.js `ITheme` property name; `buildTheme` iterates it to copy only truthy values from a `ThemeConfig` into xterm's `ITheme`
- `terminalFactory` always sets `allowTransparency=true` (not conditionally) and overrides `theme.background` to `rgba(0,0,0,0)` when a background image is configured — this ensures runtime theme switching to a background-image theme works without a page reload
- Addon load order matters: `FitAddon` is loaded and `fitAddon.fit()` called immediately after `term.open()`, before `WebLinksAddon` and `ImageAddon`, to minimize layout invalidation when `fitAddon.fit()` forces a synchronous `getBoundingClientRect()`
- `applyThemeStyles(theme, hasBackgroundImage)` is shared by initial load and runtime theme switching; when `hasBackgroundImage` is true it sets a CSS `linear-gradient` tint over `url('/background')` and injects a `<style id="b3tty-bg-style">` to make `.xterm-viewport` transparent
- `main()` uses an `AbortController` whose signal is passed to all `addEventListener` calls; `listenerController.abort()` in `socket.onclose` cleans up all listeners when the session ends
- On WebSocket close: cursor is disabled, then the "Connection closed" dialog is shown only when `event.wasClean` is `false` — clean PTY exits suppress the dialog
- Menu bar custom events: `b3tty-theme-change` → `handleThemeChange` (calls `postThemeConfig`, skips if already active theme); `b3tty-profile-change` → `handleProfileChange` (opens new tab); `b3tty-open-theme-selector` → `picker.open(config.allThemeNames)`; `b3tty-theme-selected` → `handleThemeSelected` (calls `postAddTheme`, refreshes Themes menu if `themeNames` in response); `b3tty-open-theme-editor` → `editor.open(config.themeNames, config.builtinThemeNames)`; `b3tty-theme-edited` → `handleThemeEdited` (applies edited theme to xterm.js and page styles, sets transparent background when `hasBackgroundImage`, refreshes menu bar); `b3tty-open-profile-editor` → `profileEditor.open(editableNames)` (filters out `"default"` before passing); `b3tty-profile-edited` → `handleProfileEdited` (updates `config.profileNames` and rebuilds menu bar; dispatched for both save and delete with `name: null` on delete)
- All three runtime theme-apply paths (`handleThemeChange`, `handleThemeSelected`, `handleThemeEdited`) zero out `builtTheme.background` via `withAlpha(..., 0)` when `hasBackgroundImage` is true, so xterm renders transparently over the background image
- When `columns=0`: window resize → debounced (100ms) `fitAddon.fit()` → `term.onResize` → sends resize JSON over WebSocket; `term.onResize` registered after initial fit to avoid spurious startup resize

**Web components (`src/client/components.ts`):**

`B3ttyPaletteCard` (`<b3tty-palette-card>`, used by `B3ttyThemeSelector`, `B3ttyThemePicker`, and `B3ttyThemeEditor`):
- `setup(value, label, palette)` populates the card with a theme name, display label, and color palette; `readonly value` and `readonly selected` reflect internal state
- When clicked, sets its own `[selected]` attribute and dispatches a composed `b3tty-card-select` CustomEvent with `{ detail: { value: string } }`
- `observedAttributes: ["selected"]` + `attributeChangedCallback` keep the internal radio input in sync when `[selected]` is toggled externally (e.g. by the theme editor's live-preview logic)
- Styled from a parent shadow DOM via CSS custom properties (`--palette-card-padding`, `--palette-card-gap`, `--palette-card-overflow`, `--palette-card-header-bg`, `--palette-card-header-padding`, `--palette-card-header-font-size`, `--palette-card-terminal-gap`, `--palette-card-terminal-shadow`, `--palette-card-terminal-min-width`)
- `isB3ttyPaletteCard(el)` type guard checks for `setup` function and `value`/`selected` properties

`B3ttyDialog` (`<b3tty-dialog>`, used in `terminal.tmpl`):
- Shadow DOM; visibility driven by the `open` attribute; `show(message)` / `hide()` API
- Full-viewport backdrop blocks pointer interaction while open

`B3ttyThemeSelector` (`<b3tty-theme-selector>`, used in `setup.tmpl` first-run page):
- Renders palette preview cards for `b3tty-dark`, `b3tty-light`, and "No theme"; palettes fetched async from `GET /theme`
- On OK: POSTs `{ theme }` to `/save-config` and reloads; palette fetch failure leaves "No theme" available

`B3ttyMenuBar` (`<b3tty-menu-bar>`, always present in `terminal.tmpl`):
- 6px trigger strip at top of viewport; slides open on `mouseenter`, auto-closes after 5s or `pointerdown` outside
- `setup(themeNames, profileNames, colors)` always renders both a "Themes" section ("Select Theme…" and "Edit Theme…" as the first two items) and a "Profiles" section ("Edit Profile…" as the first item, then non-default switchable profiles); both sections are unconditional so their action items are always accessible
- The profiles section filters `"default"` out of the switchable item list; only non-default profiles appear as navigation items below the separator
- Dispatches: `b3tty-menubar-open`, `b3tty-menubar-close`, `b3tty-theme-change`, `b3tty-profile-change`, `b3tty-open-theme-selector`, `b3tty-open-theme-editor`, `b3tty-open-profile-editor`

`B3ttyThemePicker` (`<b3tty-theme-picker>`, hidden overlay in `terminal.tmpl`):
- `open(themeNames)` / `close()` API; narrowed in `terminal.ts` via `isB3ttyThemePicker` type guard
- `open` fetches all palette data in parallel via `fetchPaletteCards`; failed fetches are silently skipped; OK button disabled until a card is selected
- On OK: dispatches `b3tty-theme-selected` (bubbles + composed); Cancel closes without interrupting the PTY session

`B3ttyThemeEditor` (`<b3tty-theme-editor>`, hidden overlay in `terminal.tmpl`):
- `open(themeNames, builtinThemeNames)` / `close()` API; narrowed in `terminal.ts` via `isB3ttyThemeEditor` type guard (tag-name check)
- Left panel: "Create new theme" card (always first) + palette cards for existing user themes fetched via `fetchPaletteCards`; palette cache (`#paletteCache`) populated at open time so live preview can restore originals
- Right panel: theme name input (read-only when editing an existing theme; validated against `builtinThemeNames` to prevent overwriting built-ins), core color fields (background, foreground, cursor, cursorAccent, selectionBackground, selectionForeground), ANSI color pairs (normal + bright for each of 8 colors) with inline color swatch previews
- Live preview: selecting a card loads its colors into the right panel via `GET /theme-config`; each valid input change calls `#buildPreviewPalette()` (merges valid inputs over the cached base palette) and re-calls `setup()` on the selected card; switching cards or closing calls `#restoreSelectedCard()` to revert to the original palette
- On OK: calls `postEditTheme(name, colorData)` → `POST /edit-theme`; dispatches `b3tty-theme-edited` with `{ name, response }`; Cancel closes without saving

`B3ttyProfileEditor` (`<b3tty-profile-editor>`, hidden overlay in `terminal.tmpl`):
- `open(profileNames)` / `close()` API; narrowed in `terminal.ts` via `isB3ttyProfileEditor` type guard (tag-name check); caller passes all profile names — the component itself filters out `"default"`
- Left panel: "Create new profile" card (always first) + plain text cards (no palette) for each non-default profile
- Right panel: Name (required), Shell, Title, Working Directory, Root text inputs; code-editor-style Commands block with a synchronized line-number sidebar and `wrap="off"` textarea (no visual line wrapping — long commands scroll horizontally)
- Name input is read-only when an existing profile card is selected; editable when the create card is selected; `"default"` is blocked as a name with an inline error
- Delete button shown only when an existing profile is selected; hidden on create card
- On OK: filters empty command lines, defaults empty shell to `DEFAULT_SHELL`, calls `postEditProfile(name, profile)` → `POST /edit-profile`; dispatches `b3tty-profile-edited` with `{ name, response }`; profile is saved but NOT activated
- On Delete: calls `postDeleteProfile(name)` → `POST /delete-profile`; dispatches `b3tty-profile-edited` with `{ name: null, response }`
- Cancel closes without saving; both OK and Delete keep the editor open on request failure for retry

**CSS layout (`src/assets/terminal.css`):**
- `#container`: full-viewport flex column with `box-sizing: border-box` padding; `#terminal` uses `flex: 1; min-height: 0` to grow and shrink correctly
- `#profile`: flex item in normal flow (not fixed-position); collapses via `#profile:empty { display: none }` for the default profile; styled via `--b3tty-font-size` and `--b3tty-font-family` CSS custom properties set by `applyPageStyles`

**Security:**
- CSP headers set on every page response via `GetCSPHeaders()`; `displayTermHandler` injects a per-request nonce into `script-src` for the inline `window.B3TTY` assignment; directives include `default-src 'none'`, `'wasm-unsafe-eval'` (required by xterm.js), `style-src 'self' 'unsafe-inline'` (required by xterm.js and background tinting), `frame-ancestors 'none'`
- WebSocket `upgrader` rejects cross-origin upgrades: absent `Origin` is allowed (non-browser clients); browser-sent `Origin` must have `Host` matching `r.Host`
- All mutating handlers (`setSizeHandler`, `themeConfigHandler`, `addThemeHandler`, `editThemeHandler`, `editProfileHandler`, `deleteProfileHandler`, `saveConfigHandler`) enforce same-origin CSRF via `Sec-Fetch-Site: same-origin`; absent header (non-browser) is allowed
- `displayTermHandler` applies exponential backoff on failed token validation: `authBackoffDelay(n)` returns 1s/2s/4s/8s/16s capped at 30s; `TerminalServer.AuthSleep` is injectable for tests; backoff skipped entirely when `NoAuth` is set
- `terminalHandler` discards resize requests where either dimension is zero
- `Theme.BackgroundImage` carries `json:"-"` — the file path is never sent to the browser; the client receives only `HasBackgroundImage bool`

**CI/CD (`.github/workflows/`):**
- `ci.yml` — on every PR to `main`; parallel jobs: `test` (Go + bun tests via `make test`) and `format` (`make format-check`)
- `tag.yml` — on push to `main`; creates a `v`-prefixed git tag when `VERSION` changed

**Key design notes:**
- Version stored in `VERSION` file and injected at build time via `-ldflags`
- `Theme.MapToTheme()` uses reflection to map hyphenated YAML keys (e.g. `bright-red`) to struct fields (e.g. `BrightRed`); `Theme.toColorMap()` is the inverse, omitting empty fields and always excluding `BackgroundImage`
- `builtinThemes` (package-level in `theme_handler.go`) is populated at init time from all 10 embedded JSON files; built-in themes are NOT auto-registered in `ts.Themes` — they are only added to `ts.Themes` in memory when selected via `POST /add-theme`; `selection-background` is the correct key name (not `sel-bg`) — the wrong key causes `KnownFields` validation to reject the generated `conf.yaml` on restart; `themePaletteHandler` checks `ts.Themes` first so user-edited colors take priority over built-in defaults for the same name
- `ReadThemeNames` in `config.go` reads theme names directly from YAML rather than via Viper because `viper.GetStringMap` lowercases all keys, which would corrupt mixed-case user theme names in `ts.Themes` and the Themes menu
- `TermConfig` / `NewTermConfig` in `models.go` is the canonical browser config shape; `ThemeNames` is the user-configured theme list (Themes menu), `AllThemeNames` is the union of all built-in and user-defined names (passed to `picker.open()`), `BuiltinThemeNames` is the list of built-in theme names (passed to `editor.open()` so the editor can reject those names); `buildConfigJSON` in `display_handler.go` accepts matching `allThemeNames` and `builtinThemeNames` parameters
- `themeConfigResponse` (in `models.go`) embeds `Theme` and adds `HasBackgroundImage bool`; carries `ThemeNames []string \`json:"themeNames,omitempty"\`` populated by `addThemeHandler` and `editThemeHandler`
- `TerminalServer` has all exported fields; `cmd/start.go` constructs and populates it directly before calling `Serve()`; no package-level globals for client/server/profiles/themes exist in `src`; `ConfigFile` holds the resolved config path from `viper.ConfigFileUsed()` and is passed to all config-writing functions
- `TerminalServer.FirstRun` is set by `cmd/start.go` when no config file is found; `saveConfigHandler` sets it to `false` after writing config so the browser can reload into the normal flow
- Profiles keyed by name; `"default"` always present; selected via `?profile=<name>`; TLS changes default port from 8080 to 8443
- `http.Server` timeouts: `ReadTimeout: 10s`, `WriteTimeout: 10s`, `IdleTimeout: 120s`; gorilla/websocket hijacks the connection on upgrade so WebSocket sessions are unaffected
- `start.go` runs three validation passes before `Serve()`: (1) `validateConfig` with `KnownFields(true)`, (2) `ValidateTheme` (accepts empty string, `#rgb`/`#rrggbb`, or letters-only named color; skips `BackgroundImage`), (3) `ValidatePortNumber` (1–65535)
- `buildConfigYAML` returns `(string, error)` rather than panicking; `WriteDefaultConfig` propagates the error to the handler which responds 500
- `make build` always runs `make client` first so `terminal.min.js` stays in sync with the Go binary
- Profile editor saves profiles without activating them (unlike theme editor which activates on save); the running PTY session is unaffected; the new profile becomes available in the Profiles menu immediately via the `profileNames` array returned in `editProfileResponse`
- `editProfileHandler` defaults an empty shell to `DEFAULT_SHELL` (`"$SHELL"`) because `terminalHandler` runs `exec.Command("/bin/sh", "-c", profile.Shell)` — an empty string would produce `/bin/sh -c ""` which exits immediately
- `profileConfigResponse` and `editProfileResponse` in `models.go` use camelCase JSON keys (`workingDirectory`) while `SaveProfileToConfig` writes hyphenated YAML keys (`working-directory`) to match the `profileConfig` struct validated by `KnownFields(true)` at startup
