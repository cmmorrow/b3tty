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
    return `${httpProto}://${uri}:${port}/size?cols=${cols}&rows=${rows}`;
}

/**
 * Builds the CSS text for the background gradient injected beneath the terminal.
 * The gradient fills the viewport area below the terminal element.
 */
export function buildBackgroundStyleContent(
    themeBackground: string,
    boundingBoxHeight: number,
    viewportHeight: number
): string {
    const percentage = ((boundingBoxHeight / viewportHeight) * 100).toFixed(2);
    return `#container::after { content: ""; left: 0; right: 0; bottom: 0; height: ${100 - parseFloat(percentage)}%; position: absolute; background: linear-gradient(to bottom, ${themeBackground}, #000000 120%); z-index: 1; }`;
}

/**
 * Handles an incoming WebSocket message by writing its content to the terminal.
 * ArrayBuffer messages are decoded as streaming UTF-8; string messages are written directly.
 */
export function handleSocketMessage(event: SocketMessageEvent, decoder: TextDecoder, term: TerminalLike): void {
    if (event.data instanceof ArrayBuffer) {
        term.write(decoder.decode(event.data, { stream: true }));
    } else {
        term.write(event.data);
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
 */
export function initTerm(term: TerminalLike, socket: SocketLike, bellElement: BellElementLike): void {
    if (term._initialized) return;
    term._initialized = true;

    term.onData((chunk) => socket.send(chunk));

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
export function main(config: TermConfig): void {
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
        const boundingBox = termElement.getBoundingClientRect();
        const styleContent = buildBackgroundStyleContent(
            config.theme.background,
            boundingBox.height,
            window.innerHeight
        );
        const style = document.createElement("style");
        style.textContent = styleContent;
        document.head.appendChild(style);
    }

    const sizeUrl = buildSizeUrl(httpProto, config.uri, config.port, term.cols, term.rows);
    fetch(sizeUrl, { method: "POST" });

    const socket = new WebSocket(`${wsProtocol}://${config.uri}:${config.port}/ws`);
    socket.binaryType = "arraybuffer";

    const decoder = new TextDecoder("utf-8");

    socket.onmessage = (event) => {
        if (socket.readyState !== 1) {
            console.log("websocket not ready!");
        }
        handleSocketMessage(event as SocketMessageEvent, decoder, term);
    };

    socket.onclose = () => handleSocketClose(term, alert);
    socket.onerror = (event) => console.log("A socket error occurred: ", event);
    socket.onopen = () => console.log("Socket opened");

    const bellElement = document.getElementById("bell")!;
    initTerm(term, socket, bellElement);

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
