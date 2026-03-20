# b3tty

A browser-based terminal emulator.

## Description

b3tty is a terminal emulator accessible entirely from your web browser. It is built using xterm.js which provides the terminal look and feel using Javascript and CSS. A small web server acts as a proxy between a psuedo terminal and the browser, which communicates over web sockets.

The terminal appearance and server can be configured with a configuration yaml file or command-line flags. Use the following command to display available server and terminal configuration options:

```bash
b3tty start --help
```

### Architecture

b3tty uses a client/server model to enable the connection from a web browser to a pseudo terminal. When the server is started, a url where b3tty can be accessed from a web browser is displayed. When the url is visited through a web browser, the server renders an HTML page containing a JSON configuration object (`window.B3TTY`) with the terminal settings, then loads the frontend JavaScript bundle. The frontend determines the width of the browser window to know how many columns to use, then sends that size to the server and waits for confirmation before opening a WebSocket connection. The server then forks a new pseudo terminal process sized to those dimensions. All keyboard input is forwarded over the WebSocket to the pseudo terminal, and any output from the pseudo terminal is sent back and displayed on the page.

When the server closes the WebSocket connection, a modal dialog is displayed in the browser informing the user that the connection has been closed. The terminal cursor is also hidden at this point. Dismissing the modal by clicking OK restores the page to its normal state.

### A word on security

Because b3tty is opening a connection from a web browser to a new psuedo terminal proccess as the user of b3tty's parent process, it's important to ensure the connection and access to the server are secure. For this reason, b3tty features several security features.

#### Origin check

The server will only allow connections from localhost or 127.0.0.1 as part of an origin check. This is to prevent opening a remote connection to a b3tty server. If access to remote machine is needed, run betty locally then SSH into the remove machine from b3tty.

#### Access token

By default, when the server starts, the url with a token of 24 randomly generated characters is provided and must be provided to access the b3tty client in the browser. This is to prevent a user without access to the terminal session where b3tty was started from guessing the url. This behavior can be disabled by passing the `--no-auth` flag at start up or setting the `server.no-auth: true` property in the b3tty config.

Each failed token validation incurs an exponential backoff delay before the 403 response is sent: 1s after the first failure, doubling on each subsequent attempt up to a maximum of 30s. The counter resets when a valid token is presented. Backoff is skipped entirely when `--no-auth` is set.

#### Content Security Policy

The server sets a `Content-Security-Policy` header on every page response. Scripts are restricted to same-origin files and a single per-request nonce used for the inline configuration block. `'wasm-unsafe-eval'` is also permitted to support xterm.js's internal use of WebAssembly. Framing by other pages is blocked via `frame-ancestors 'none'`.

#### CSRF protection

The `POST /size` endpoint (used by the frontend to communicate terminal dimensions before opening the WebSocket) is protected against cross-site request forgery. Requests that carry a `Sec-Fetch-Site` header with a value other than `same-origin` are rejected. This header is set automatically by browsers and cannot be overridden by page scripts.

#### TLS

The connection between the client and server can be secured over TLS. Using TLS will change the protocol from http and ws to https and wss as well as change the default port from 8080 to 8443. TLS can be enabled by passing the `--tls`, `--cert-file`, and `--key-file` flags on start up or by setting the `server.tls: true`, `server.cert-file: <file path>`, and `server.key-file: <file path>` properties in the b3tty config.

## Installation

**NOTE: b3tty is not compatible with Windows.**

### Requirements

* A machine running Linux or MacOS.
* Your favorite web browser.
* git and familiarity with using git.
* Go version 1.25 or higher.
* [bun](https://bun.sh) — used to bundle the frontend TypeScript (`src/client/terminal.ts` → `src/assets/terminal.min.js`)

### Steps

1. Open a new terminal window.
2. b3tty does not ship as a binary. To install b3tty, clone the repo at [https://github.com/cmmorrow/b3tty.git](https://github.com/cmmorrow/b3tty.git).
3. In the root b3tty directory, run `make build` to build b3tty for your system. This will first bundle the frontend JavaScript with bun, then compile the Go binary.
   * To format frontend source files before building, run `make format`. To check formatting without modifying files (e.g. in CI), run `make format-check`.
4. Make the b3tty binary executable with `chmod u+x b3tty`.
5. Either copy the b3tty binary to a directory in your `$PATH` such as /usr/local/bin, or create a symlink to the betty executable that is in your `$PATH`.

## Usage

To start using b3tty, first make sure the executable works by running `b3tty --version`. To start the server with default options, run `b3tty start`. To see the available command-line options when starting the server, run `b3tty start --help`.

When the b3tty server is started, the output should look something like the output below:

```bash
> b3tty start
2024/10/14 00:49:12 [INFO ] http server started on http://localhost:8080/?token=2uzc8uFR7o5yUDy9
```

Log output uses level prefixes to make it easier to identify the nature of each message:

| Level     | Color      | Meaning                                          |
|-----------|------------|--------------------------------------------------|
| `[INFO ]` | Cyan       | Normal operational messages                      |
| `[WARN ]` | Yellow     | Rejected requests, recoverable conditions        |
| `[ERROR]` | Red        | Handler errors that did not stop the server      |
| `[FATAL]` | Bold red   | Unrecoverable errors — server exits immediately  |
| `[DEBUG]` | Magenta    | Verbose diagnostics, only shown with `--debug`   |

Colors are shown when output is an interactive terminal and suppressed when piped or redirected.

## Configuration

b3tty can be configured via a yaml file specified on startup with the command `b3tty start --config <file path>`. While basic server settings can be set via command-line flags, terminal appearance settings (`--rows`, `--columns`, `--cursor-blink`, `--font-family`, `--font-size`) are deprecated as flags and should be set in the config file instead — a warning is printed at startup if any of these deprecated flags are used. Themes and profiles can only be specified in the config file. The config file is a yaml file and b3tty isn't picky about the file name or path, however, it's recommended to name the file b3tty.yaml and place it in ~/.config/b3tty.

When a config file is provided, b3tty validates it on startup before the server starts. Any unknown keys or fields with the wrong data type are reported with the line number where the problem occurs, and the server will not start until the config file is corrected. An example config yaml file can be seen below:

```yaml
server:
  tls: true
  cert-file: "/path/to/cert/file"
  key-file: "/path/to/key/file"
  no-auth: false
  no-browser: false
terminal:
  font-family: "MesloLGS Nerd Font Mono"
  font-size: 16
  cursor-blink: false
  rows: 30
profiles:
  projects:
    working-directory: "~/projects"
    title: "Project Development"
    shell: "/bin/fish"
theme: "my-theme"
themes:
  my-theme:
    black: "#14181d"
    bright-black: "#404040"
    red: "#eb5a4b"
    bright-red: "#ee837b"
    green: "#c6d173"
    bright-green: "#dff06c"
    yellow: "#e6ce6c"
    bright-yellow: "#fdf699"
    blue: "#5998db"
    bright-blue: "#8ccdfa"
    magenta: "#e68c8c"
    bright-magenta: "#f3b7b9"
    cyan: "#b2e7d4"
    bright-cyan: "#b2e7d4"
    white: "#fefefe"
    bright-white: "#feffff"
    foreground: "#dbdbdb"
    background: "#15191e"
    cursor: "#dbdbdb"
    cursor-accent: "#15191e"
    selection-foreground: "#000000"
    selection-background: "#bad5fb"
```

## Themes

b3tty allows the look and feel of the browser-based terminal to be customized in the b3tty config file. Themes set the colors used by the terminal representation in the browser. Multiple themes can be defined in the config file but only one theme can be used when the b3tty server is started.

Each color value in a theme must be either a 3- or 6-digit CSS hex color (e.g. `#fff` or `#14181d`) or a letters-only CSS named color (e.g. `red` or `cornflowerblue`). Invalid color values are reported at startup and the server will not start until they are corrected.

In addition to the standard terminal palette colors, themes support two cursor-specific keys:

| Key | Description |
|-----|-------------|
| `cursor` | Color of the terminal cursor |
| `cursor-accent` | Color of the character beneath the cursor |

## Profiles

Profiles are used to set the default terminal behavior when navigating to the b3tty url. Profiles allow the working directory and shell to be used to be set when the pseudo terminal is started by the server. The title of the browser tab can also be set to make different profiles easier to distinguish from one another.

Unlike server, terminal, and theme settings, different profiles can be used by different browser tabs (or browser windows) when connecting to the b3tty server. To use a profile defined in the b3tty config file, add the `profile=` query parameter to the end of the b3tty url where the value is the name of the profile to use.

When a non-default profile is active, the profile's name (its key in the config file) is displayed in a small label below the terminal in the browser. The label uses the configured font family, font size, and theme foreground and background colors. The label is hidden when using the default profile.

When more than one profile is configured, the server lists them on startup with their URL, shell, and working directory:

```
2024/10/14 00:49:12 [INFO ] Configured profiles:
2024/10/14 00:49:12 [INFO ]   projects    http://localhost:8080/?token=2uzc8uFR7o5yUDy9&profile=projects    (shell: /bin/fish | dir: ~/projects)
2024/10/14 00:49:12 [INFO ]   work        http://localhost:8080/?token=2uzc8uFR7o5yUDy9&profile=work        (shell: /bin/zsh  | dir: ~/work)
```

Profile names are sorted alphabetically and aligned for readability.

## Debug mode

Passing `--debug` to `b3tty start` enables verbose diagnostic output:

```bash
b3tty start --debug
```

On the server side, additional `[DEBUG]` log lines are printed covering startup configuration, incoming request metadata, PTY dimensions, resize events, and WebSocket lifecycle events.

On the browser side, debug mode activates keypress round-trip timing. After each keypress, the time from when the input is sent to the server until xterm.js has finished rendering the PTY response is printed to the browser console:

```
[b3tty] keypress round-trip: 4.23ms
```

Debug mode has no effect on normal terminal operation and is intended for development and performance investigation only.

## Contributing

Pull requests are welcome. The following checks run automatically on every PR and must pass before merging:

- **Test** — `make test` runs the full Go and bun test suites.
- **Format** — `make format-check` verifies that all frontend TypeScript source files are formatted with prettier. Run `make format` locally to fix any formatting issues before pushing.

When a PR that updates the `VERSION` file is merged into `main`, a git tag matching the new version number is created automatically.
