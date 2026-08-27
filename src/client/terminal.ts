import { Terminal } from "@xterm/xterm";
import type { ITerminalInitOnlyOptions, ITerminalOptions, ITheme } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import type { WebLinksAddon } from "@xterm/addon-web-links";
import type { ImageAddon } from "@xterm/addon-image";
import type {
    TermConfig,
    ThemeActivateResponse,
    EditProfileResponse,
    BellElementLike,
    SocketLike,
    SocketMessageEvent,
    TerminalLike,
    ClientConfig,
    ThemeConfig,
    SettingsConfig,
    SettingsTerminalConfig,
} from "./types.ts";
import { isValidWsProtocol, isValidPort, isValidUri } from "./validators.ts";
import { postThemeConfig, postAddTheme, getSettings } from "./api.ts";
import type { B3ttyDialog, B3ttyMenuBar, B3ttyThemePicker, MenuBarColors } from "./components.ts";

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
 * Derives the menu bar's background/foreground colors from a theme's foreground
 * and background colors, falling back to white/black when unset.
 */
export function menuBarColors(theme: { foreground?: string; background?: string }): MenuBarColors {
    return { bg: setLight(theme.foreground), fg: setDark(theme.background) };
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
 * Fallback font stack appended after the user's configured font family.
 */
export const FONT_FALLBACK_STACK = "Menlo, DejaVu Sans Mono, Ubuntu Mono, Inconsolata, Fira, monospace";

/**
 * Builds the font-family value for xterm.js, appending FONT_FALLBACK_STACK after
 * the configured font family (or using it alone when fontFamily is empty).
 */
export function buildFontFamily(fontFamily?: string): string {
    return fontFamily ? `${fontFamily}, ${FONT_FALLBACK_STACK}` : FONT_FALLBACK_STACK;
}

/**
 * Builds the value for the --b3tty-font-family CSS custom property, quoting the
 * configured font family and falling back to "monospace" when empty.
 */
export function buildFontFamilyCssVar(fontFamily?: string): string {
    return fontFamily ? `"${fontFamily}", monospace` : "monospace";
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
        fontFamily: buildFontFamily(config.fontFamily),
        fontSize: config.fontSize,
    };
    if (allowTransparency) options.allowTransparency = true;
    options.rows = config.rows;
    options.cols = config.columns;
    if (Object.keys(theme).length > 0) options.theme = theme;
    return options;
}

/**
 * Builds the URL used to open the terminal WebSocket connection. cols/rows are
 * carried as query parameters so the server can size the pty at upgrade time
 * without a separate /size round trip before the socket opens.
 */
export function buildWsUrl(wsProtocol: string, uri: string, port: number, cols: number, rows: number): URL {
    if (!isValidWsProtocol(wsProtocol)) throw new Error(`Invalid WebSocket protocol: "${wsProtocol}"`);
    if (!isValidUri(uri)) throw new Error(`Invalid URI: "${uri}"`);
    if (!isValidPort(port)) throw new Error(`Invalid port: ${port}`);
    const url = new URL(`${wsProtocol}://${uri}:${port}/ws`);
    url.searchParams.set("cols", String(cols));
    url.searchParams.set("rows", String(rows));
    return url;
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
    writeCallback?: () => void,
    onSettings?: (t: SettingsTerminalConfig) => void,
    onTheme?: (name: string, theme: ThemeActivateResponse) => void
): void {
    if (!(event.data instanceof ArrayBuffer)) {
        if (typeof event.data === "string" && event.data.length > 0) {
            try {
                const msg = JSON.parse(event.data) as {
                    type: string;
                    terminal?: SettingsTerminalConfig;
                    name?: string;
                    theme?: ThemeActivateResponse;
                };
                if (msg.type === "settings" && msg.terminal && onSettings) {
                    onSettings(msg.terminal);
                } else if (msg.type === "theme" && msg.name && msg.theme && onTheme) {
                    onTheme(msg.name, msg.theme);
                }
            } catch {}
        }
        return;
    }
    const data = decoder.decode(event.data, { stream: true });
    if (writeCallback !== undefined) {
        term.write(data, writeCallback);
    } else {
        term.write(data);
    }
}

/**
 * Applies a resolved theme (from a fetch response or WebSocket broadcast) to the
 * live xterm.js Terminal and page styles, and updates activeTheme.current. Shared
 * by every runtime theme-apply path so the "zero out background when a background
 * image is configured" behavior stays in sync across all of them. Returns the
 * menu bar colors derived from the theme so callers can apply them as needed.
 */
export function applyResolvedTheme(
    name: string,
    themeResp: ThemeActivateResponse,
    term: Terminal,
    activeTheme: { current: string }
): MenuBarColors {
    const builtTheme = buildTheme(themeResp);
    if (themeResp.hasBackgroundImage) {
        builtTheme.background = withAlpha(themeResp.background || "#000", 0);
    }
    term.options.theme = builtTheme;
    applyThemeStyles(themeResp, themeResp.hasBackgroundImage);
    activeTheme.current = name;
    return menuBarColors(themeResp);
}

/**
 * Applies a theme received via WebSocket broadcast to the live terminal session.
 * Called when the server pushes a "theme" control message — for example when the
 * theme is changed via `b3tty theme set` from the CLI. Updates the xterm.js theme,
 * page background styles, and menu bar colors. When menuBar is null the color
 * update is skipped.
 */
export function applyThemeBroadcast(
    name: string,
    themeResp: ThemeActivateResponse,
    term: Terminal,
    menuBar: B3ttyMenuBar | null,
    activeTheme: { current: string }
): void {
    const colors = applyResolvedTheme(name, themeResp, term, activeTheme);
    menuBar?.updateColors(colors);
}

/**
 * Handles a WebSocket close event by writing an exit notice to the terminal.
 * The "Connection closed" dialog is shown only when wasClean is false, indicating
 * an unexpected drop rather than a server- or client-initiated close handshake.
 * alertFn is injectable for testing. The "Socket closed" console message is
 * gated behind debug, matching every other connection-lifecycle log.
 */
export function handleSocketClose(
    term: TerminalLike,
    alertFn: (msg: string) => void,
    wasClean = false,
    debug = false
): void {
    if (debug) console.log("Socket closed");
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
 * Returns the element with the given id if it exists and matches the given type
 * guard, or null otherwise. Used to look up optional overlay components (theme
 * picker/editor, profile/settings editors) that may be absent from the page.
 */
function getOverlay<T>(id: string, guard: (el: Element) => el is HTMLElement & T): (HTMLElement & T) | null {
    const el = document.getElementById(id);
    return el && guard(el) ? el : null;
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
    document.documentElement.style.setProperty("--b3tty-font-family", buildFontFamilyCssVar(config.fontFamily));
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

    menuBar.updateColors(applyResolvedTheme(name, newTheme, term, activeTheme));
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
 * Refreshes the menu bar after a theme change that may have added a new theme to
 * the Themes menu. When the response includes an updated themeNames list, config
 * and the menu bar are rebuilt with it; otherwise only the menu bar colors are updated.
 */
function refreshMenuBarThemeNames(
    newTheme: ThemeActivateResponse,
    menuBar: B3ttyMenuBar,
    config: TermConfig,
    colors: MenuBarColors
): void {
    if (newTheme.themeNames) {
        config.themeNames = newTheme.themeNames;
        menuBar.setup(config.themeNames, config.profileNames ?? [], colors);
    } else {
        menuBar.updateColors(colors);
    }
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

    const colors = applyResolvedTheme(name, newTheme, term, activeTheme);
    refreshMenuBarThemeNames(newTheme, menuBar, config, colors);
}

/**
 * Updates the menu bar after a profile is saved or deleted.
 * Called when b3tty-profile-editor dispatches "b3tty-profile-edited".
 */
export async function handleProfileEdited(e: Event, menuBar: B3ttyMenuBar, config: TermConfig): Promise<void> {
    const { response } = (e as CustomEvent<{ name: string | null; response: EditProfileResponse }>).detail;
    config.profileNames = response.profileNames;
    menuBar.setup(config.themeNames ?? [], config.profileNames, menuBarColors(config.theme));
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

    const colors = applyResolvedTheme(name, newTheme, term, activeTheme);

    if (newTheme.themeNames && !config.allThemeNames?.includes(name)) {
        config.allThemeNames = [...(config.allThemeNames ?? []), name].sort();
    }
    refreshMenuBarThemeNames(newTheme, menuBar, config, colors);
}

/**
 * Applies saved terminal settings (font, cursor) to the live xterm.js Terminal.
 * When the session started in fixed-size mode (auto-resize false), rows and columns
 * are also applied live via term.resize(); the registered term.onResize listener
 * propagates the new dimensions to the server PTY over WebSocket.
 */
export function applyTerminalSettings(t: SettingsTerminalConfig, term: Terminal, config: TermConfig): void {
    term.options.cursorBlink = t.cursorBlink;
    if (t.fontFamily) {
        term.options.fontFamily = buildFontFamily(t.fontFamily);
        document.documentElement.style.setProperty("--b3tty-font-family", buildFontFamilyCssVar(t.fontFamily));
    }
    if (t.fontSize > 0) {
        term.options.fontSize = t.fontSize;
        document.documentElement.style.setProperty("--b3tty-font-size", `${t.fontSize}px`);
    }
    config.fontFamily = t.fontFamily;
    config.fontSize = t.fontSize;
    config.cursorBlink = t.cursorBlink;
    if (!config.autoResize) {
        config.rows = t.rows;
        config.columns = t.columns;
        term.resize(t.columns, t.rows);
    }
    config.autoResize = t.autoResize;
}

export function handleSettingsEdited(e: Event, term: Terminal, config: TermConfig): void {
    const { response } = (e as CustomEvent<{ response: SettingsConfig }>).detail;
    applyTerminalSettings(response.terminal, term, config);
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
    // Kicked off immediately so these chunks download in parallel with terminal
    // setup below, instead of being parsed/evaluated as part of the same script
    // that has to run before the WebSocket can open. Neither the web components
    // (menu bar, dialogs, editors) nor the link/image addons are needed to
    // compute cols/rows or open the WebSocket.
    const componentsPromise = import("./components.ts");
    const extraAddonsPromise = Promise.all([import("@xterm/addon-web-links"), import("@xterm/addon-image")]);

    const { wsProtocol } = getProtocols(config.tls);

    applyPageStyles(config);

    const term = terminalFactory(config);
    term.open(requireElement("terminal"));

    let fitAddon: FitAddon | undefined;
    if (config.autoResize ?? true) {
        fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        fitAddon.fit();
    }

    extraAddonsPromise
        .then(([{ WebLinksAddon }, { ImageAddon }]) => {
            term.loadAddon(new WebLinksAddon());
            term.loadAddon(new ImageAddon());
        })
        .catch((err) => console.warn("failed to load terminal addons:", err));

    // cols/rows travel on the /ws URL itself so the server can size the pty at
    // upgrade time with no separate /size round trip before the socket opens.
    const wsUrl = buildWsUrl(wsProtocol, config.uri, config.port, term.cols, term.rows);
    const socket = new WebSocket(wsUrl);
    socket.binaryType = "arraybuffer";

    // listenerController cleans up all DOM event listeners when the session ends.
    const listenerController = new AbortController();
    const { signal } = listenerController;

    const decoder = new TextDecoder("utf-8");
    const { onBeforeSend, writeCallback } = buildDebugHooks(!!config.debug);

    // refit re-fits the terminal to its container on the next animation frame —
    // used after layout-affecting changes such as menu bar open/close or settings edits.
    const refit = () => requestAnimationFrame(() => fitAddon?.fit());

    let menuBar: (HTMLElement & B3ttyMenuBar) | null = null;
    const activeTheme = { current: config.activeTheme ?? "" };

    socket.onmessage = (event) => {
        if (socket.readyState !== 1 && config.debug) {
            console.log("websocket not ready!");
        }
        handleSocketMessage(
            event as SocketMessageEvent,
            decoder,
            term,
            writeCallback,
            (t) => {
                applyTerminalSettings(t, term, config);
                refit();
            },
            (name, themeResp) => {
                applyThemeBroadcast(name, themeResp, term, menuBar, activeTheme);
            }
        );
    };

    const components = await componentsPromise;

    const dialogEl = requireElement("dialog");
    if (!components.isB3ttyDialog(dialogEl)) throw new Error("Element #dialog is not a B3ttyDialog");
    const dialog: B3ttyDialog = dialogEl;
    socket.onclose = (event) => {
        listenerController.abort();
        disableCursor(term);
        handleSocketClose(term, (msg) => dialog.show(msg), event.wasClean, !!config.debug);
    };
    socket.onerror = (event) => {
        if (config.debug) console.log("A socket error occurred: ", event);
    };
    socket.onopen = () => {
        if (config.debug) console.log("Socket opened");
    };

    const bellElement = requireElement("bell");
    initTerm(term, socket, bellElement, onBeforeSend);

    const menuBarEl = document.getElementById("menubar");
    if (menuBarEl) {
        if (!components.isB3ttyMenuBar(menuBarEl)) throw new Error("Element #menubar is not a B3ttyMenuBar");
        menuBar = menuBarEl;
        menuBar.setup(config.themeNames ?? [], config.profileNames ?? [], menuBarColors(config.theme));

        menuBarEl.addEventListener("b3tty-menubar-open", refit, { signal });
        menuBarEl.addEventListener("b3tty-menubar-close", refit, { signal });

        menuBarEl.addEventListener("b3tty-theme-change", (e) => handleThemeChange(e, term, menuBarEl, activeTheme), {
            signal,
        });
        menuBarEl.addEventListener("b3tty-profile-change", handleProfileChange, { signal });

        const picker = getOverlay("theme-picker", components.isB3ttyThemePicker);

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
                (e) => handleThemeSelected(e, term, menuBarEl, picker!, config, activeTheme),
                { signal }
            );
        }

        const editor = getOverlay("theme-editor", components.isB3ttyThemeEditor);

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
                (e) => handleThemeEdited(e, term, menuBarEl, config, activeTheme),
                { signal }
            );
        }

        const profileEditor = getOverlay("profile-editor", components.isB3ttyProfileEditor);

        menuBarEl.addEventListener(
            "b3tty-open-profile-editor",
            () => {
                const editableNames = (config.profileNames ?? []).filter((n) => n !== "default");
                profileEditor?.open(editableNames);
            },
            { signal }
        );

        if (profileEditor) {
            profileEditor.addEventListener("b3tty-profile-edited", (e) => handleProfileEdited(e, menuBarEl, config), {
                signal,
            });
        }

        const settingsEditor = getOverlay("settings-editor", components.isB3ttySettingsEditor);

        menuBarEl.addEventListener(
            "b3tty-open-settings-editor",
            () => {
                void getSettings().then((settings) => {
                    settingsEditor?.open(settings);
                });
            },
            { signal }
        );

        if (settingsEditor) {
            settingsEditor.addEventListener(
                "b3tty-settings-edited",
                (e) => {
                    handleSettingsEdited(e, term, config);
                    refit();
                },
                {
                    signal,
                }
            );
        }
    }

    term.onResize(({ cols, rows }) => {
        sendResizeMessage(socket, cols, rows);
    });

    if (config.autoResize ?? true) {
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
