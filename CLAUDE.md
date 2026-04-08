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
  - `config.go` — typed YAML config structs and `validateConfig`; decodes the config file with `KnownFields(true)` to reject unknown keys and wrong types
- `src/` — core server logic
  - `server.go` — HTTP handlers and pty lifecycle; embeds `assets/`, `templates/terminal.tmpl`, and `templates/setup.tmpl` at compile time; exports `TerminalServer` (all fields exported) and `Serve(*TerminalServer, bool, bool)`; pure helper functions `authBackoffDelay`, `parseSizeParams`, `resolveProfileName`, `parseResizeMessage`, `buildConfigJSON`, and `formatCommand` are extracted for testability
  - `defaults.go` — named constants shared across `cmd/` and `src/`: default values (`DEFAULT_SHELL`, `DEFAULT_URI`, `DEFAULT_ROWS`, `DEFAULT_COLS`, etc.) and server constants (`BUFFER_SIZE`, `TOKEN_LENGTH`, `MAX_REQUEST_BODY_SIZE`)
  - `models.go` — data structs: `Client`, `Server`, `TLS`, `Profile`, `Theme`, `TermConfig`, `CSPHeader`, `CSPHeaders`
  - `utils.go` — helpers: token generation, browser open, field name conversion, theme color validation, `ValidatePortNumber` (range 1–65535)
- `src/client/` — frontend source
  - `terminal.ts` — TypeScript module; imports xterm.js addons from `node_modules`, reads `window.B3TTY` for config, implements all terminal logic; exports `THEME_KEYS`, `getProtocols`, `hexToRgba`, `withAlpha`, `buildTheme`, `buildTermOptions`, `buildSizeUrl`, `buildWsUrl`, `sendResizeMessage`, `handleSocketMessage`, `handleSocketClose`, `buildDebugHooks`, `requireElement`, `terminalFactory`, `initTerm`, `handleThemeChange`, `handleProfileChange`
  - `components.ts` — web components used by the terminal page; exports `B3ttyDialog` and `B3ttyMenuBar` interfaces and `MenuBarColors` type; also exports `isB3ttyDialog(el)` and `isB3ttyMenuBar(el)` runtime type guards used by `terminal.ts` to safely narrow custom element types without `as unknown as` casts; conditionally defines `B3ttyDialogImpl`, `B3ttyThemeSelectorImpl`, and `B3ttyMenuBarImpl` (all guarded by `typeof HTMLElement !== "undefined"` so the module is safely importable in the bun test environment); `B3ttyThemeSelectorImpl` fetches palette data via `getThemePalette` from `api.ts` rather than using hard-coded constants, so the two cards are populated asynchronously after the component renders; imports `getThemePalette` and `postSaveConfig` from `api.ts`
  - `api.ts` — client-side HTTP helpers: `postSize(url)` POSTs terminal dimensions to `/size` and throws on non-ok status; `postThemeConfig(name)` POSTs to `/theme-config?name=<name>`, validates the response with `isThemeActivateResponse`, and returns a `ThemeActivateResponse`; `getThemePalette(name)` GETs `/theme?name=<name>` and returns a validated `Palette`; `postSaveConfig(theme)` POSTs to `/save-config` (fire-and-forget, caller handles reload); both `postThemeConfig` and `getThemePalette` throw if the response is non-ok or fails shape validation
  - `validators.ts` — input validation helpers used by `terminal.ts`: `isValidHttpProtocol`, `isValidWsProtocol`, `isValidPort`, `isValidUri`
  - `types.ts` — shared TypeScript interfaces: `TermConfig` (includes `themeNames`, `profileNames`, and `activeTheme` fields), `ThemeActivateResponse` (extends `ThemeConfig` with `hasBackgroundImage: boolean`, used as the response type for `POST /theme-config`), `ClientConfig`, `ThemeConfig`, and DOM/socket stubs used in tests; `isThemeActivateResponse(val)` is an exported runtime type guard that validates the minimum required shape of a parsed JSON response before it is cast
  - `package.json` / `bun.lock` — bun project; dependencies are `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-web-links`, `@xterm/addon-image`
- `src/assets/` — static files embedded into the binary at compile time
  - `terminal.min.js` — bundled, minified output of `src/client/terminal.ts` (generated by `make client`)
  - `terminal.css` — page-level styles (layout, bell, profile label)
  - `xterm.6.0.0.min.css` — xterm.js stylesheet (vendored)
- `src/default_themes/` — embedded JSON files for the built-in dark and light themes
  - `b3tty_dark.json` — default dark palette; keys use the hyphenated form expected by `MapToTheme` (e.g. `bright-red`, `selection-background`)
  - `b3tty_light.json` — default light palette; same key format
- `src/logger.go` — leveled, color-aware logger used throughout the `src` package and called from `cmd` via `src.Info/Warn/Error/Fatal/Debug`

**Logging:**
- All log output goes through `src/logger.go` which wraps the standard `log` package with level prefixes: cyan `[INFO ]`, yellow `[WARN ]`, red `[ERROR]`, bold-red `[FATAL]`, magenta `[DEBUG]`
- Colors are emitted only when stdout is an interactive terminal; piped/redirected output receives plain text prefixes
- `Bold(s)` and `BoldGreen(s)` helpers apply inline emphasis (used for the server URL and profile names respectively); both are no-ops when color is disabled
- `NewWarnLogger()` returns a `*log.Logger` whose writer funnels output through `Warnf`, used as `http.Server.ErrorLog` so internal HTTP/TLS messages (e.g. TLS handshake errors) carry the `[WARN ]` prefix with the timestamp in the correct position
- `SetDebug(bool)` / `debugEnabled` gate the `Debug`/`Debugf` helpers; when false these functions are no-ops
- `cmd/` calls `src.Info/Warn/Error/Fatalf` directly; no `log` or `fmt.Fprintf(os.Stderr, ...)` calls remain in the `cmd` package

**Debug mode:**
- Enabled with `b3tty start --debug`; calls `src.SetDebug(true)` before any other startup work
- Server-side: `[DEBUG]` lines are emitted throughout `Serve`, `setSizeHandler`, `displayTermHandler`, and `terminalHandler` covering startup flags, request metadata, mutex operations, PTY dimensions, resize events, and buffer reads
- Frontend: when `config.debug` is `true` (propagated from server via `window.B3TTY`), the browser console logs `[b3tty] keypress round-trip: Xms` for every keypress — measured from just before `socket.send` to the `term.write` completion callback (i.e. when xterm.js has finished rendering the PTY response)

**Request flow:**
1. Browser `GET /` → `displayTermHandler` returns 404 immediately for any path other than `/` (prevents browser-initiated probes such as favicon requests from triggering auth logic). For `/`, it validates `?token=`; if `ts.FirstRun` is true it calls `renderSetupPage` (renders `setup.tmpl` with the `<b3tty-theme-selector>` component) and returns without starting a terminal. On normal (non-first-run) requests, it selects the active profile, builds a `TermConfig` from `ts.Client`/`ts.Server` (including sorted `themeNames`, `profileNames`, and `activeTheme`), serializes it to JSON, and renders `terminal.tmpl` with `ConfigJSON`, `Profile.Title`, `ProfileName`, and `Nonce`
2. Static assets served from `/assets/` via the embedded `assets/` directory
3. `GET /background` → `backgroundHandler` serves the configured background image file using `http.ServeFile`; returns 404 when no background image is configured or the file is not found
4. `GET /theme?name=<dark|light>` → `themePaletteHandler` returns a `themePaletteResponse` JSON object shaped for the `B3ttyThemeSelector` component: `bg`, `fg`, `selBg` (from the `selection-background` key), `cursor`, `normal` (8-element array in ANSI display order), and `bright` (8-element array in ANSI display order); returns 405 for non-GET methods and 400 for unknown theme names; both error cases are logged at `[WARN ]`; color arrays follow the order: black, red, yellow, green, cyan, blue, magenta, white
5. `GET /theme-config?name=<themename>` → `themeConfigHandler` returns a `themeConfigResponse` (all `Theme` color fields plus `hasBackgroundImage bool`) without side effects; `POST /theme-config?name=<themename>` additionally activates the named theme server-side (updates `ts.Client.Theme` and `ts.ActiveTheme`) so that `/background` immediately serves the correct image and subsequent page loads receive the new theme; POST requires a same-origin `Sec-Fetch-Site` header; returns 400 when the `name` parameter is missing, 404 for unknown theme names
6. `POST /save-config` → `saveConfigHandler` handles the first-run theme selection; only reachable when `ts.FirstRun` is true (returns 404 otherwise); accepts a JSON body `{ "theme": "dark" | "light" | "skip" }`; for `dark`/`light` writes a default `conf.yaml` via `WriteDefaultConfig` and applies the selected theme colors to `ts.Client.Theme`; for `skip` skips file writing; sets `ts.FirstRun = false` and responds 200 so the browser can reload into the normal terminal flow; enforces same-origin CSRF check and `MAX_REQUEST_BODY_SIZE`-byte body limit
7. The page injects `window.B3TTY = <JSON>` then loads `terminal.min.js` as an ES module; the module reads `window.B3TTY`, initializes xterm.js, optionally calls `fitAddon.fit()` to compute cols from the browser window width, then `await`s a `fetch POST /size?cols=N&rows=N` to `setSizeHandler` (which stores `OrgCols`/`OrgRows`); only after that response resolves does it open `WS /ws`, guaranteeing the pty is sized correctly on first connection
8. `WS /ws` → `terminalHandler` forks a pty using `creack/pty` sized with `OrgCols`/`OrgRows`; the profile's shell is launched as `exec.Command("/bin/sh", "-c", profile.Shell)` with `profile.ApplyToCommand(c)` applied before starting; then runs two goroutines bridging pty output → WebSocket and WebSocket input → pty; a `done` channel closed exactly once via `sync.Once` (`signalDone`) lets the input goroutine distinguish a clean PTY-initiated shutdown from an unexpected WebSocket error; after the goroutines start, if the profile has `Commands`, the handler sleeps 1s then writes each command to the pty via `formatCommand` with 200ms between commands; the handler blocks on `<-done` until the PTY session closes (signaled by the output goroutine), then returns — this prevents a goroutine leak that would otherwise accumulate one blocked goroutine per session

**WebSocket message protocol (`/ws`):**
- **pty output → browser:** binary messages; `ws.SetWriteDeadline(time.Now().Add(10 * time.Second))` is called before each `ws.WriteMessage` call in the output goroutine so a stalled browser cannot block the pty indefinitely; raw pty bytes decoded client-side with a persistent `TextDecoder("utf-8", { stream: true })` to handle multi-byte sequences split across message boundaries
- **keyboard input → pty:** text WebSocket messages containing raw input strings; written directly to the pty
- **resize → pty:** text WebSocket messages containing JSON `{ type: "resize", cols: N, rows: N }`; server calls `pty.Setsize()` on the running pty. The server distinguishes resize from keyboard input by attempting JSON unmarshal — non-JSON text (keyboard input) falls through to `ptmx.Write`

**Frontend (`src/client/terminal.ts` → `src/assets/terminal.min.js`):**
- `terminal.tmpl` is a thin HTML shell; it injects `window.B3TTY` (a `TermConfig` JSON object) and loads `terminal.min.js` as `<script type="module">`
- `terminal.ts` reads `window.B3TTY` for all config (TLS, font, theme, URI, port, rows/cols); no Go template syntax in the TS source
- `THEME_KEYS` is an exported `const` array of every xterm.js theme property name (`foreground`, `background`, `cursor`, `cursorAccent`, the 16 ANSI colors, `selectionForeground`, `selectionBackground`); used by `buildTheme` to copy only defined (truthy) values from a `ThemeConfig` into xterm's `ITheme`
- `getProtocols(tls)` returns `{ wsProtocol, httpProto }` ("wss"/"https" or "ws"/"http"); used in `main()` to derive both the WebSocket and fetch URLs from the single `config.tls` boolean
- `hexToRgba(hex, alpha)` converts a CSS hex color (`#rgb` or `#rrggbb`) to an `rgba()` string; normalizes 3-digit to 6-digit form before parsing; falls back to `rgba(0, 0, 0, alpha)` for non-hex input
- `withAlpha(color, alpha)` returns a semi-transparent version of a color: hex colors are passed through `hexToRgba`; named CSS colors fall back to `rgba(0, 0, 0, alpha)` since their RGB values are not known at runtime
- `buildTheme(themeConfig)` iterates `THEME_KEYS` and copies every truthy value from a `ThemeConfig` or `ThemeActivateResponse` into an `ITheme` object; keys absent or falsy in the config are omitted so xterm.js uses its own defaults
- `buildTermOptions(config, theme, allowTransparency)` builds the xterm.js `ITerminalOptions` object: always sets `cursorBlink`, `fontFamily` (with fallback stack), and `fontSize`; sets `rows`/`cols` only when non-zero; sets `theme` only when at least one key is present; sets `allowTransparency` only when the parameter is true
- `sendResizeMessage(socket, cols, rows)` sends a `{ type: "resize", cols, rows }` JSON message over the WebSocket only when `socket.readyState === 1` (OPEN), guarding against sends on a closing or closed socket
- xterm.js addons bundled by bun: `FitAddon` (auto-fit cols to browser width when `columns=0`), `WebLinksAddon`, `ImageAddon`
- After terminal opens, the module `await`s `postSize(sizeUrl)` (from `api.ts`) wrapped in try/catch — a failure is logged via `console.warn` but does not abort the connection; then opens the WebSocket with `socket.binaryType = "arraybuffer"` so PTY output arrives as `ArrayBuffer` rather than `Blob` — the await ensures the pty is always started with the correct size
- When `columns=0`: `window resize` → debounced (100ms) `fitAddon.fit()` → `term.onResize` → sends JSON resize message over the WebSocket; `term.onResize` is registered after the initial `fitAddon.fit()` so startup does not send a spurious resize; the debounce uses a `ReturnType<typeof setTimeout>` timer variable scoped inside `main()` so rapid resize events during window dragging collapse into a single `fit()` call
- Addon load order in `main()` matters for startup latency: `FitAddon` is loaded and `fitAddon.fit()` is called immediately after `term.open()`, before `WebLinksAddon` and `ImageAddon` are loaded; this minimizes accumulated layout invalidation at the time `fitAddon.fit()` forces a synchronous `getBoundingClientRect()` read
- `terminalFactory(config)` constructs the xterm.js Terminal: builds the theme from `config.theme`, overrides `theme.background` to fully transparent (`rgba(0,0,0,0)`) when a background image is configured (so the canvas does not add a second color layer over the body tint), then calls `buildTermOptions` with `allowTransparency=true` always (not conditionally) so that switching to a background-image theme at runtime works without a page reload
- `buildDebugHooks(debug)` encapsulates keypress round-trip timing: returns `{ onBeforeSend, writeCallback }` when `debug` is true, or `{}` when false; `keypressTime` is held in the closure so both hooks share state without it leaking into `main()`
- `applyThemeStyles(theme, hasBackgroundImage)` applies background and profile label styles for a given theme; shared by `applyPageStyles` (initial load) and `handleThemeChange` (runtime switching); when `hasBackgroundImage` is true, sets the body background to a CSS `linear-gradient` tint stacked over `url('/background')`, injects or updates a `<style id="b3tty-bg-style">` element to make `.xterm-viewport` transparent, and clears the container background; when false, clears the body background and style element and sets `#container`'s inline `background` to the theme background color; also applies `foreground`/`background` to the profile label when it has text content
- `applyPageStyles(config)` handles all config-driven DOM styling: sets `--b3tty-font-size` and `--b3tty-font-family` CSS custom properties, then delegates background and profile label styling to `applyThemeStyles`; `--b3tty-font-size` and `--b3tty-font-family` use `px` units and CSS-quoted font-family so multi-word names like `"Fira Code"` work correctly; `terminal.css` references them via `var()` on `#profile`
- `requireElement(id)` replaces `document.getElementById(id)!` throughout `main()`: it throws a descriptive error if the element is absent rather than producing a cryptic null-dereference deep in a handler
- `terminal.ts` imports `components.ts` for side effects (registers `<b3tty-dialog>`, `<b3tty-theme-selector>`, and `<b3tty-menu-bar>`) and imports `B3ttyDialog`, `B3ttyMenuBar` interfaces and `isB3ttyDialog`, `isB3ttyMenuBar` type guards; elements retrieved by id are narrowed via the type guards (throwing a clear error on mismatch) rather than cast with `as unknown as`
- `main()` creates an `AbortController` whose `signal` is passed to every `addEventListener` call for menu bar and window resize events; `listenerController.abort()` is called in `socket.onclose` to remove all listeners when the session ends, preventing leaks on hot-reload or component reuse
- `main()` checks for a `#menubar` element; if present and `themeNames` or `profileNames` are non-empty, calls `menuBar.setup(themeNames, profileNames, { bg: theme.foreground, fg: theme.background })` — note the colors are inverted (the menu bar uses the terminal's foreground color as its background and vice versa); listens for `b3tty-menubar-open` and `b3tty-menubar-close` events and calls `requestAnimationFrame(() => fitAddon?.fit())` on each so the terminal resizes when the menu bar expands or collapses
- `disableCursor(term)` permanently hides the terminal cursor: sets `cursorBlink` to `false`, `cursorInactiveStyle` to `"none"`, calls `term.blur()`, and adds a `focus` listener to `term.textarea` that immediately re-blurs to prevent the cursor reappearing on click
- On WebSocket close: `disableCursor(term)` is called, then `handleSocketClose(term, alertFn, event.wasClean)` — the "Connection closed" dialog is shown only when `event.wasClean` is `false` (unexpected drop); clean closes (server-initiated PTY exit or browser navigation) suppress the dialog
- When the menu bar is present, `main()` listens for two custom events and delegates each to a standalone exported function: `handleThemeChange(e, term, menuBar, activeTheme)` and `handleProfileChange(e)`
  - `handleThemeChange` accepts a `{ current: string }` ref (`activeTheme`) initialized from `config.activeTheme` (the lowercased active theme name sent by the server); if the selected theme name matches `activeTheme.current` the handler returns immediately without making a network request; otherwise calls `postThemeConfig(name)` from `api.ts` (which handles the POST, ok-check, and `isThemeActivateResponse` type-guard internally) inside a try/catch — any failure returns early; on success applies the returned theme to the terminal and page styles via `applyThemeStyles`, updates the menu bar colors, and sets `activeTheme.current` to the new name
  - `handleProfileChange` uses `URLSearchParams` to clone all existing query parameters, sets (or replaces) the `profile` key, then calls `window.open("/?<params>", "_blank")` — existing parameters such as `token` are preserved in the new tab

**Web components (`src/client/components.ts`):**

`B3ttyDialog` (`<b3tty-dialog>`):
- Used in `terminal.tmpl` as `<b3tty-dialog id="dialog">`
- Uses Shadow DOM for style encapsulation; visibility is driven by the presence of the `open` attribute (`:host` is `display: none` by default, `display: block` when `[open]`)
- `show(message: string)` — sets the `<p>` text content and adds the `open` attribute; `hide()` removes it
- The backdrop (`position: fixed; inset: 0`) covers the full viewport at `z-index: 10000`, blocking all pointer interaction with the page beneath
- The `p` and `button` elements inside the shadow DOM use `font-family: sans-serif` (hardcoded); the dialog intentionally uses a neutral system font rather than the terminal's configured font family

`B3ttyThemeSelector` (`<b3tty-theme-selector>`):
- Used in `setup.tmpl` (the first-run setup page) as `<b3tty-theme-selector>`
- Renders a modal with palette preview cards for Dark, Light, and a "No theme" (skip) option; the dark and light palettes are fetched asynchronously from `GET /theme?name=dark` and `GET /theme?name=light` after the component renders
- The OK button is disabled until the user selects a radio option; on click it POSTs `{ theme: "dark" | "light" | "skip" }` to `/save-config` and calls `window.location.reload()` to transition into the normal terminal flow; the `Promise.all([fetchPalette("dark"), fetchPalette("light")])` call includes a `.catch()` handler so a palette fetch failure leaves the "No theme" option available rather than breaking the UI

`B3ttyMenuBar` (`<b3tty-menu-bar>`):
- Used in `terminal.tmpl` as `<b3tty-menu-bar id="menubar">` (always present in the DOM; hidden when setup returns no themes/profiles)
- Renders as a 6px tall fixed trigger strip centered at the top of the viewport; on `mouseenter` of the trigger the full 32px menu bar slides into view (sets `[open]` attribute, dispatches `b3tty-menubar-open`) and auto-closes after 5 seconds of inactivity or on `pointerdown` outside the element (dispatches `b3tty-menubar-close`)
- `setup(themeNames, profileNames, colors)` populates "Themes" and "Profiles" dropdown sections and calls `updateColors`; `updateColors(colors)` sets `--menu-bg` and `--menu-fg` CSS custom properties on the shadow host
- Selecting a theme item dispatches `b3tty-theme-change` with `{ detail: { name } }` and closes the dropdown; selecting a profile item dispatches `b3tty-profile-change` and closes the entire menu bar
- All three web component class definitions are guarded by `typeof HTMLElement !== "undefined"` so importing `components.ts` in the bun test environment does not throw a `ReferenceError`; exported interfaces (`B3ttyDialog`, `B3ttyMenuBar`, `MenuBarColors`) are TypeScript interfaces (no runtime values) so they are safely importable anywhere

**CSS layout (`src/assets/terminal.css`):**
- `#container` has `height: 100%; box-sizing: border-box; padding: 4px; overflow: hidden; display: flex; flex-direction: column; gap: 4px` — a full-viewport flex column; `box-sizing: border-box` keeps padding inside the height so nothing overflows
- `#terminal` has `flex: 1; min-height: 0` — grows to fill all remaining space after the profile label; `min-height: 0` is required so a flex child with overflow can shrink below its content size
- `#profile` is a flex item (not fixed-position) with `font-size: var(--b3tty-font-size, 12px); font-family: var(--b3tty-font-family, monospace); padding: 2px 8px; border: none; align-self: flex-start` — sits in normal flow below the terminal; `main()` applies `color` and `background` from the active theme when the element has text content; `#profile:empty { display: none }` collapses the label entirely (reclaiming the gap) for the default profile

**Security:**
- Both `displayTermHandler` and `renderSetupPage` set a `Content-Security-Policy` header via `GetCSPHeaders()`; `displayTermHandler` additionally calls `csp.Get("script-src").Add("nonce-" + nonce)` to inject a per-request nonce (used for the inline `window.B3TTY` assignment); `renderSetupPage` uses the baseline headers without a nonce since the setup page has no inline scripts. Directives: `default-src 'none'`; scripts restricted to same-origin plus the per-request nonce and `'wasm-unsafe-eval'` (required by xterm.js); `style-src 'self' 'unsafe-inline'` (`'unsafe-inline'` is required because xterm.js and the background image tinting inject inline styles); `connect-src 'self'`; `img-src 'self'` (permits the `/background` endpoint); `font-src 'self'`; `frame-ancestors 'none'`; `base-uri 'self'`
- The WebSocket `upgrader` uses a custom `CheckOrigin` function: an absent `Origin` header (non-browser clients) is allowed; a browser-sent `Origin` is parsed and its `Host` must match `r.Host` — cross-origin WebSocket upgrade attempts are rejected with a `false` return, preventing third-party pages from silently opening terminal connections
- `CSPHeader` (in `models.go`) represents a single CSP directive; `Add` and `Set` use pointer receivers and return `*CSPHeader` for chaining — mutations apply in place. `CSPHeaders` stores directives in `map[string]*CSPHeader` (pointer values) so `Get(key)` returns a pointer to the live map entry; mutating the returned pointer is reflected in the final `String()` output
- `setSizeHandler` enforces CSRF protection via the `Sec-Fetch-Site` fetch-metadata header: requests with a value other than `same-origin` are rejected with 403; absent header (non-browser clients) is allowed
- `themeConfigHandler` enforces the same `Sec-Fetch-Site` CSRF check on POST requests
- `saveConfigHandler` enforces the same `Sec-Fetch-Site` CSRF check; it also wraps `r.Body` with `io.LimitReader(r.Body, MAX_REQUEST_BODY_SIZE)` before JSON decoding to cap request body size; it returns 404 when `ts.FirstRun` is false so the endpoint is unreachable after initial setup
- All handlers log a message at `[WARN ]` level whenever a 403 or 405 response is returned, including the method, path, and reason; internal server errors are logged at `[ERROR]`; the `http.Server.ErrorLog` is set to `NewWarnLogger()` so TLS/HTTP-layer errors also appear at `[WARN ]` with the timestamp in the correct position
- `displayTermHandler` applies exponential backoff on consecutive failed token validations: the pure function `authBackoffDelay(n int) time.Duration` returns delays of 1s, 2s, 4s, 8s, 16s, capped at 30s (`backoffBase` and `backoffMax` named constants). The counter resets on a successful auth. Backoff is skipped entirely when auth is disabled (`NoAuth`). The sleep function is injectable via `TerminalServer.AuthSleep` so tests run without real delays. Debug log calls are emitted before acquiring and after releasing `BackoffMu`, never while holding it
- `terminalHandler` receives resize messages of the form `{"type":"resize","cols":N,"rows":N}`; before calling `pty.Setsize`, it validates that both `cols` and `rows` are non-zero — zero-dimension resize requests are logged at `[WARN ]` and discarded
- `buildSizeUrl` and `buildWsUrl` in `terminal.ts` validate their `httpProto`/`wsProtocol`, `uri`, and `port` arguments using helpers from `validators.ts` and throw on invalid input
- `handleSocketMessage` accepts an optional `writeCallback?: () => void` that is passed to `term.write`; the callback fires after xterm.js finishes rendering — used by debug timing to mark the end of the round-trip
- `initTerm` accepts an optional `onBeforeSend?: () => void` hook that fires immediately before `socket.send` — used by debug timing to record the keypress timestamp

**CI/CD (`.github/workflows/`):**
- `ci.yml` — triggered on every pull request targeting `main`; runs two parallel jobs:
  - `test`: sets up Go (version read from `go.mod`) and bun, installs client dependencies, runs `make test` (Go tests + bun tests)
  - `format`: sets up bun, installs client dependencies, runs `make format-check` (exits non-zero if any file differs from prettier's output)
- `tag.yml` — triggered on every push to `main`; uses `fetch-depth: 2` to compare `VERSION` before and after the merge; if the file changed, creates and pushes a `v`-prefixed git tag (e.g. `v0.9.1`) using `github-actions[bot]` as the committer; requires `permissions: contents: write`

**Key design notes:**
- Version is stored in the `VERSION` file and injected at build time via `-ldflags`
- The `Theme.MapToTheme()` method uses reflection to map hyphenated YAML keys (e.g. `bright-red`) to struct fields (e.g. `BrightRed`); `Theme` fields carry JSON tags with camelCase names for direct serialization into `TermConfig`; supported fields include all standard xterm.js `ITheme` colors plus `cursor` (`json:"cursor"`) and `cursorAccent` (`json:"cursorAccent"`) added in addition to the existing palette and selection fields
- `defaultDarkTheme` and `defaultLightTheme` are loaded at startup from embedded JSON files (`src/default_themes/b3tty_dark.json` and `b3tty_light.json`) via `mustUnmarshalTheme`, which panics on invalid JSON since the files are compiled into the binary; keys use the same hyphenated form as YAML theme keys so the maps work directly with `MapToTheme` and `buildConfigYAML`; `selection-background` is the correct key name for the selection color (not `sel-bg`) — using the wrong key would cause `KnownFields` validation to reject the generated `conf.yaml` on restart
- `TermConfig` / `NewTermConfig` in `models.go` is the canonical shape of the config object passed to the browser; it includes a `Debug bool` field (`json:"debug"`) populated from `debugEnabled` so the frontend knows whether to enable timing output; it also includes `ThemeNames []string` (`json:"themeNames"`), `ProfileNames []string` (`json:"profileNames"`), and `ActiveTheme string` (`json:"activeTheme"`) — the lowercased name of the currently active theme, used by the frontend to initialize the deduplication ref so re-selecting the active theme never triggers a network request; `NewTermConfig` accepts `themeNames`, `profileNames`, and `activeTheme` as its final three parameters
- `themePaletteResponse` and `themeConfigResponse` are defined in `models.go` (not `server.go`) alongside the other data types; `themePaletteResponse` includes `Bg`, `Fg`, `SelBg`, `Cursor`, `Normal`, and `Bright` fields; `themeConfigResponse` embeds `Theme` and adds `HasBackgroundImage bool` so the client receives all color fields plus the background-image flag without the server-side file path
- `TerminalServer` in `server.go` has all exported fields (`Client`, `Server`, `Profiles`, `Themes`, `Token`, `OrgCols`, `OrgRows`, `ProfileName`, `ActiveTheme`, `FailedAttempts`, `FirstRun`, `BackoffMu`, `AuthSleep`); `cmd/start.go` constructs and populates a `TerminalServer` value directly before passing it to `Serve()`; no package-level `InitClient`, `InitServer`, `Profiles`, `Themes`, or `ActiveThemeName` globals exist in the `src` package
- `TerminalServer.FirstRun bool` is set by `cmd/start.go` to `true` when no config file is found (first-time run); when `true`, `displayTermHandler` renders `setup.tmpl` instead of the normal terminal; `saveConfigHandler` sets `ts.FirstRun = false` after a successful config write, allowing the browser to reload into the normal flow; `ts.ActiveTheme` is updated by `themeConfigHandler` on each successful POST
- Profiles are keyed by name; `"default"` is always present; non-default profiles are selected via `?profile=<name>` query param
- TLS support changes default port from 8080 to 8443 automatically when enabled
- `http.Server` is configured with `ReadTimeout: 10s`, `WriteTimeout: 10s`, and `IdleTimeout: 120s`; these apply to all non-WebSocket handlers — gorilla/websocket hijacks the connection on upgrade so the WebSocket session is unaffected by these timeouts
- `make build` always rebuilds the JS bundle via `make client` before compiling Go, so the embedded `terminal.min.js` stays in sync
- `start.go` runs three validation passes before `src.Serve()`: (1) `validateConfig` (in `cmd/config.go`) decodes the config file into typed structs with `gopkg.in/yaml.v3` and `KnownFields(true)`, catching unknown keys and wrong field types; (2) `src.ValidateTheme` checks every theme color field against `validateThemeColor`, which accepts an empty string, a 3- or 6-digit CSS hex color (`#rgb` / `#rrggbb`), or a letters-only named color (`ValidateTheme` skips the `BackgroundImage` field by name since it holds a file path, not a color); (3) `src.ValidatePortNumber` confirms the port is in the range 1–65535. All three pass before calling `src.Serve()`; any failure calls `src.Fatalf`
- `Theme.BackgroundImage` carries `json:"-"` so the server-side file path is never serialized into the `window.B3TTY` config object sent to the browser; the frontend receives only `HasBackgroundImage bool` (`json:"backgroundImage"`) — a boolean flag that tells it whether to enable the transparency tinting and fetch `/background`
- The flags `--cursor-blink`, `--font-family`, and `--font-size` have been removed from the CLI entirely; `--rows` and `--columns` remain registered but are hidden from help output via `MarkHidden`. All five settings are still configurable via the YAML config file. No deprecation warning is emitted at runtime.
- `cmd/config.go` contains only validation structs and `validateConfig`; it is intentionally separate from `src/models.go` to avoid coupling the runtime data model to the config file shape
- `Serve()` handles `SIGINT` and `SIGTERM` via `signal.Notify`; on receipt it calls `httpServer.Shutdown(ctx)` with a 30-second context, allowing in-flight HTTP requests to complete before the process exits; the HTTP server runs in a goroutine and any non-`ErrServerClosed` error is fatal
- `buildConfigYAML` in `src/config.go` returns `(string, error)` rather than panicking on YAML marshal failure; `WriteDefaultConfig` propagates this error up to the handler, which responds 500
