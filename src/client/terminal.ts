import { Terminal } from "@xterm/xterm";
import type { ITerminalInitOnlyOptions, ITerminalOptions, ITheme } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { ImageAddon } from "@xterm/addon-image";
import type {
    TermConfig,
    BellElementLike,
    SocketLike,
    SocketMessageEvent,
    TerminalLike,
    ClientConfig,
    ThemeConfig,
} from "./types.ts";
import { isValidHttpProtocol, isValidWsProtocol, isValidPort, isValidUri } from "./validators.ts";
import "./components.ts";
import type { B3ttyDialog } from "./components.ts";

export const THEME_KEYS = [
    "foreground",
    "background",
    "black",
    "brightBlack",
    "red",
    "brightRed",
    "green",
    "brightGreen",
    "yellow",
    "brightYellow",
    "blue",
    "brightBlue",
    "magenta",
    "brightMagenta",
    "cyan",
    "brightCyan",
    "white",
    "brightWhite",
    "selectionForeground",
    "selectionBackground",
] as const;

/**
 * Returns the WebSocket and HTTP protocol strings based on whether TLS is enabled.
 */
export function getProtocols(tls: boolean): { wsProtocol: string; httpProto: string } {
    return {
        wsProtocol: tls ? "wss" : "ws",
        httpProto: tls ? "https" : "http",
    };
}

/**
 * Extracts defined theme color values from the config's theme object.
 * Only keys present in THEME_KEYS with truthy values are included.
 */
export function buildTheme(themeConfig: ThemeConfig): ITheme {
    const theme: Record<string, string> = {};
    for (const k of THEME_KEYS) {
        const val = themeConfig[k];
        if (val) theme[k] = val;
    }
    return theme as ITheme;
}

/**
 * Builds the xterm.js Terminal options object from the b3tty config and resolved theme.
 */
export function buildTermOptions(config: ClientConfig, theme: ITheme): ITerminalOptions & ITerminalInitOnlyOptions {
    const options: ITerminalOptions & ITerminalInitOnlyOptions = {
        cursorBlink: config.cursorBlink,
        fontFamily: `${config.fontFamily}, Menlo, DejaVu Sans Mono, Ubuntu Mono, Inconsolata, Fira, monospace`,
        fontSize: config.fontSize,
    };
    if (config.rows) options.rows = config.rows;
    if (config.columns) options.cols = config.columns;
    if (Object.keys(theme).length > 0) options.theme = theme;
    return options;
}

/**
 * Builds the URL used to POST the initial terminal size to the server.
 */
export function buildSizeUrl(httpProto: string, uri: string, port: number, cols: number, rows: number): string {
    if (!isValidHttpProtocol(httpProto)) throw new Error(`Invalid HTTP protocol: "${httpProto}"`);
    if (!isValidUri(uri)) throw new Error(`Invalid URI: "${uri}"`);
    if (!isValidPort(port)) throw new Error(`Invalid port: ${port}`);
    const url = new URL(`${httpProto}://${uri}:${port}/size`);
    url.searchParams.set("cols", String(cols));
    url.searchParams.set("rows", String(rows));
    return url.toString();
}

/**
 * Builds the URL used to open the terminal WebSocket connection.
 */
export function buildWsUrl(wsProtocol: string, uri: string, port: number): URL {
    if (!isValidWsProtocol(wsProtocol)) throw new Error(`Invalid WebSocket protocol: "${wsProtocol}"`);
    if (!isValidUri(uri)) throw new Error(`Invalid URI: "${uri}"`);
    if (!isValidPort(port)) throw new Error(`Invalid port: ${port}`);
    return new URL(`${wsProtocol}://${uri}:${port}/ws`);
}

/**
 * Handles an incoming WebSocket message by writing its content to the terminal.
 * ArrayBuffer messages are decoded as streaming UTF-8; string messages are written directly.
 * writeCallback, when provided, is passed to term.write and fires after xterm.js has
 * fully parsed and rendered the data — used by debug timing to mark the end of a round-trip.
 */
export function handleSocketMessage(
    event: SocketMessageEvent,
    decoder: TextDecoder,
    term: TerminalLike,
    writeCallback?: () => void
): void {
    const data = event.data instanceof ArrayBuffer ? decoder.decode(event.data, { stream: true }) : event.data;
    if (writeCallback !== undefined) {
        term.write(data, writeCallback);
    } else {
        term.write(data);
    }
}

/**
 * Handles a WebSocket close event by writing an exit notice and alerting the user.
 * alertFn is injectable to allow testing without a real browser alert.
 */
export function handleSocketClose(term: TerminalLike, alertFn: (msg: string) => void): void {
    console.log("Socket closed");
    term.writeln("[exited]");
    alertFn("Connection closed");
}

/**
 * Sends a JSON resize message over the WebSocket if it is open (readyState === 1).
 */
export function sendResizeMessage(socket: SocketLike, cols: number, rows: number): void {
    if (socket.readyState === 1) {
        socket.send(JSON.stringify({ type: "resize", cols, rows }));
    }
}

/**
 * Registers terminal event listeners for keyboard input and the bell.
 * Idempotent — subsequent calls are no-ops once term._initialized is true.
 * bellElement is passed in so the function can be tested without a real DOM.
 * onBeforeSend, when provided, is called immediately before each socket.send —
 * used by debug timing to record the keypress timestamp.
 */
export function initTerm(
    term: TerminalLike,
    socket: SocketLike,
    bellElement: BellElementLike,
    onBeforeSend?: () => void
): void {
    if (term._initialized) return;
    term._initialized = true;

    term.onData((chunk) => {
        if (onBeforeSend !== undefined) onBeforeSend();
        socket.send(chunk);
    });

    term.onBell(() => {
        bellElement.style.display = "block";
        setTimeout(() => {
            bellElement.style.display = "none";
        }, 500);
    });
}

/**
 * Main entry point. Wires together all terminal, WebSocket, and DOM interactions.
 */
export async function main(config: TermConfig): Promise<void> {
    const { wsProtocol, httpProto } = getProtocols(config.tls);

    document.documentElement.style.setProperty("--b3tty-font-size", `${config.fontSize}pt`);

    const theme = buildTheme(config.theme);
    const termOptions = buildTermOptions(config, theme);

    const term = new Terminal(termOptions);
    const termElement = document.getElementById("terminal")!;
    term.open(termElement);

    term.loadAddon(new WebLinksAddon());
    term.loadAddon(new ImageAddon());

    let fitAddon: FitAddon | undefined;
    if (!config.columns) {
        fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        fitAddon.fit();
    }

    if (config.theme.background) {
        document.getElementById("container")!.style.background = config.theme.background;
    }

    const sizeUrl = buildSizeUrl(httpProto, config.uri, config.port, term.cols, term.rows);
    await fetch(sizeUrl, { method: "POST" });

    const wsUrl = buildWsUrl(wsProtocol, config.uri, config.port);
    const socket = new WebSocket(wsUrl);
    socket.binaryType = "arraybuffer";

    const decoder = new TextDecoder("utf-8");

    // When debug mode is enabled, measure the round-trip time from the moment
    // a keypress is sent to the pty to when the response has been rendered by
    // xterm.js. keypressTime holds the performance.now() snapshot taken just
    // before socket.send; the write callback computes and logs the delta.
    let keypressTime: number | null = null;
    const onBeforeSend = config.debug
        ? () => {
              keypressTime = performance.now();
          }
        : undefined;
    const writeCallback = config.debug
        ? () => {
              if (keypressTime !== null) {
                  const elapsed = (performance.now() - keypressTime).toFixed(2);
                  console.log(`[b3tty] keypress round-trip: ${elapsed}ms`);
                  keypressTime = null;
              }
          }
        : undefined;

    socket.onmessage = (event) => {
        if (socket.readyState !== 1) {
            console.log("websocket not ready!");
        }
        handleSocketMessage(event as SocketMessageEvent, decoder, term, writeCallback);
    };

    const dialog = document.getElementById("dialog") as unknown as B3ttyDialog;
    socket.onclose = () => {
        // Disable and hide the cursor. cursorInactiveStyle "none" hides it
        // when unfocused; the focus listener ensures a click on the terminal
        // after close cannot bring the cursor back.
        term.options.cursorBlink = false;
        term.options.cursorInactiveStyle = "none";
        term.blur();
        term.textarea?.addEventListener("focus", () => term.blur());
        handleSocketClose(term, (msg) => dialog.show(msg));
    };
    socket.onerror = (event) => console.log("A socket error occurred: ", event);
    socket.onopen = () => console.log("Socket opened");

    const bellElement = document.getElementById("bell")!;
    initTerm(term, socket, bellElement, onBeforeSend);

    if (!config.columns) {
        term.onResize(({ cols, rows }) => {
            sendResizeMessage(socket, cols, rows);
        });

        window.addEventListener("resize", () => {
            fitAddon!.fit();
        });
    }
}

if (typeof window !== "undefined" && window.B3TTY) {
    main(window.B3TTY);
}
