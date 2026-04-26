import { Terminal } from "@xterm/xterm";
import type { ITerminalInitOnlyOptions, ITerminalOptions, ITheme } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { ImageAddon } from "@xterm/addon-image";
import type {
    TermConfig,
    ThemeActivateResponse,
    BellElementLike,
    SocketLike,
    SocketMessageEvent,
    TerminalLike,
    ClientConfig,
    ThemeConfig,
} from "./types.ts";
import { isValidHttpProtocol, isValidWsProtocol, isValidPort, isValidUri } from "./validators.ts";
import { postSize, postThemeConfig, postAddTheme } from "./api.ts";
import "./components.ts";
import type { B3ttyDialog, B3ttyMenuBar, B3ttyThemePicker, B3ttyThemeEditor } from "./components.ts";
import { isB3ttyDialog, isB3ttyMenuBar, isB3ttyThemePicker, isB3ttyThemeEditor } from "./components.ts";

export const THEME_KEYS = [
    "foreground",
    "background",
    "cursor",
    "cursorAccent",
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
 * Converts a CSS hex color (#rgb or #rrggbb) to an rgba() string with the given alpha.
 * Falls back to rgba(0, 0, 0, alpha) for any input that is not a valid hex color.
 */
export function hexToRgba(hex: string, alpha: number): string {
    const full = hex.replace(/^#([0-9a-fA-F])([0-9a-fA-F])([0-9a-fA-F])$/, "#$1$1$2$2$3$3");
    const m = full.match(/^#([0-9a-fA-F]{2})([0-9a-fA-F]{2})([0-9a-fA-F]{2})$/);
    if (!m) return `rgba(0, 0, 0, ${alpha})`;
    return `rgba(${parseInt(m[1]!, 16)}, ${parseInt(m[2]!, 16)}, ${parseInt(m[3]!, 16)}, ${alpha})`;
}

/**
 * Returns a semi-transparent version of color at the given alpha (0–1).
 * Hex colors (#rgb / #rrggbb) are converted to rgba(); named CSS colors fall
 * back to rgba(0, 0, 0, alpha) since their RGB values are not known at runtime.
 */
export function withAlpha(color: string, alpha: number): string {
    if (color.startsWith("#")) return hexToRgba(color, alpha);
    return `rgba(0, 0, 0, ${alpha})`;
}

/**
 * Returns the color value, falling back to "white" if undefined or empty.
 */
export function setLight(color: string | undefined): string {
    return color || "white";
}

/**
 * Returns the color value, falling back to "black" if undefined or empty.
 */
export function setDark(color: string | undefined): string {
    return color || "black";
}

/**
 * Extracts defined theme color values from the config's theme object.
 * Only keys present in THEME_KEYS with truthy values are included.
 */
export function buildTheme(themeConfig: ThemeConfig | ThemeActivateResponse): ITheme {
    const theme: Record<string, string> = {};
    for (const k of THEME_KEYS) {
        const val = themeConfig[k];
        if (val) theme[k] = val;
    }
    return theme as ITheme;
}

/**
 * Builds the xterm.js Terminal options object from the b3tty config and resolved theme.
 * Pass allowTransparency=true when a background image is active so xterm.js renders
 * its canvas with a transparent background, letting the page background show through.
 */
export function buildTermOptions(
    config: ClientConfig,
    theme: ITheme,
    allowTransparency = false
): ITerminalOptions & ITerminalInitOnlyOptions {
    const options: ITerminalOptions & ITerminalInitOnlyOptions = {
        cursorBlink: config.cursorBlink,
        fontFamily: `${config.fontFamily}, Menlo, DejaVu Sans Mono, Ubuntu Mono, Inconsolata, Fira, monospace`,
        fontSize: config.fontSize,
    };
    if (allowTransparency) options.allowTransparency = true;
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
 * Handles a WebSocket close event by writing an exit notice to the terminal.
 * The "Connection closed" dialog is shown only when wasClean is false, indicating
 * an unexpected drop rather than a server- or client-initiated close handshake.
 * alertFn is injectable for testing.
 */
export function handleSocketClose(term: TerminalLike, alertFn: (msg: string) => void, wasClean = false): void {
    console.log("Socket closed");
    term.writeln("[exited]");
    if (!wasClean) {
        alertFn("Connection closed");
    }
}

/**
 * Builds the optional debug timing hooks used to measure keypress round-trip latency.
 * When debug is false both fields are undefined, adding no overhead to normal operation.
 * onBeforeSend fires immediately before socket.send; writeCallback fires after xterm.js
 * finishes rendering the PTY response. keypressTime is captured in the closure so the
 * two hooks share state without leaking it into main().
 */
export function buildDebugHooks(debug: boolean): { onBeforeSend?: () => void; writeCallback?: () => void } {
    if (!debug) return {};
    let keypressTime: number | null = null;
    return {
        onBeforeSend: () => {
            keypressTime = performance.now();
        },
        writeCallback: () => {
            if (keypressTime !== null) {
                const elapsed = (performance.now() - keypressTime).toFixed(2);
                console.log(`[b3tty] keypress round-trip: ${elapsed}ms`);
                keypressTime = null;
            }
        },
    };
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
 * Returns the element with the given id or throws a descriptive error if it is absent.
 * Prefer this over getElementById(id)! so missing elements produce clear failure messages
 * rather than cryptic null-dereference errors deep inside a handler.
 */
export function requireElement(id: string): HTMLElement {
    const el = document.getElementById(id);
    if (!el) throw new Error(`Required element #${id} not found`);
    return el;
}

/**
 * Constructs and returns a configured xterm.js Terminal from the given TermConfig.
 *
 * Builds the xterm.js theme from config.theme. When a background image is active,
 * the theme's background color is overridden to fully transparent so the canvas
 * does not add a second color layer on top of the body-level tint. Terminal
 * options (font, dimensions, cursor behavior, transparency) are derived via
 * buildTermOptions. The returned Terminal is ready to be mounted with term.open().
 */
export function terminalFactory(config: TermConfig): Terminal {
    const theme = buildTheme(config.theme);

    if (config.backgroundImage) {
        // The body provides a single uniform tint over the background image, so
        // xterm.js cell backgrounds must be fully transparent to avoid adding a
        // second layer of color that would make the terminal darker than the gap.
        theme.background = withAlpha("#000", 0);
    }
    // Always enable transparency so themes with background images can be switched
    // to at runtime without requiring a page reload.
    const termOptions = buildTermOptions(config, theme, true);

    return new Terminal(termOptions);
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
 * Applies theme-driven background and profile label styles to the page.
 * Called on initial load and on theme change. When hasBackgroundImage is true,
 * the body receives a semi-transparent gradient tint over the background image
 * and the xterm viewport is made transparent; otherwise the container gets a
 * solid background color and the body/style-element overrides are cleared.
 */
export function applyThemeStyles(
    theme: { background?: string; foreground?: string },
    hasBackgroundImage: boolean
): void {
    const containerEl = requireElement("container");

    if (hasBackgroundImage) {
        const bgColor = withAlpha(theme.background || "", 0.5);
        document.body.style.background = `linear-gradient(${bgColor}, ${bgColor}), url('/background') center / cover fixed no-repeat`;
        let bgStyle = document.getElementById("b3tty-bg-style") as HTMLStyleElement | null;
        if (!bgStyle) {
            bgStyle = document.createElement("style");
            bgStyle.id = "b3tty-bg-style";
            document.head.appendChild(bgStyle);
        }
        bgStyle.textContent = `#terminal .xterm-viewport { background-color: transparent !important; }`;
        containerEl.style.background = "";
    } else {
        document.body.style.background = "";
        document.getElementById("b3tty-bg-style")?.remove();
        containerEl.style.background = theme.background || "";
    }

    const profileEl = requireElement("profile");
    if (profileEl.textContent?.trim()) {
        profileEl.style.color = setLight(theme.foreground);
        profileEl.style.background = hasBackgroundImage ? "" : setDark(theme.background);
    }
}

/**
 * Applies all config-driven styles to the page: CSS custom properties for font,
 * the container/body background (solid color or background-image tint), and the
 * profile label colors. Kept separate from main() so DOM-style concerns don't
 * obscure the connection setup flow.
 */
export function applyPageStyles(config: TermConfig): void {
    document.documentElement.style.setProperty("--b3tty-font-size", `${config.fontSize}px`);
    document.documentElement.style.setProperty("--b3tty-font-family", `"${config.fontFamily}", monospace`);
    applyThemeStyles(config.theme, !!config.backgroundImage);
}

/**
 * Handles a b3tty-theme-change event by fetching the new theme config from the server,
 * applying it to the terminal and page styles, and updating the menu bar colors.
 * activeTheme is a ref object whose `current` field tracks the last successfully applied
 * theme name; if the selected name matches the current value the handler returns early
 * without making a network request. `current` is updated after each successful change.
 */
export async function handleThemeChange(
    e: Event,
    term: Terminal,
    menuBar: B3ttyMenuBar,
    activeTheme: { current: string }
): Promise<void> {
    const { name } = (e as CustomEvent<{ name: string }>).detail;
    if (name === activeTheme.current) return;
    let newTheme: ThemeActivateResponse;
    try {
        newTheme = await postThemeConfig(name);
    } catch {
        return;
    }

    const builtTheme = buildTheme(newTheme);
    if (newTheme.hasBackgroundImage) {
        builtTheme.background = withAlpha(newTheme.background || "#000", 0);
    }
    term.options.theme = builtTheme;

    applyThemeStyles(newTheme, newTheme.hasBackgroundImage);
    menuBar.updateColors({
        bg: setLight(newTheme.foreground),
        fg: setDark(newTheme.background),
    });
    activeTheme.current = name;
}

/**
 * Handles a b3tty-profile-change event by opening the selected profile in a new tab,
 * preserving any existing query parameters.
 */
export function handleProfileChange(e: Event): void {
    const { name } = (e as CustomEvent<{ name: string }>).detail;
    const params = new URLSearchParams(window.location.search);
    params.set("profile", name);
    window.open(`/?${params.toString()}`, "_blank");
}

/**
 * Handles a b3tty-theme-selected event from the B3ttyThemePicker overlay by posting
 * the chosen theme to /add-theme, applying it to the terminal and page styles, and
 * refreshing the menu bar. If the server returns an updated themeNames list, the menu
 * bar is fully rebuilt; otherwise only its colors are updated. `activeTheme.current` is
 * updated after each successful selection. On failure, the picker is closed without
 * changing the active theme.
 */
export async function handleThemeSelected(
    e: Event,
    term: Terminal,
    menuBar: B3ttyMenuBar,
    picker: B3ttyThemePicker,
    config: TermConfig,
    activeTheme: { current: string }
): Promise<void> {
    const { name } = (e as CustomEvent<{ name: string }>).detail;
    let newTheme: ThemeActivateResponse;
    try {
        newTheme = await postAddTheme(name);
    } catch {
        picker.close();
        return;
    }
    picker.close();

    const builtTheme = buildTheme(newTheme);
    if (newTheme.hasBackgroundImage) {
        builtTheme.background = withAlpha(newTheme.background || "#000", 0);
    }
    term.options.theme = builtTheme;
    applyThemeStyles(newTheme, newTheme.hasBackgroundImage);
    if (newTheme.themeNames) {
        config.themeNames = newTheme.themeNames;
        menuBar.setup(config.themeNames, config.profileNames ?? [], {
            bg: setLight(newTheme.foreground),
            fg: setDark(newTheme.background),
        });
    } else {
        menuBar.updateColors({
            bg: setLight(newTheme.foreground),
            fg: setDark(newTheme.background),
        });
    }
    activeTheme.current = name;
}

/**
 * Applies an edited or newly created theme to the terminal and menu bar.
 * Called when the b3tty-theme-editor dispatches "b3tty-theme-edited". The full
 * ThemeActivateResponse is carried in the event detail so no extra fetch is needed.
 */
export async function handleThemeEdited(
    e: Event,
    term: Terminal,
    menuBar: B3ttyMenuBar,
    config: TermConfig,
    activeTheme: { current: string }
): Promise<void> {
    const { name, response: newTheme } = (e as CustomEvent<{ name: string; response: ThemeActivateResponse }>).detail;

    const builtTheme = buildTheme(newTheme);
    if (newTheme.hasBackgroundImage) {
        builtTheme.background = withAlpha(newTheme.background || "#000", 0);
    }
    term.options.theme = builtTheme;
    applyThemeStyles(newTheme, newTheme.hasBackgroundImage);

    if (newTheme.themeNames) {
        config.themeNames = newTheme.themeNames;
        if (!config.allThemeNames?.includes(name)) {
            config.allThemeNames = [...(config.allThemeNames ?? []), name].sort();
        }
        menuBar.setup(config.themeNames, config.profileNames ?? [], {
            bg: setLight(newTheme.foreground),
            fg: setDark(newTheme.background),
        });
    } else {
        menuBar.updateColors({
            bg: setLight(newTheme.foreground),
            fg: setDark(newTheme.background),
        });
    }
    activeTheme.current = name;
}

/**
 * Permanently disables and hides the terminal cursor after the WebSocket closes.
 * cursorInactiveStyle "none" hides the cursor when the terminal loses focus; the
 * focus listener ensures a subsequent click cannot briefly restore it.
 */
export function disableCursor(term: Terminal): void {
    term.options.cursorBlink = false;
    term.options.cursorInactiveStyle = "none";
    term.blur();
    term.textarea?.addEventListener("focus", () => term.blur());
}

/**
 * Main entry point. Wires together all terminal, WebSocket, and DOM interactions.
 */
export async function main(config: TermConfig): Promise<void> {
    const { wsProtocol, httpProto } = getProtocols(config.tls);

    applyPageStyles(config);

    const term = terminalFactory(config);
    term.open(requireElement("terminal"));

    let fitAddon: FitAddon | undefined;
    if (!config.columns) {
        fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        fitAddon.fit();
    }

    term.loadAddon(new WebLinksAddon());
    term.loadAddon(new ImageAddon());

    const sizeUrl = buildSizeUrl(httpProto, config.uri, config.port, term.cols, term.rows);
    try {
        await postSize(sizeUrl);
    } catch (err) {
        console.warn(err instanceof Error ? err.message : String(err));
    }

    const wsUrl = buildWsUrl(wsProtocol, config.uri, config.port);
    const socket = new WebSocket(wsUrl);
    socket.binaryType = "arraybuffer";

    // listenerController cleans up all DOM event listeners when the session ends.
    const listenerController = new AbortController();
    const { signal } = listenerController;

    const decoder = new TextDecoder("utf-8");
    const { onBeforeSend, writeCallback } = buildDebugHooks(!!config.debug);

    socket.onmessage = (event) => {
        if (socket.readyState !== 1) {
            console.log("websocket not ready!");
        }
        handleSocketMessage(event as SocketMessageEvent, decoder, term, writeCallback);
    };

    const dialogEl = requireElement("dialog");
    if (!isB3ttyDialog(dialogEl)) throw new Error("Element #dialog is not a B3ttyDialog");
    const dialog: B3ttyDialog = dialogEl;
    socket.onclose = (event) => {
        listenerController.abort();
        disableCursor(term);
        handleSocketClose(term, (msg) => dialog.show(msg), event.wasClean);
    };
    socket.onerror = (event) => console.log("A socket error occurred: ", event);
    socket.onopen = () => console.log("Socket opened");

    const bellElement = requireElement("bell");
    initTerm(term, socket, bellElement, onBeforeSend);

    const menuBarEl = document.getElementById("menubar");
    if (menuBarEl) {
        if (!isB3ttyMenuBar(menuBarEl)) throw new Error("Element #menubar is not a B3ttyMenuBar");
        const menuBar: B3ttyMenuBar = menuBarEl;
        menuBar.setup(config.themeNames ?? [], config.profileNames ?? [], {
            bg: setLight(config.theme.foreground),
            fg: setDark(config.theme.background),
        });

        menuBarEl.addEventListener("b3tty-menubar-open", () => requestAnimationFrame(() => fitAddon?.fit()), {
            signal,
        });
        menuBarEl.addEventListener("b3tty-menubar-close", () => requestAnimationFrame(() => fitAddon?.fit()), {
            signal,
        });

        const activeTheme = { current: config.activeTheme ?? "" };
        menuBarEl.addEventListener("b3tty-theme-change", (e) => handleThemeChange(e, term, menuBar, activeTheme), {
            signal,
        });
        menuBarEl.addEventListener("b3tty-profile-change", handleProfileChange, { signal });

        const pickerEl = document.getElementById("theme-picker");
        let picker: (HTMLElement & B3ttyThemePicker) | null = null;
        if (pickerEl && isB3ttyThemePicker(pickerEl)) {
            picker = pickerEl;
        }

        menuBarEl.addEventListener(
            "b3tty-open-theme-selector",
            () => {
                picker?.open(config.allThemeNames ?? []);
            },
            { signal }
        );

        if (picker) {
            picker.addEventListener(
                "b3tty-theme-selected",
                (e) => handleThemeSelected(e, term, menuBar, picker!, config, activeTheme),
                { signal }
            );
        }

        const editorEl = document.getElementById("theme-editor");
        let editor: (HTMLElement & B3ttyThemeEditor) | null = null;
        if (editorEl && isB3ttyThemeEditor(editorEl)) {
            editor = editorEl;
        }

        menuBarEl.addEventListener(
            "b3tty-open-theme-editor",
            () => {
                editor?.open(config.allThemeNames ?? [], config.builtinThemeNames ?? []);
            },
            { signal }
        );

        if (editor) {
            editor.addEventListener(
                "b3tty-theme-edited",
                (e) => handleThemeEdited(e, term, menuBar, config, activeTheme),
                { signal }
            );
        }
    }

    if (!config.columns) {
        term.onResize(({ cols, rows }) => {
            sendResizeMessage(socket, cols, rows);
        });

        let resizeTimer: ReturnType<typeof setTimeout>;
        window.addEventListener(
            "resize",
            () => {
                clearTimeout(resizeTimer);
                resizeTimer = setTimeout(() => fitAddon!.fit(), 100);
            },
            { signal }
        );
    }
}

if (typeof window !== "undefined" && window.B3TTY) {
    main(window.B3TTY);
}
