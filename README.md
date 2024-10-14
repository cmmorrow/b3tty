# b3tty

A browser-based terminal emulator.

## Description

b3tty is a terminal emulator accessible entirely from your web browser. It is built using xterm.js which provides the terminal look and feel using Javascript and CSS. A small web server acts as a proxy between a psuedo terminal and the browser, which communicates over web sockets.

The terminal appearance and server can be configured with command-line flags or a configuration yaml file. Use the following command to display available server and terminal configuration options:

```bash
b3tty start --help
```

### Architecture

b3tty uses a client/server model to enable the connection from a web browser to a pseudo terminal. When the server is started, a url where where b3tty can be accessed from a web browser is displayed. When the url is visited through a web browser, the page is first served to the browser. The page will then determine the width of the browser window to know how many columns to use when starting the pseudo terminal. Next, the server will fork a new pseudo terminal process for b3tty to use. Finally, the page will establish a websocket connection to the server which will forward all writes to the pseudo terminal. Any data returned from the pseudo terminal is sent back to the page over the websocket connection and displayed on the page.

### A word on security

Because b3tty is opening a connection from a web browser to a new psuedo terminal proccess as the user of b3tty's parent process, it's important to ensure the connection and access to the server are secure. For this reason, b3tty features several security features.

#### Origin check

The server will only allow connections from localhost or 127.0.0.1 as part of an origin check. This is to prevent opening a remote connection to a b3tty server. If access to remote machine is needed, run betty locally then SSH into the remove machine from b3tty.

#### Access token

By default, when the server starts, the url with a token of 16 randomly generated characters is provided and must be provided to access the b3tty client in the browser. This is to prevent a user without access to the terminal session where b3tty was started from guessing the url. This behavior can be disabled by passing the `--no-auth` flag at start up or setting the `server.no-auth: true` property in the b3tty config.

#### TLS

The connection between the client and server can be secured over TLS. Using TLS will change the protocol from http and ws to https and wss as well as change the default port from 8080 to 8443. TLS can be enabled by passing the `--tls`, `--cert-file`, and `--key-file` flags on start up or by setting the `server.tls: true`, `server.cert-file: <file path>`, and `server.key-file: <file path>` properties in the b3tty config.

## Installation

**NOTE: b3tty is not compatible with Windows.**

### Requirements

* A machine running Linux or MacOS.
* Your favorite web browser.
* git and familiarity with using git.
* Go version 1.22 or higher.

### Steps

1. Open a new terminal window.
2. b3tty does not ship as a binary. To install b3tty, clone the repo at [https://github.com/cmmorrow/b3tty.git](https://github.com/cmmorrow/b3tty.git).
3. In the root b3tty directory, run `make build` to build b3tty for your system.
4. Make the b3tty binary executable with `chmod u+x b3tty`.
5. Either copy the b3tty binary to a directory in your `$PATH` such as /usr/local/bin, or create a symlink to the betty executable that is in your `$PATH`.

## Usage

To start using b3tty, first make sure the executable works by running `b3tty --version`. To start the server with default options, run `b3tty start`. To see the available command-line options when starting the server, run `b3tty start --help`.

When the b3tty server is started, the output should look something like the output below:

```bash
> b3tty start
2024/10/14 00:49:12 http server started on http://localhost:8080/?token=2uzc8uFR7o5yUDy9
```

## Configuration

b3tty can be configured via a yaml file specified on startup with the command `b3tty start --config <file path>`. While the server and terminal settings can be configured on start up, some settings such as themes and profiles can only be specified in the config file. The config file is a yaml file and b3tty isn't picky about the file name or path, however, it's recommended to name the file b3tty.yaml and place it in ~/.config/b3tty. An example config yaml file can be seen below:

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
    selection-foreground: "#000000"
    selection-background: "#bad5fb"
```

## Themes

b3tty allows the look and feel of the browser-based terminal to be customized in the b3tty config file. Themes set the colors used by the terminal representation in the browser. Multiple themes can be defined in the config file but only one theme can be used when the b3tty server is started.

## Profiles

Profiles are used to set the default terminal behavior when navigating to the b3tty url. Profiles allow the working directory and shell to be used to be set when the pseudo terminal is started by the server. The title of the browser tab can also be set to make different profiles easier to distinguish from one another.

Unlike server, terminal, and theme settings, different profiles can be used by different browser tabs (or browser windows) when connecting to the b3tty server. To use a profile defined in the b3tty config file, add the `profile=` query parameter to the end of the b3tty url where the value is the name of the profile to use.
