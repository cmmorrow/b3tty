# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
make build          # builds ./b3tty binary (reads version from VERSION file)
make build-linux    # cross-compile for linux/amd64
make build-mac      # cross-compile for darwin/amd64

# Test
make test           # runs all tests with verbose output
go test -v ./src -run TestFunctionName  # run a single test

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
  - `start.go` — `b3tty start` subcommand; constructs `src.Client` and `src.Server`, then calls `src.Serve()`
  - `defaults.go` — default constants for shell, working directory, title, root
- `src/` — core server logic
  - `server.go` — HTTP handlers and pty lifecycle; embeds `assets/` and `templates/terminal.tmpl` at compile time
  - `models.go` — data structs: `Client`, `Server`, `TLS`, `Profile`, `Theme`
  - `utils.go` — helpers: token generation, browser open, field name conversion

**Request flow:**
1. Browser `GET /` → `displayTermHandler` renders `terminal.tmpl` with `Client`/`Server`/`Profile.Title`; validates the `?token=` query param
2. Static assets served from `/assets/` via the embedded `assets/` directory
3. After the page loads, JavaScript initializes xterm.js, optionally calls `fitAddon.fit()` to compute cols from the browser window width, then fires a non-blocking `fetch POST /size?cols=N&rows=N` to `setSizeHandler` (which stores `orgCols`/`orgRows`) and immediately opens `WS /ws` — these two are concurrent, creating a potential race where the pty may start before the size is stored
4. `WS /ws` → `terminalHandler` forks a pty using `creack/pty` sized with `orgCols`/`orgRows`, then runs two goroutines bridging pty output → WebSocket and WebSocket input → pty

**WebSocket message protocol (`/ws`):**
- **pty output → browser:** binary messages; raw pty bytes decoded client-side with a persistent `TextDecoder("utf-8", { stream: true })` to handle multi-byte sequences split across message boundaries
- **keyboard input → pty:** text WebSocket messages containing raw input strings; written directly to the pty
- **resize → pty:** text WebSocket messages containing JSON `{ type: "resize", cols: N, rows: N }`; server calls `pty.Setsize()` on the running pty. The server distinguishes resize from keyboard input by attempting JSON unmarshal — non-JSON text (keyboard input) falls through to `ptmx.Write`

**Frontend (`src/templates/terminal.tmpl`):**
- Go HTML template; server-side config values (font, theme colors, TLS, port, etc.) are injected at render time
- xterm.js addons loaded: `FitAddon` (auto-fit cols to browser width when `columns=0`), `WebLinksAddon`, `ImageAddon`
- After terminal opens, browser POSTs to `/size` with actual terminal dimensions, then opens the WebSocket
- When `columns=0`: `window resize` → `fitAddon.fit()` → `term.onResize` → sends JSON resize message over the WebSocket; `term.onResize` is registered after the initial `fitAddon.fit()` so startup does not send a spurious resize

**Key design notes:**
- Version is stored in the `VERSION` file and injected at build time via `-ldflags`
- The `Theme.MapToTheme()` method uses reflection to map hyphenated YAML keys (e.g. `bright-red`) to struct fields (e.g. `BrightRed`)
- Profiles are keyed by name; `"default"` is always present; non-default profiles are selected via `?profile=<name>` query param
- TLS support changes default port from 8080 to 8443 automatically when enabled
