import { describe, it, expect, beforeEach, afterEach, mock, spyOn } from "bun:test";
import { Terminal } from "@xterm/xterm";
import type { ITerminalInitOnlyOptions, ITerminalOptions } from "@xterm/xterm";
import {
    THEME_KEYS,
    getProtocols,
    buildTheme,
    buildTermOptions,
    buildWsUrl,
    handleSocketMessage,
    handleSocketClose,
    applyTerminalSettings,
    sendResizeMessage,
    initTerm,
    hexToRgba,
    withAlpha,
    setLight,
    setDark,
    menuBarColors,
    menuBarMode,
    FONT_FALLBACK_STACK,
    buildFontFamily,
    buildFontFamilyCssVar,
    terminalFactory,
    buildDebugHooks,
    requireElement,
    disableCursor,
    applyThemeStyles,
    applyPageStyles,
    handleThemeChange,
    handleProfileChange,
    handleThemeSelected,
    handleThemeEdited,
    applyResolvedTheme,
    applyThemeBroadcast,
    applyThemeWithBackground,
    backgroundImageUrl,
    backgroundImageAlpha,
    loadBackgroundImage,
    activateTheme,
    handleProfileEdited,
    isCommandPaletteShortcut,
    buildPaletteCommands,
    loadRecentCommandIds,
    recordRecentCommandId,
    RECENT_COMMANDS_KEY,
    MAX_RECENT_COMMANDS,
} from "./terminal.ts";
import type { PaletteActions } from "./terminal.ts";
import {
    isValidHttpProtocol,
    isValidWsProtocol,
    isValidPort,
    isValidUri,
    isValidThemeColor,
    isValidBackgroundImageTransparency,
    MAX_UINT16,
} from "./validators.ts";
import { isB3ttyDialog, isB3ttyMenuBar } from "./components.ts";
import { apiFetch, getSettings } from "./api.ts";
import { isThemeActivateResponse } from "./types.ts";

// ---------------------------------------------------------------------------
// Shared mock factories
// ---------------------------------------------------------------------------

function makeMockTerm() {
    return {
        _initialized: false,
        write: mock(() => {}),
        writeln: mock(() => {}),
        onData: mock((_cb: (chunk: string) => void) => {}),
        onBell: mock((_cb: () => void) => {}),
        onResize: mock((_cb: (size: { cols: number; rows: number }) => void) => {}),
    };
}

function makeMockSocket(readyState = 1) {
    return {
        readyState,
        send: mock((_data: string) => {}),
    };
}

function makeMockBellElement() {
    return {
        style: { display: "none" },
    };
}

// ---------------------------------------------------------------------------
// hexToRgba
// ---------------------------------------------------------------------------

describe("hexToRgba", () => {
    it("converts a 6-digit hex color to rgba", () => {
        expect(hexToRgba("#ff0000", 1)).toBe("rgba(255, 0, 0, 1)");
    });

    it("converts a 6-digit hex color with given alpha", () => {
        expect(hexToRgba("#14181d", 0.5)).toBe("rgba(20, 24, 29, 0.5)");
    });

    it("expands a 3-digit shorthand before converting", () => {
        expect(hexToRgba("#fff", 0.5)).toBe("rgba(255, 255, 255, 0.5)");
        expect(hexToRgba("#abc", 1)).toBe("rgba(170, 187, 204, 1)");
    });

    it("is case-insensitive for hex digits", () => {
        expect(hexToRgba("#FFFFFF", 0.5)).toBe("rgba(255, 255, 255, 0.5)");
        expect(hexToRgba("#aAbBcC", 0.5)).toBe("rgba(170, 187, 204, 0.5)");
    });

    it("falls back to rgba(0,0,0,alpha) for named colors", () => {
        expect(hexToRgba("red", 0.5)).toBe("rgba(0, 0, 0, 0.5)");
    });

    it("falls back to rgba(0,0,0,alpha) for empty string", () => {
        expect(hexToRgba("", 0.5)).toBe("rgba(0, 0, 0, 0.5)");
    });

    it("falls back to rgba(0,0,0,alpha) for invalid hex", () => {
        expect(hexToRgba("#gggggg", 0.5)).toBe("rgba(0, 0, 0, 0.5)");
    });
});

// ---------------------------------------------------------------------------
// withAlpha
// ---------------------------------------------------------------------------

describe("withAlpha", () => {
    it("converts a hex color to rgba with the given alpha", () => {
        expect(withAlpha("#14181d", 0.5)).toBe("rgba(20, 24, 29, 0.5)");
    });

    it("delegates 3-digit hex to hexToRgba", () => {
        expect(withAlpha("#fff", 0.5)).toBe("rgba(255, 255, 255, 0.5)");
    });

    it("falls back to rgba(0,0,0,alpha) for a named color", () => {
        expect(withAlpha("black", 0.5)).toBe("rgba(0, 0, 0, 0.5)");
        expect(withAlpha("cornflowerblue", 0.5)).toBe("rgba(0, 0, 0, 0.5)");
    });

    it("falls back to rgba(0,0,0,alpha) for an empty string", () => {
        expect(withAlpha("", 0.5)).toBe("rgba(0, 0, 0, 0.5)");
    });
});

// ---------------------------------------------------------------------------
// setLight
// ---------------------------------------------------------------------------

describe("setLight", () => {
    it("returns the color when defined and non-empty", () => {
        expect(setLight("#ffffff")).toBe("#ffffff");
    });

    it("returns 'white' when the value is undefined", () => {
        expect(setLight(undefined)).toBe("white");
    });

    it("returns 'white' when the value is an empty string", () => {
        expect(setLight("")).toBe("white");
    });
});

// ---------------------------------------------------------------------------
// setDark
// ---------------------------------------------------------------------------

describe("setDark", () => {
    it("returns the color when defined and non-empty", () => {
        expect(setDark("#000000")).toBe("#000000");
    });

    it("returns 'black' when the value is undefined", () => {
        expect(setDark(undefined)).toBe("black");
    });

    it("returns 'black' when the value is an empty string", () => {
        expect(setDark("")).toBe("black");
    });
});

// ---------------------------------------------------------------------------
// menuBarColors
// ---------------------------------------------------------------------------

describe("menuBarColors", () => {
    it("uses the theme foreground as bg and background as fg", () => {
        expect(menuBarColors({ foreground: "#f8f8f2", background: "#282a36" })).toEqual({
            bg: "#f8f8f2",
            fg: "#282a36",
        });
    });

    it("falls back to white/black when foreground/background are undefined", () => {
        expect(menuBarColors({})).toEqual({ bg: "white", fg: "black" });
    });

    it("falls back to white/black when foreground/background are empty strings", () => {
        expect(menuBarColors({ foreground: "", background: "" })).toEqual({ bg: "white", fg: "black" });
    });
});

// ---------------------------------------------------------------------------
// menuBarMode
// ---------------------------------------------------------------------------

describe("menuBarMode", () => {
    it("returns visible when config.showMenubar is 'visible'", () => {
        expect(menuBarMode({ showMenubar: "visible" })).toBe("visible");
    });

    it("returns hover when config.showMenubar is 'hover'", () => {
        expect(menuBarMode({ showMenubar: "hover" })).toBe("hover");
    });

    it("falls back to hover when config.showMenubar is undefined", () => {
        expect(menuBarMode({})).toBe("hover");
    });

    it("falls back to hover for any other value", () => {
        expect(menuBarMode({ showMenubar: "disable" })).toBe("hover");
    });
});

// ---------------------------------------------------------------------------
// getProtocols
// ---------------------------------------------------------------------------

describe("getProtocols", () => {
    it("returns ws/http when tls is false", () => {
        const result = getProtocols(false);
        expect(result.wsProtocol).toBe("ws");
        expect(result.httpProto).toBe("http");
    });

    it("returns wss/https when tls is true", () => {
        const result = getProtocols(true);
        expect(result.wsProtocol).toBe("wss");
        expect(result.httpProto).toBe("https");
    });

    it("treats falsy values as non-TLS", () => {
        // @ts-expect-error — testing runtime coercion with null
        expect(getProtocols(null).wsProtocol).toBe("ws");
        // @ts-expect-error
        expect(getProtocols(0).httpProto).toBe("http");
    });

    it("treats truthy non-boolean values as TLS", () => {
        // @ts-expect-error
        expect(getProtocols(1).wsProtocol).toBe("wss");
        // @ts-expect-error
        expect(getProtocols("yes").httpProto).toBe("https");
    });
});

// ---------------------------------------------------------------------------
// THEME_KEYS
// ---------------------------------------------------------------------------

describe("THEME_KEYS", () => {
    it("contains all expected keys", () => {
        const expected = [
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
        expect(THEME_KEYS).toEqual(expected);
    });

    it("has 22 entries", () => {
        expect(THEME_KEYS).toHaveLength(22);
    });

    it("contains no duplicate keys", () => {
        const unique = new Set(THEME_KEYS);
        expect(unique.size).toBe(THEME_KEYS.length);
    });
});

// ---------------------------------------------------------------------------
// buildTheme
// ---------------------------------------------------------------------------

describe("buildTheme", () => {
    it("returns only keys that have truthy values", () => {
        const themeConfig = { foreground: "#ffffff", background: "#000000" };
        const result = buildTheme(themeConfig);
        expect(result).toEqual({ foreground: "#ffffff", background: "#000000" });
    });

    it("returns an empty object when no theme keys are set", () => {
        expect(buildTheme({})).toEqual({});
    });

    it("omits keys with empty-string values", () => {
        const result = buildTheme({ foreground: "", background: "#000" });
        expect(result).not.toHaveProperty("foreground");
        expect(result).toHaveProperty("background", "#000");
    });

    it("ignores keys not in THEME_KEYS", () => {
        const result = buildTheme({ foreground: "#fff", unknownKey: "value" });
        expect(result).not.toHaveProperty("unknownKey");
    });

    it("includes all 22 keys when every theme value is provided", () => {
        const full: Record<string, string> = {};
        for (const k of THEME_KEYS) full[k] = "#aabbcc";
        const result = buildTheme(full);
        expect(Object.keys(result)).toHaveLength(22);
    });

    it("preserves the exact color string values", () => {
        const result = buildTheme({ red: "rgb(255, 0, 0)", blue: "hsl(240, 100%, 50%)" });
        expect(result.red).toBe("rgb(255, 0, 0)");
        expect(result.blue).toBe("hsl(240, 100%, 50%)");
    });
});

// ---------------------------------------------------------------------------
// buildFontFamily
// ---------------------------------------------------------------------------

describe("buildFontFamily", () => {
    it("appends the fallback stack after the configured font", () => {
        expect(buildFontFamily("Fira Code")).toBe(`Fira Code, ${FONT_FALLBACK_STACK}`);
    });

    it("returns the fallback stack alone when fontFamily is undefined", () => {
        expect(buildFontFamily(undefined)).toBe(FONT_FALLBACK_STACK);
    });

    it("returns the fallback stack alone when fontFamily is an empty string", () => {
        expect(buildFontFamily("")).toBe(FONT_FALLBACK_STACK);
    });
});

// ---------------------------------------------------------------------------
// buildFontFamilyCssVar
// ---------------------------------------------------------------------------

describe("buildFontFamilyCssVar", () => {
    it("quotes the configured font family", () => {
        expect(buildFontFamilyCssVar("Fira Code")).toBe(`"Fira Code", monospace`);
    });

    it("returns 'monospace' when fontFamily is undefined", () => {
        expect(buildFontFamilyCssVar(undefined)).toBe("monospace");
    });

    it("returns 'monospace' when fontFamily is an empty string", () => {
        expect(buildFontFamilyCssVar("")).toBe("monospace");
    });
});

// ---------------------------------------------------------------------------
// buildTermOptions
// ---------------------------------------------------------------------------

describe("buildTermOptions", () => {
    const baseConfig = {
        fontFamily: "Fira Code",
        fontSize: 14,
        rows: 24,
        columns: 80,
    };

    it("always includes fontFamily and fontSize, and hardcodes cursorBlink to false", () => {
        const result = buildTermOptions(baseConfig, {});
        expect(result.cursorBlink).toBe(false);
        expect(result.fontSize).toBe(14);
        expect(result.fontFamily).toContain("Fira Code");
    });

    it("appends fallback font families to the configured font", () => {
        const result = buildTermOptions(baseConfig, {});
        expect(result.fontFamily).toContain("Menlo");
        expect(result.fontFamily).toContain("monospace");
    });

    it("always sets rows from config", () => {
        const result = buildTermOptions({ ...baseConfig, rows: 24 }, {});
        expect(result.rows).toBe(24);
    });

    it("always sets cols from config", () => {
        const result = buildTermOptions({ ...baseConfig, columns: 80 }, {});
        expect(result.cols).toBe(80);
    });

    it("includes theme when a non-empty theme object is passed", () => {
        const theme = { foreground: "#fff" };
        const result = buildTermOptions(baseConfig, theme);
        expect(result.theme).toEqual(theme);
    });

    it("omits theme key when theme object is empty", () => {
        const result = buildTermOptions(baseConfig, {});
        expect(result).not.toHaveProperty("theme");
    });

    it("includes both rows and cols when both are set", () => {
        const result = buildTermOptions({ ...baseConfig, rows: 40, columns: 120 }, {});
        expect(result.rows).toBe(40);
        expect(result.cols).toBe(120);
    });

    it("does not set allowTransparency when the third argument is omitted", () => {
        const result = buildTermOptions(baseConfig, {});
        expect(result).not.toHaveProperty("allowTransparency");
    });

    it("does not set allowTransparency when the third argument is false", () => {
        const result = buildTermOptions(baseConfig, {}, false);
        expect(result).not.toHaveProperty("allowTransparency");
    });

    it("sets allowTransparency to true when the third argument is true", () => {
        const result = buildTermOptions(baseConfig, {}, true);
        expect(result.allowTransparency).toBe(true);
    });
});

// ---------------------------------------------------------------------------
// buildWsUrl
// ---------------------------------------------------------------------------

describe("buildWsUrl", () => {
    it("builds a correct ws URL", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 80, 24);
        expect(url.toString()).toBe("ws://localhost:8080/ws?cols=80&rows=24");
    });

    it("builds a correct wss URL", () => {
        const url = buildWsUrl("wss", "example.com", 8443, 132, 48);
        expect(url.toString()).toBe("wss://example.com:8443/ws?cols=132&rows=48");
    });

    it("handles non-standard ports", () => {
        const url = buildWsUrl("ws", "localhost", 3000, 80, 24);
        expect(url.toString()).toContain(":3000/");
    });

    it("returns a URL instance", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 80, 24);
        expect(url).toBeInstanceOf(URL);
    });

    it("reflects cols and rows exactly in the query string", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 1, 1);
        expect(url.searchParams.get("cols")).toBe("1");
        expect(url.searchParams.get("rows")).toBe("1");
    });

    it("handles zero cols and rows", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 0, 0);
        expect(url.toString()).toBe("ws://localhost:8080/ws?cols=0&rows=0");
    });

    it("carries the page's token, which the server requires on /ws", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 80, 24, "?token=abc123");
        expect(url.toString()).toBe("ws://localhost:8080/ws?cols=80&rows=24&token=abc123");
    });

    it("drops other page query parameters", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 80, 24, "?profile=work&token=abc123");
        expect(url.searchParams.get("token")).toBe("abc123");
        expect(url.searchParams.has("profile")).toBe(false);
    });

    it("URL-encodes the token", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 80, 24, "?token=a%26b");
        expect(url.searchParams.get("token")).toBe("a&b");
    });

    it("omits the token when the page has none (no-auth mode)", () => {
        const url = buildWsUrl("ws", "localhost", 8080, 80, 24, "?profile=work");
        expect(url.searchParams.has("token")).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// isValidHttpProtocol
// ---------------------------------------------------------------------------

describe("isValidHttpProtocol", () => {
    it("accepts http", () => {
        expect(isValidHttpProtocol("http")).toBe(true);
    });

    it("accepts https", () => {
        expect(isValidHttpProtocol("https")).toBe(true);
    });

    it("rejects ws", () => {
        expect(isValidHttpProtocol("ws")).toBe(false);
    });

    it("rejects wss", () => {
        expect(isValidHttpProtocol("wss")).toBe(false);
    });

    it("rejects empty string", () => {
        expect(isValidHttpProtocol("")).toBe(false);
    });

    it("rejects arbitrary string", () => {
        expect(isValidHttpProtocol("ftp")).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// isValidWsProtocol
// ---------------------------------------------------------------------------

describe("isValidWsProtocol", () => {
    it("accepts ws", () => {
        expect(isValidWsProtocol("ws")).toBe(true);
    });

    it("accepts wss", () => {
        expect(isValidWsProtocol("wss")).toBe(true);
    });

    it("rejects http", () => {
        expect(isValidWsProtocol("http")).toBe(false);
    });

    it("rejects https", () => {
        expect(isValidWsProtocol("https")).toBe(false);
    });

    it("rejects empty string", () => {
        expect(isValidWsProtocol("")).toBe(false);
    });

    it("rejects arbitrary string", () => {
        expect(isValidWsProtocol("ftp")).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// isValidPort
// ---------------------------------------------------------------------------

describe("isValidPort", () => {
    it("accepts port 1 (minimum)", () => {
        expect(isValidPort(1)).toBe(true);
    });

    it("accepts port 8080", () => {
        expect(isValidPort(8080)).toBe(true);
    });

    it("accepts port 65535 (MaxUint16)", () => {
        expect(isValidPort(MAX_UINT16)).toBe(true);
    });

    it("rejects port 0", () => {
        expect(isValidPort(0)).toBe(false);
    });

    it("rejects negative port", () => {
        expect(isValidPort(-1)).toBe(false);
    });

    it("rejects port above 65535", () => {
        expect(isValidPort(MAX_UINT16 + 1)).toBe(false);
    });

    it("rejects non-integer port", () => {
        expect(isValidPort(80.5)).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// isValidUri
// ---------------------------------------------------------------------------

describe("isValidUri", () => {
    it("accepts localhost", () => {
        expect(isValidUri("localhost")).toBe(true);
    });

    it("accepts a domain name", () => {
        expect(isValidUri("example.com")).toBe(true);
    });

    it("accepts a subdomain", () => {
        expect(isValidUri("sub.example.com")).toBe(true);
    });

    it("accepts an IPv4 address", () => {
        expect(isValidUri("192.168.1.1")).toBe(true);
    });

    it("accepts a hostname with hyphens", () => {
        expect(isValidUri("my-server.local")).toBe(true);
    });

    it("rejects an empty string", () => {
        expect(isValidUri("")).toBe(false);
    });

    it("rejects a URI with a protocol prefix", () => {
        expect(isValidUri("http://example.com")).toBe(false);
    });

    it("rejects a URI with a path", () => {
        expect(isValidUri("example.com/path")).toBe(false);
    });

    it("rejects a hostname with a trailing dot", () => {
        expect(isValidUri("example.com.")).toBe(false);
    });

    it("rejects a hostname with spaces", () => {
        expect(isValidUri("my server")).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// buildWsUrl — validation errors
// ---------------------------------------------------------------------------

describe("buildWsUrl validation", () => {
    it("throws on invalid WebSocket protocol", () => {
        expect(() => buildWsUrl("http", "localhost", 8080, 80, 24)).toThrow("Invalid WebSocket protocol");
    });

    it("throws on invalid URI", () => {
        expect(() => buildWsUrl("ws", "bad uri", 8080, 80, 24)).toThrow("Invalid URI");
    });

    it("throws on invalid port", () => {
        expect(() => buildWsUrl("ws", "localhost", 0, 80, 24)).toThrow("Invalid port");
    });
});

// ---------------------------------------------------------------------------
// handleSocketMessage
// ---------------------------------------------------------------------------

describe("handleSocketMessage", () => {
    let term: ReturnType<typeof makeMockTerm>;
    let decoder: TextDecoder;

    beforeEach(() => {
        term = makeMockTerm();
        decoder = new TextDecoder("utf-8");
    });

    it("decodes an ArrayBuffer and writes it to the terminal", () => {
        const text = "hello terminal";
        const buffer = new TextEncoder().encode(text).buffer;
        const event = { data: buffer };
        handleSocketMessage(event, decoder, term);
        expect(term.write).toHaveBeenCalledTimes(1);
        expect(term.write).toHaveBeenCalledWith(text);
    });

    it("ignores text frames — they are control messages, not PTY output", () => {
        const event = { data: "raw string" };
        handleSocketMessage(event, decoder, term);
        expect(term.write).not.toHaveBeenCalled();
    });

    it("handles multi-byte UTF-8 sequences in ArrayBuffer correctly", () => {
        const text = "こんにちは";
        const buffer = new TextEncoder().encode(text).buffer;
        handleSocketMessage({ data: buffer }, decoder, term);
        expect(term.write).toHaveBeenCalledWith(text);
    });

    it("handles an empty ArrayBuffer", () => {
        const buffer = new ArrayBuffer(0);
        handleSocketMessage({ data: buffer }, decoder, term);
        expect(term.write).toHaveBeenCalledTimes(1);
        expect(term.write).toHaveBeenCalledWith("");
    });

    it("silently ignores an empty string text frame", () => {
        handleSocketMessage({ data: "" }, decoder, term);
        expect(term.write).not.toHaveBeenCalled();
    });

    it("calls write once per binary message", () => {
        const a = new TextEncoder().encode("a").buffer;
        const b = new TextEncoder().encode("b").buffer;
        handleSocketMessage({ data: a }, decoder, term);
        handleSocketMessage({ data: b }, decoder, term);
        expect(term.write).toHaveBeenCalledTimes(2);
    });

    it("calls onSettings when a settings text frame is received", () => {
        const onSettings = mock((_t: unknown) => {});
        const payload = JSON.stringify({
            type: "settings",
            terminal: {
                fontFamily: "Fira Code",
                fontSize: 18,
                autoResize: true,
                rows: 24,
                columns: 80,
            },
        });
        handleSocketMessage({ data: payload }, decoder, term, undefined, onSettings);
        expect(onSettings).toHaveBeenCalledTimes(1);
        expect(onSettings).toHaveBeenCalledWith({
            fontFamily: "Fira Code",
            fontSize: 18,
            autoResize: true,
            rows: 24,
            columns: 80,
        });
        expect(term.write).not.toHaveBeenCalled();
    });

    it("ignores text frames with unknown type", () => {
        const onSettings = mock((_t: unknown) => {});
        const payload = JSON.stringify({ type: "unknown", data: "whatever" });
        handleSocketMessage({ data: payload }, decoder, term, undefined, onSettings);
        expect(onSettings).not.toHaveBeenCalled();
        expect(term.write).not.toHaveBeenCalled();
    });

    it("silently ignores malformed JSON text frames", () => {
        const onSettings = mock((_t: unknown) => {});
        handleSocketMessage({ data: "not json {{{" }, decoder, term, undefined, onSettings);
        expect(onSettings).not.toHaveBeenCalled();
        expect(term.write).not.toHaveBeenCalled();
    });

    it("calls onTheme when a theme text frame is received", () => {
        const onTheme = mock((_name: string, _theme: unknown) => {});
        const payload = JSON.stringify({
            type: "theme",
            name: "solarized-light",
            theme: { hasBackgroundImage: false, foreground: "#657b83", background: "#fdf6e3" },
        });
        handleSocketMessage({ data: payload }, decoder, term, undefined, undefined, onTheme);
        expect(onTheme).toHaveBeenCalledTimes(1);
        expect(onTheme.mock.calls[0]![0]).toBe("solarized-light");
        expect(onTheme.mock.calls[0]![1]).toMatchObject({ hasBackgroundImage: false, foreground: "#657b83" });
        expect(term.write).not.toHaveBeenCalled();
    });

    it("does not call onTheme when name is missing from the theme frame", () => {
        const onTheme = mock((_name: string, _theme: unknown) => {});
        const payload = JSON.stringify({
            type: "theme",
            theme: { hasBackgroundImage: false },
        });
        handleSocketMessage({ data: payload }, decoder, term, undefined, undefined, onTheme);
        expect(onTheme).not.toHaveBeenCalled();
    });

    it("does not call onTheme when theme data is missing from the theme frame", () => {
        const onTheme = mock((_name: string, _theme: unknown) => {});
        const payload = JSON.stringify({ type: "theme", name: "dracula" });
        handleSocketMessage({ data: payload }, decoder, term, undefined, undefined, onTheme);
        expect(onTheme).not.toHaveBeenCalled();
    });

    it("does not call onTheme for non-theme type frames", () => {
        const onTheme = mock((_name: string, _theme: unknown) => {});
        const payload = JSON.stringify({ type: "settings", terminal: {} });
        handleSocketMessage({ data: payload }, decoder, term, undefined, undefined, onTheme);
        expect(onTheme).not.toHaveBeenCalled();
    });
});

// ---------------------------------------------------------------------------
// backgroundImageUrl
// ---------------------------------------------------------------------------

describe("backgroundImageUrl", () => {
    it("carries the page's token over to /background", () => {
        expect(backgroundImageUrl("?token=abc123", "")).toBe("/background?token=abc123");
    });

    it("drops other page query parameters", () => {
        expect(backgroundImageUrl("?profile=work&token=abc123", "")).toBe("/background?token=abc123");
    });

    it("URL-encodes the token and theme name", () => {
        expect(backgroundImageUrl("?token=a%26b", "my theme&x")).toBe("/background?token=a%26b&theme=my+theme%26x");
    });

    it("gives each theme its own URL", () => {
        expect(backgroundImageUrl("?token=abc123", "one-light")).toBe("/background?token=abc123&theme=one-light");
        expect(backgroundImageUrl("?token=abc123", "iTerm")).toBe("/background?token=abc123&theme=iTerm");
    });

    it("carries the theme name without a token (no-auth mode)", () => {
        expect(backgroundImageUrl("", "iTerm")).toBe("/background?theme=iTerm");
    });

    it("adds a version derived from the image path, so an edited path gets a new URL", () => {
        const a = backgroundImageUrl("?token=abc123", "iTerm", "/srv/a.png");
        const b = backgroundImageUrl("?token=abc123", "iTerm", "/srv/b.png");
        expect(a).toMatch(/^\/background\?token=abc123&theme=iTerm&v=[0-9a-f]+$/);
        expect(b).not.toBe(a);
        expect(backgroundImageUrl("?token=abc123", "iTerm", "/srv/a.png")).toBe(a);
    });

    it("omits the version when no image path is given", () => {
        expect(backgroundImageUrl("?token=abc123", "iTerm")).toBe("/background?token=abc123&theme=iTerm");
    });

    it("omits the query string when there is neither a token nor a theme name", () => {
        expect(backgroundImageUrl("", "")).toBe("/background");
        expect(backgroundImageUrl("?profile=work", "")).toBe("/background");
    });
});

// ---------------------------------------------------------------------------
// backgroundImageAlpha
// ---------------------------------------------------------------------------

describe("backgroundImageAlpha", () => {
    it("defaults to 0.5 when the theme sets no transparency", () => {
        expect(backgroundImageAlpha({})).toBe(0.5);
    });

    it("maps the 0–100 transparency to an alpha, so higher is fainter", () => {
        expect(backgroundImageAlpha({ backgroundImageTransparency: 0 })).toBe(0);
        expect(backgroundImageAlpha({ backgroundImageTransparency: 20 })).toBe(0.2);
        expect(backgroundImageAlpha({ backgroundImageTransparency: 100 })).toBe(1);
    });
});

// ---------------------------------------------------------------------------
// loadBackgroundImage
// ---------------------------------------------------------------------------

describe("loadBackgroundImage", () => {
    it("resolves true when the image loads", async () => {
        const image = stubImage(true);
        try {
            expect(await loadBackgroundImage("/background?token=abc")).toBe(true);
            expect(image.srcs).toEqual(["/background?token=abc"]);
        } finally {
            image.restore();
        }
    });

    it("resolves false when the image fails to load", async () => {
        const image = stubImage(false);
        try {
            expect(await loadBackgroundImage("/background")).toBe(false);
        } finally {
            image.restore();
        }
    });

    it("resolves false when Image is unavailable", async () => {
        expect(typeof (globalThis as Record<string, unknown>)["Image"]).toBe("undefined");
        expect(await loadBackgroundImage("/background")).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// applyThemeWithBackground
// ---------------------------------------------------------------------------

describe("applyThemeWithBackground", () => {
    let savedDocument: unknown;
    let savedWindow: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
        savedWindow = (globalThis as Record<string, unknown>)["window"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
        (globalThis as Record<string, unknown>)["window"] = savedWindow;
    });

    const baseConfig = {
        tls: false,
        uri: "localhost",
        port: 8080,
        fontSize: 14,
        fontFamily: "monospace",
        rows: 24,
        columns: 80,
        theme: {},
    };

    it("renders synchronously without requesting an image when hasBackgroundImage is false", async () => {
        const { doc, elements } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            const applied = applyThemeWithBackground({ background: "#282a36" }, term, false, "dracula");
            expect(term.options.theme?.background).toBe("#282a36");
            expect(elements["container"]!.style["background"]).toBe("#282a36");
            expect(await applied).toBe(false);
            expect(image.srcs).toEqual([]);
        } finally {
            image.restore();
        }
    });

    it("resolves true and applies the image styles once the image loads", async () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            expect(await applyThemeWithBackground({ background: "#282a36" }, term, true, "dracula")).toBe(true);
            expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
            expect(bodyStyle["background"]).toContain("url('/background?theme=dracula')");
        } finally {
            image.restore();
        }
    });

    it("requests the image, and references it in CSS, with the page's token and the theme name", async () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        (globalThis as Record<string, unknown>)["window"] = { location: { search: "?token=abc123" } };
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            await applyThemeWithBackground({ background: "#282a36" }, term, true, "dracula");
            expect(image.srcs).toEqual(["/background?token=abc123&theme=dracula"]);
            expect(bodyStyle["background"]).toContain("url('/background?token=abc123&theme=dracula')");
        } finally {
            image.restore();
        }
    });

    it("resolves false and leaves the theme opaque when the image fails to load", async () => {
        const { doc, bodyStyle, elements } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(false);
        const warn = spyOn(console, "warn").mockImplementation(() => {});
        try {
            const term = terminalFactory(baseConfig);
            expect(await applyThemeWithBackground({ background: "#282a36" }, term, true, "dracula")).toBe(false);
            expect(term.options.theme?.background).toBe("#282a36");
            expect(bodyStyle["background"]).toBe("");
            expect(elements["container"]!.style["background"]).toBe("#282a36");
        } finally {
            image.restore();
            warn.mockRestore();
        }
    });

    it("does not apply a stale image over a theme applied while it was loading", async () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            const first = applyThemeWithBackground({ background: "#282a36" }, term, true, "dracula");
            const second = applyThemeWithBackground({ background: "#fdf6e3" }, term, false, "solarized-light");
            expect(await first).toBe(false);
            expect(await second).toBe(false);
            expect(term.options.theme?.background).toBe("#fdf6e3");
            expect(bodyStyle["background"]).toBe("");
        } finally {
            image.restore();
        }
    });
});

// ---------------------------------------------------------------------------
// applyResolvedTheme
// ---------------------------------------------------------------------------

describe("applyResolvedTheme", () => {
    let savedDocument: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
    });

    const baseConfig = {
        tls: false,
        uri: "localhost",
        port: 8080,
        fontSize: 14,
        fontFamily: "monospace",
        rows: 24,
        columns: 80,
        theme: {},
    };

    it("applies the theme colors to term.options.theme", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        const activeTheme = { current: "b3tty-dark" };
        applyResolvedTheme(
            "solarized-light",
            { hasBackgroundImage: false, foreground: "#657b83", background: "#fdf6e3" },
            term,
            activeTheme
        );
        expect(term.options.theme?.foreground).toBe("#657b83");
        expect(term.options.theme?.background).toBe("#fdf6e3");
    });

    it("keeps the theme background opaque until the background image loads, then makes it transparent", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            applyResolvedTheme("my-theme", { hasBackgroundImage: true, background: "#282a36" }, term, { current: "" });
            expect(term.options.theme?.background).toBe("#282a36");
            await flushPromises();
            expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
        } finally {
            image.restore();
        }
    });

    it("requests a different image URL for each theme, so one theme's loaded image is never reused for another", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            applyResolvedTheme("one-light", { hasBackgroundImage: true, background: "#fafafa" }, term, { current: "" });
            await flushPromises();
            applyResolvedTheme("iTerm", { hasBackgroundImage: true, background: "#15191e" }, term, { current: "" });
            await flushPromises();
            expect(image.srcs).toEqual(["/background?theme=one-light", "/background?theme=iTerm"]);
        } finally {
            image.restore();
        }
    });

    it("requests a new image URL when a theme's image path is edited, so the old image is never reused", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            const theme = { hasBackgroundImage: true, background: "#15191e" };
            applyResolvedTheme("iTerm", { ...theme, backgroundImage: "/srv/a.png" }, term, { current: "" });
            await flushPromises();
            applyResolvedTheme("iTerm", { ...theme, backgroundImage: "/srv/b.png" }, term, { current: "" });
            await flushPromises();
            expect(image.srcs).toHaveLength(2);
            expect(image.srcs[0]).not.toBe(image.srcs[1]);
        } finally {
            image.restore();
        }
    });

    it("renders the theme as if it had no background image when the image fails to load", async () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(false);
        const warn = spyOn(console, "warn").mockImplementation(() => {});
        try {
            const term = terminalFactory(baseConfig);
            applyResolvedTheme("my-theme", { hasBackgroundImage: true, background: "#282a36" }, term, { current: "" });
            await flushPromises();
            expect(term.options.theme?.background).toBe("#282a36");
            expect(bodyStyle["background"]).toBe("");
            expect(warn).toHaveBeenCalledTimes(1);
        } finally {
            image.restore();
            warn.mockRestore();
        }
    });

    it("updates activeTheme.current to the given name", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        const activeTheme = { current: "b3tty-dark" };
        applyResolvedTheme("dracula", { hasBackgroundImage: false }, term, activeTheme);
        expect(activeTheme.current).toBe("dracula");
    });

    it("returns menu bar colors derived from the theme's foreground and background", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        const colors = applyResolvedTheme(
            "dracula",
            { hasBackgroundImage: false, foreground: "#f8f8f2", background: "#282a36" },
            term,
            { current: "" }
        );
        expect(colors).toEqual({ bg: "#f8f8f2", fg: "#282a36" });
    });

    it("returns fallback menu bar colors when the theme has no foreground/background", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        const colors = applyResolvedTheme("dracula", { hasBackgroundImage: false }, term, { current: "" });
        expect(colors).toEqual({ bg: "white", fg: "black" });
    });
});

// ---------------------------------------------------------------------------
// applyThemeBroadcast
// ---------------------------------------------------------------------------

describe("applyThemeBroadcast", () => {
    let savedDocument: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
    });

    const baseConfig = {
        tls: false,
        uri: "localhost",
        port: 8080,
        fontSize: 14,
        fontFamily: "monospace",
        rows: 24,
        columns: 80,
        theme: {},
    };

    function makeMenuBar() {
        return { setup: mock(() => {}), updateColors: mock((_c: unknown) => {}) };
    }

    it("applies the theme colors to term.options.theme", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        const activeTheme = { current: "b3tty-dark" };
        applyThemeBroadcast(
            "solarized-light",
            { hasBackgroundImage: false, foreground: "#657b83", background: "#fdf6e3" },
            term,
            null,
            activeTheme
        );
        expect(term.options.theme?.foreground).toBe("#657b83");
        expect(term.options.theme?.background).toBe("#fdf6e3");
    });

    it("makes the theme background transparent once the background image loads", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(baseConfig);
            applyThemeBroadcast("my-theme", { hasBackgroundImage: true, background: "#282a36" }, term, null, {
                current: "",
            });
            await flushPromises();
            expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
        } finally {
            image.restore();
        }
    });

    it("updates activeTheme.current to the broadcast name", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        const activeTheme = { current: "b3tty-dark" };
        applyThemeBroadcast("dracula", { hasBackgroundImage: false }, term, null, activeTheme);
        expect(activeTheme.current).toBe("dracula");
    });

    it("calls menuBar.updateColors with the theme foreground and background", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        const menuBar = makeMenuBar();
        applyThemeBroadcast(
            "dracula",
            { hasBackgroundImage: false, foreground: "#f8f8f2", background: "#282a36" },
            term,
            menuBar,
            { current: "" }
        );
        expect(menuBar.updateColors).toHaveBeenCalledWith({ bg: "#f8f8f2", fg: "#282a36" });
    });

    it("skips menuBar.updateColors when menuBar is null", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(baseConfig);
        expect(() =>
            applyThemeBroadcast("dracula", { hasBackgroundImage: false }, term, null, { current: "" })
        ).not.toThrow();
    });
});

// ---------------------------------------------------------------------------
// handleSocketClose
// ---------------------------------------------------------------------------

describe("handleSocketClose", () => {
    it("always writes the [exited] message to the terminal", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn);
        expect(term.writeln).toHaveBeenCalledWith("[exited]");
    });

    it("writes [exited] even when the close was clean", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn, true);
        expect(term.writeln).toHaveBeenCalledWith("[exited]");
    });

    it("shows the dialog when wasClean is false (default)", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn);
        expect(alertFn).toHaveBeenCalledWith("Connection closed");
    });

    it("shows the dialog when wasClean is explicitly false", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn, false);
        expect(alertFn).toHaveBeenCalledWith("Connection closed");
    });

    it("suppresses the dialog when wasClean is true", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn, true);
        expect(alertFn).not.toHaveBeenCalled();
    });

    it("calls writeln exactly once regardless of wasClean", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn, true);
        expect(term.writeln).toHaveBeenCalledTimes(1);
    });

    it("does not log to the console when debug is false (default)", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        const logSpy = spyOn(console, "log");
        handleSocketClose(term, alertFn, true);
        expect(logSpy).not.toHaveBeenCalled();
        logSpy.mockRestore();
    });

    it("logs 'Socket closed' when debug is true", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        const logSpy = spyOn(console, "log");
        handleSocketClose(term, alertFn, true, true);
        expect(logSpy).toHaveBeenCalledWith("Socket closed");
        logSpy.mockRestore();
    });
});

// ---------------------------------------------------------------------------
// sendResizeMessage
// ---------------------------------------------------------------------------

describe("sendResizeMessage", () => {
    it("sends a JSON resize message when socket is open (readyState === 1)", () => {
        const socket = makeMockSocket(1);
        sendResizeMessage(socket, 80, 24);
        expect(socket.send).toHaveBeenCalledTimes(1);
        const sent = JSON.parse(socket.send.mock.calls[0]![0]);
        expect(sent).toEqual({ type: "resize", cols: 80, rows: 24 });
    });

    it("does not send when socket is connecting (readyState === 0)", () => {
        const socket = makeMockSocket(0);
        sendResizeMessage(socket, 80, 24);
        expect(socket.send).not.toHaveBeenCalled();
    });

    it("does not send when socket is closing (readyState === 2)", () => {
        const socket = makeMockSocket(2);
        sendResizeMessage(socket, 80, 24);
        expect(socket.send).not.toHaveBeenCalled();
    });

    it("does not send when socket is closed (readyState === 3)", () => {
        const socket = makeMockSocket(3);
        sendResizeMessage(socket, 80, 24);
        expect(socket.send).not.toHaveBeenCalled();
    });

    it("encodes cols and rows correctly in the JSON payload", () => {
        const socket = makeMockSocket(1);
        sendResizeMessage(socket, 132, 48);
        const sent = JSON.parse(socket.send.mock.calls[0]![0]);
        expect(sent.cols).toBe(132);
        expect(sent.rows).toBe(48);
    });

    it("sets type to 'resize' in the payload", () => {
        const socket = makeMockSocket(1);
        sendResizeMessage(socket, 80, 24);
        const sent = JSON.parse(socket.send.mock.calls[0]![0]);
        expect(sent.type).toBe("resize");
    });

    it("handles zero-value dimensions", () => {
        const socket = makeMockSocket(1);
        sendResizeMessage(socket, 0, 0);
        const sent = JSON.parse(socket.send.mock.calls[0]![0]);
        expect(sent.cols).toBe(0);
        expect(sent.rows).toBe(0);
    });
});

// ---------------------------------------------------------------------------
// buildDebugHooks
// ---------------------------------------------------------------------------

describe("buildDebugHooks", () => {
    it("returns empty object when debug is false", () => {
        const hooks = buildDebugHooks(false);
        expect(hooks.onBeforeSend).toBeUndefined();
        expect(hooks.writeCallback).toBeUndefined();
    });

    it("returns both hooks when debug is true", () => {
        const hooks = buildDebugHooks(true);
        expect(hooks.onBeforeSend).toBeTypeOf("function");
        expect(hooks.writeCallback).toBeTypeOf("function");
    });

    it("writeCallback is a no-op when called before onBeforeSend", () => {
        const { writeCallback } = buildDebugHooks(true);
        const logSpy = spyOn(console, "log");
        writeCallback!();
        expect(logSpy).not.toHaveBeenCalled();
        logSpy.mockRestore();
    });

    it("writeCallback logs a round-trip message after onBeforeSend is called", () => {
        const { onBeforeSend, writeCallback } = buildDebugHooks(true);
        const logSpy = spyOn(console, "log");
        onBeforeSend!();
        writeCallback!();
        expect(logSpy).toHaveBeenCalledTimes(1);
        expect(logSpy.mock.calls[0]![0]).toMatch(/\[b3tty\] keypress round-trip: \d+\.\d+ms/);
        logSpy.mockRestore();
    });

    it("writeCallback is a no-op on the second call without an intervening onBeforeSend", () => {
        const { onBeforeSend, writeCallback } = buildDebugHooks(true);
        const logSpy = spyOn(console, "log");
        onBeforeSend!();
        writeCallback!();
        writeCallback!();
        expect(logSpy).toHaveBeenCalledTimes(1);
        logSpy.mockRestore();
    });

    it("each onBeforeSend/writeCallback pair produces one log entry", () => {
        const { onBeforeSend, writeCallback } = buildDebugHooks(true);
        const logSpy = spyOn(console, "log");
        onBeforeSend!();
        writeCallback!();
        onBeforeSend!();
        writeCallback!();
        expect(logSpy).toHaveBeenCalledTimes(2);
        logSpy.mockRestore();
    });
});

// ---------------------------------------------------------------------------
// terminalFactory
// ---------------------------------------------------------------------------

describe("terminalFactory", () => {
    const baseConfig = {
        tls: false,
        uri: "localhost",
        port: 8080,
        fontSize: 14,
        fontFamily: "monospace",
        rows: 24,
        columns: 80,
        theme: {},
    };

    it("returns a Terminal instance", () => {
        const term = terminalFactory(baseConfig);
        expect(term).toBeInstanceOf(Terminal);
    });

    it("applies fontSize from config", () => {
        const term = terminalFactory({ ...baseConfig, fontSize: 20 });
        expect(term.options.fontSize).toBe(20);
    });

    it("applies rows from config when non-zero", () => {
        const term = terminalFactory({ ...baseConfig, rows: 30 });
        expect((term.options as ITerminalOptions & ITerminalInitOnlyOptions).rows).toBe(30);
    });

    it("applies cols from config when non-zero", () => {
        const term = terminalFactory({ ...baseConfig, columns: 120 });
        expect((term.options as ITerminalOptions & ITerminalInitOnlyOptions).cols).toBe(120);
    });

    it("sets allowTransparency when backgroundImage is absent", () => {
        const term = terminalFactory(baseConfig);
        expect(term.options.allowTransparency).toBeTruthy();
    });

    it("sets allowTransparency when backgroundImage is false", () => {
        const term = terminalFactory({ ...baseConfig, backgroundImage: false });
        expect(term.options.allowTransparency).toBeTruthy();
    });

    it("sets allowTransparency when backgroundImage is true", () => {
        const term = terminalFactory({ ...baseConfig, backgroundImage: true });
        expect(term.options.allowTransparency).toBe(true);
    });

    it("passes theme colors through to xterm.js when no background image", () => {
        const term = terminalFactory({
            ...baseConfig,
            theme: { foreground: "#ffffff", background: "#14181d" },
        });
        expect(term.options.theme?.foreground).toBe("#ffffff");
        expect(term.options.theme?.background).toBe("#14181d");
    });

    it("keeps the theme background opaque when backgroundImage is true (main applies the image once it loads)", () => {
        const term = terminalFactory({
            ...baseConfig,
            backgroundImage: true,
            theme: { background: "#14181d" },
        });
        expect(term.options.theme?.background).toBe("#14181d");
    });

    it("preserves non-background theme colors when backgroundImage is true", () => {
        const term = terminalFactory({
            ...baseConfig,
            backgroundImage: true,
            theme: { foreground: "#ffffff", background: "#14181d" },
        });
        expect(term.options.theme?.foreground).toBe("#ffffff");
    });

    it("includes fallback font families in the fontFamily option", () => {
        const term = terminalFactory({ ...baseConfig, fontFamily: "Fira Code" });
        expect(term.options.fontFamily).toContain("Fira Code");
        expect(term.options.fontFamily).toContain("monospace");
    });
});

// ---------------------------------------------------------------------------
// initTerm
// ---------------------------------------------------------------------------

describe("initTerm", () => {
    it("sets term._initialized to true", () => {
        const term = makeMockTerm();
        const socket = makeMockSocket();
        const bell = makeMockBellElement();
        initTerm(term, socket, bell);
        expect(term._initialized).toBe(true);
    });

    it("registers onData and onBell handlers", () => {
        const term = makeMockTerm();
        const socket = makeMockSocket();
        const bell = makeMockBellElement();
        initTerm(term, socket, bell);
        expect(term.onData).toHaveBeenCalledTimes(1);
        expect(term.onBell).toHaveBeenCalledTimes(1);
    });

    it("sends keyboard input to the socket via onData", () => {
        const term = makeMockTerm();
        const socket = makeMockSocket();
        const bell = makeMockBellElement();
        initTerm(term, socket, bell);

        // Extract and invoke the registered onData callback
        const onDataCallback = term.onData.mock.calls[0]![0];
        onDataCallback("ls -la\r");

        expect(socket.send).toHaveBeenCalledWith("ls -la\r");
    });

    it("sets bell element display to block when bell fires", () => {
        const term = makeMockTerm();
        const socket = makeMockSocket();
        const bell = makeMockBellElement();
        initTerm(term, socket, bell);

        const onBellCallback = term.onBell.mock.calls[0]![0];
        onBellCallback();

        expect(bell.style.display).toBe("block");
    });

    it("is idempotent — does not re-register handlers on repeated calls", () => {
        const term = makeMockTerm();
        const socket = makeMockSocket();
        const bell = makeMockBellElement();

        initTerm(term, socket, bell);
        initTerm(term, socket, bell);
        initTerm(term, socket, bell);

        expect(term.onData).toHaveBeenCalledTimes(1);
        expect(term.onBell).toHaveBeenCalledTimes(1);
    });

    it("does not overwrite _initialized if already true", () => {
        const term = { ...makeMockTerm(), _initialized: true };
        const socket = makeMockSocket();
        const bell = makeMockBellElement();
        initTerm(term, socket, bell);
        expect(term.onData).not.toHaveBeenCalled();
    });

    it("sends each keypress as a separate socket message", () => {
        const term = makeMockTerm();
        const socket = makeMockSocket();
        const bell = makeMockBellElement();
        initTerm(term, socket, bell);

        const onDataCallback = term.onData.mock.calls[0]![0];
        onDataCallback("a");
        onDataCallback("b");
        onDataCallback("c");

        expect(socket.send).toHaveBeenCalledTimes(3);
        expect(socket.send.mock.calls[0]![0]).toBe("a");
        expect(socket.send.mock.calls[1]![0]).toBe("b");
        expect(socket.send.mock.calls[2]![0]).toBe("c");
    });
});

// ---------------------------------------------------------------------------
// requireElement
// ---------------------------------------------------------------------------

describe("requireElement", () => {
    let savedDocument: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
    });

    it("returns the element when getElementById finds it", () => {
        const el = { id: "terminal" } as unknown as HTMLElement;
        (globalThis as Record<string, unknown>)["document"] = {
            getElementById: (id: string) => (id === "terminal" ? el : null),
        };
        expect(requireElement("terminal")).toBe(el);
    });

    it("throws when getElementById returns null", () => {
        (globalThis as Record<string, unknown>)["document"] = {
            getElementById: () => null,
        };
        expect(() => requireElement("missing")).toThrow("Required element #missing not found");
    });

    it("includes the element id in the error message", () => {
        (globalThis as Record<string, unknown>)["document"] = {
            getElementById: () => null,
        };
        expect(() => requireElement("dialog")).toThrow("#dialog");
        expect(() => requireElement("bell")).toThrow("#bell");
    });

    it("returns different elements for different ids", () => {
        const elA = { id: "a" } as unknown as HTMLElement;
        const elB = { id: "b" } as unknown as HTMLElement;
        (globalThis as Record<string, unknown>)["document"] = {
            getElementById: (id: string) => (id === "a" ? elA : id === "b" ? elB : null),
        };
        expect(requireElement("a")).toBe(elA);
        expect(requireElement("b")).toBe(elB);
    });
});

// ---------------------------------------------------------------------------
// isB3ttyDialog
// ---------------------------------------------------------------------------

describe("isB3ttyDialog", () => {
    it("returns true when the element has both show and hide methods", () => {
        const el = { show: () => {}, hide: () => {} } as unknown as Element;
        expect(isB3ttyDialog(el)).toBe(true);
    });

    it("returns false when show is missing", () => {
        const el = { hide: () => {} } as unknown as Element;
        expect(isB3ttyDialog(el)).toBe(false);
    });

    it("returns false when hide is missing", () => {
        const el = { show: () => {} } as unknown as Element;
        expect(isB3ttyDialog(el)).toBe(false);
    });

    it("returns false when both methods are missing", () => {
        const el = {} as unknown as Element;
        expect(isB3ttyDialog(el)).toBe(false);
    });

    it("returns false when show is a non-function value", () => {
        const el = { show: "not a function", hide: () => {} } as unknown as Element;
        expect(isB3ttyDialog(el)).toBe(false);
    });

    it("returns false when hide is a non-function value", () => {
        const el = { show: () => {}, hide: 42 } as unknown as Element;
        expect(isB3ttyDialog(el)).toBe(false);
    });

    it("narrows the type so show and hide are callable after the guard passes", () => {
        const shown: string[] = [];
        const el = {
            show: (msg: string) => {
                shown.push(msg);
            },
            hide: () => {},
        } as unknown as Element;
        if (isB3ttyDialog(el)) {
            el.show("Connection closed");
        }
        expect(shown).toEqual(["Connection closed"]);
    });
});

// ---------------------------------------------------------------------------
// isB3ttyMenuBar
// ---------------------------------------------------------------------------

describe("isB3ttyMenuBar", () => {
    it("returns true when the element has both setup and updateColors methods", () => {
        const el = { setup: () => {}, updateColors: () => {} } as unknown as Element;
        expect(isB3ttyMenuBar(el)).toBe(true);
    });

    it("returns false when setup is missing", () => {
        const el = { updateColors: () => {} } as unknown as Element;
        expect(isB3ttyMenuBar(el)).toBe(false);
    });

    it("returns false when updateColors is missing", () => {
        const el = { setup: () => {} } as unknown as Element;
        expect(isB3ttyMenuBar(el)).toBe(false);
    });

    it("returns false when both methods are missing", () => {
        const el = {} as unknown as Element;
        expect(isB3ttyMenuBar(el)).toBe(false);
    });

    it("returns false when setup is a non-function value", () => {
        const el = { setup: "not a function", updateColors: () => {} } as unknown as Element;
        expect(isB3ttyMenuBar(el)).toBe(false);
    });

    it("returns false when updateColors is a non-function value", () => {
        const el = { setup: () => {}, updateColors: true } as unknown as Element;
        expect(isB3ttyMenuBar(el)).toBe(false);
    });

    it("narrows the type so setup and updateColors are callable after the guard passes", () => {
        const calls: string[] = [];
        const el = {
            setup: () => {
                calls.push("setup");
            },
            updateColors: () => {
                calls.push("updateColors");
            },
        } as unknown as Element;
        if (isB3ttyMenuBar(el)) {
            el.setup([], [], { bg: "black", fg: "white" });
            el.updateColors({ bg: "white", fg: "black" });
        }
        expect(calls).toEqual(["setup", "updateColors"]);
    });
});

// ---------------------------------------------------------------------------
// isThemeActivateResponse
// ---------------------------------------------------------------------------

describe("isThemeActivateResponse", () => {
    it("returns true for a valid response with hasBackgroundImage true", () => {
        expect(isThemeActivateResponse({ hasBackgroundImage: true })).toBe(true);
    });

    it("returns true for a valid response with hasBackgroundImage false", () => {
        expect(isThemeActivateResponse({ hasBackgroundImage: false })).toBe(true);
    });

    it("returns true when additional theme color fields are present", () => {
        expect(
            isThemeActivateResponse({
                hasBackgroundImage: false,
                foreground: "#ffffff",
                background: "#000000",
                cursor: "#cccccc",
            })
        ).toBe(true);
    });

    it("returns false for null", () => {
        expect(isThemeActivateResponse(null)).toBe(false);
    });

    it("returns false for undefined", () => {
        expect(isThemeActivateResponse(undefined)).toBe(false);
    });

    it("returns false for a plain string", () => {
        expect(isThemeActivateResponse("dark")).toBe(false);
    });

    it("returns false for a number", () => {
        expect(isThemeActivateResponse(42)).toBe(false);
    });

    it("returns false when hasBackgroundImage is absent", () => {
        expect(isThemeActivateResponse({ foreground: "#ffffff" })).toBe(false);
    });

    it("returns false when hasBackgroundImage is a string instead of a boolean", () => {
        expect(isThemeActivateResponse({ hasBackgroundImage: "true" })).toBe(false);
    });

    it("returns false when hasBackgroundImage is a number", () => {
        expect(isThemeActivateResponse({ hasBackgroundImage: 1 })).toBe(false);
    });

    it("returns false for an empty object", () => {
        expect(isThemeActivateResponse({})).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// DOM stub helpers
// ---------------------------------------------------------------------------

type StyleMap = Record<string, string>;

function makeStyleObj(): { setProperty: ReturnType<typeof mock>; [key: string]: unknown } {
    const props: StyleMap = {};
    return {
        setProperty: mock((k: string, v: string) => {
            props[k] = v;
        }),
        get(k: string) {
            return props[k] ?? "";
        },
        _props: props,
    };
}

/**
 * Replaces globalThis.Image with a fake whose load succeeds (loads=true) or fails
 * on the next microtask, standing in for the browser's handling of a 200 vs.
 * non-200 /background response. Returns every src requested and a restore function.
 */
function stubImage(loads: boolean): { srcs: string[]; restore: () => void } {
    const g = globalThis as Record<string, unknown>;
    const saved = g["Image"];
    const srcs: string[] = [];
    g["Image"] = class {
        onload: (() => void) | null = null;
        onerror: (() => void) | null = null;
        set src(url: string) {
            srcs.push(url);
            queueMicrotask(() => (loads ? this.onload?.() : this.onerror?.()));
        }
    };
    return {
        srcs,
        restore: () => {
            g["Image"] = saved;
        },
    };
}

/** Lets pending promise callbacks (such as a stubbed image load) run. */
const flushPromises = () => new Promise((resolve) => setTimeout(resolve, 0));

function makeDomStub() {
    const elements: Record<string, { style: StyleMap; textContent: string | null }> = {
        container: { style: {} as StyleMap, textContent: null },
        profile: { style: {} as StyleMap, textContent: null },
    };
    const head = {
        _children: [] as Array<{ id: string; textContent: string }>,
        appendChild: mock(function (this: typeof head, el: { id: string; textContent: string }) {
            this._children.push(el);
        }),
    };
    const bodyStyle: StyleMap = {};
    const documentElementStyle = makeStyleObj();

    const doc = {
        body: { style: bodyStyle },
        head,
        documentElement: { style: documentElementStyle },
        getElementById: mock((id: string) => {
            if (id === "b3tty-bg-style") return head._children.find((c) => c.id === "b3tty-bg-style") ?? null;
            return elements[id] ?? null;
        }),
        createElement: mock((_tag: string) => {
            const el = {
                id: "",
                textContent: "",
                remove: () => {
                    head._children = head._children.filter((c) => c !== el);
                },
            };
            return el;
        }),
    };
    return { doc, elements, head, bodyStyle, documentElementStyle };
}

// ---------------------------------------------------------------------------
// disableCursor
// ---------------------------------------------------------------------------

describe("disableCursor", () => {
    it("sets cursorBlink to false", () => {
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        disableCursor(term);
        expect(term.options.cursorBlink).toBe(false);
    });

    it("sets cursorInactiveStyle to 'none'", () => {
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        disableCursor(term);
        expect(term.options.cursorInactiveStyle).toBe("none");
    });

    it("calls term.blur()", () => {
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        const blurSpy = spyOn(term, "blur");
        disableCursor(term);
        expect(blurSpy).toHaveBeenCalledTimes(1);
        blurSpy.mockRestore();
    });

    it("re-blurs on subsequent focus when textarea is present", () => {
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        const listeners: Array<() => void> = [];
        const fakeTextarea = { addEventListener: mock((_evt: string, cb: () => void) => listeners.push(cb)) };
        Object.defineProperty(term, "textarea", { value: fakeTextarea, configurable: true });
        const blurSpy = spyOn(term, "blur");
        disableCursor(term);
        listeners[0]?.();
        expect(blurSpy).toHaveBeenCalledTimes(2);
        blurSpy.mockRestore();
    });
});

// ---------------------------------------------------------------------------
// applyTerminalSettings
// ---------------------------------------------------------------------------

describe("applyTerminalSettings", () => {
    let savedDoc: unknown;

    beforeEach(() => {
        savedDoc = (globalThis as Record<string, unknown>)["document"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDoc;
    });

    it("resizes the terminal live when auto-resize is false", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            autoResize: false,
            rows: 24,
            columns: 80,
            theme: {},
        });
        const config = {
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            autoResize: false,
            rows: 24,
            columns: 80,
            theme: {},
        };
        applyTerminalSettings(
            { fontFamily: "mono", fontSize: 14, autoResize: false, rows: 30, columns: 100 },
            term,
            config
        );
        expect(term.rows).toBe(30);
        expect(term.cols).toBe(100);
        expect(config.rows).toBe(30);
        expect(config.columns).toBe(100);
    });

    it("does not resize the terminal when auto-resize is true", () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            autoResize: true,
            rows: 24,
            columns: 80,
            theme: {},
        });
        const config = {
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            autoResize: true,
            rows: 24,
            columns: 80,
            theme: {},
        };
        applyTerminalSettings(
            { fontFamily: "mono", fontSize: 14, autoResize: true, rows: 30, columns: 100 },
            term,
            config
        );
        expect(term.rows).toBe(24);
        expect(term.cols).toBe(80);
    });
});

// ---------------------------------------------------------------------------
// applyThemeStyles
// ---------------------------------------------------------------------------

describe("applyThemeStyles", () => {
    let saved: unknown;

    beforeEach(() => {
        saved = (globalThis as Record<string, unknown>)["document"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = saved;
    });

    it("sets body background to a linear-gradient when hasBackgroundImage is true", () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#14181d" }, true);
        expect(bodyStyle["background"]).toContain("linear-gradient");
        expect(bodyStyle["background"]).toContain("url('/background')");
    });

    it("includes a semi-transparent tint derived from the theme background color", () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#ffffff" }, true);
        expect(bodyStyle["background"]).toContain("rgba(255, 255, 255, 0.5)");
    });

    it("uses the theme's background image transparency as the tint's alpha", () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#ffffff", backgroundImageTransparency: 20 }, true);
        expect(bodyStyle["background"]).toContain("rgba(255, 255, 255, 0.2)");
    });

    it("references the image by a URL versioned with its path", () => {
        const { doc, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#ffffff", backgroundImage: "/srv/a.png" }, true, "iTerm");
        expect(bodyStyle["background"]).toContain(`url('${backgroundImageUrl("", "iTerm", "/srv/a.png")}')`);
    });

    it("injects a b3tty-bg-style element into the head when hasBackgroundImage is true", () => {
        const { doc, head } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#14181d" }, true);
        expect(head.appendChild).toHaveBeenCalledTimes(1);
        expect(head._children[0]?.textContent).toContain("xterm-viewport");
    });

    it("makes the xterm viewport transparent so the background image shows through", () => {
        const { doc, head } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#14181d" }, true);
        expect(head._children[0]?.textContent).toContain("background-color: transparent !important");
    });

    it("clears the container background when hasBackgroundImage is true", () => {
        const { doc, elements } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#14181d" }, true);
        expect(elements["container"]!.style["background"]).toBe("");
    });

    it("clears body background and sets container background when hasBackgroundImage is false", () => {
        const { doc, elements, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#14181d" }, false);
        expect(bodyStyle["background"]).toBe("");
        expect(elements["container"]!.style["background"]).toBe("#14181d");
    });

    it("sets profile label colors when the profile element has text content", () => {
        const { doc, elements } = makeDomStub();
        elements["profile"]!.textContent = "myprofile";
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ foreground: "#ffffff", background: "#14181d" }, false);
        expect(elements["profile"]!.style["color"]).toBe("#ffffff");
        expect(elements["profile"]!.style["background"]).toBe("#14181d");
    });

    it("does not set profile label colors when the profile element is empty", () => {
        const { doc, elements } = makeDomStub();
        elements["profile"]!.textContent = "";
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ foreground: "#ffffff", background: "#14181d" }, false);
        expect(elements["profile"]!.style["color"]).toBeUndefined();
    });

    it("clears profile label background when hasBackgroundImage is true", () => {
        const { doc, elements } = makeDomStub();
        elements["profile"]!.textContent = "myprofile";
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ foreground: "#ffffff", background: "#14181d" }, true);
        expect(elements["profile"]!.style["background"]).toBe("");
    });
});

// ---------------------------------------------------------------------------
// applyPageStyles
// ---------------------------------------------------------------------------

describe("applyPageStyles", () => {
    let saved: unknown;

    beforeEach(() => {
        saved = (globalThis as Record<string, unknown>)["document"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = saved;
    });

    const baseConfig = {
        tls: false,
        uri: "localhost",
        port: 8080,
        fontSize: 16,
        fontFamily: "Fira Code",
        rows: 24,
        columns: 80,
        theme: { background: "#14181d", foreground: "#ffffff" },
    };

    it("sets --b3tty-font-size CSS custom property", () => {
        const { doc, documentElementStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyPageStyles(baseConfig);
        expect(documentElementStyle.setProperty).toHaveBeenCalledWith("--b3tty-font-size", "16px");
    });

    it("sets --b3tty-font-family CSS custom property", () => {
        const { doc, documentElementStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyPageStyles(baseConfig);
        expect(documentElementStyle.setProperty).toHaveBeenCalledWith("--b3tty-font-family", `"Fira Code", monospace`);
    });

    it("delegates to applyThemeStyles with the config theme", () => {
        const { doc, elements, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyPageStyles({ ...baseConfig, backgroundImage: false });
        expect(bodyStyle["background"]).toBe("");
        expect(elements["container"]!.style["background"]).toBe("#14181d");
    });

    it("does not apply the background image even when backgroundImage is true", () => {
        const { doc, elements, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyPageStyles({ ...baseConfig, backgroundImage: true });
        expect(bodyStyle["background"]).toBe("");
        expect(elements["container"]!.style["background"]).toBe("#14181d");
    });
});

// ---------------------------------------------------------------------------
// handleProfileChange
// ---------------------------------------------------------------------------

describe("handleProfileChange", () => {
    let savedWindow: unknown;

    beforeEach(() => {
        savedWindow = (globalThis as Record<string, unknown>)["window"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["window"] = savedWindow;
    });

    function makeEvent(name: string): Event {
        return { detail: { name } } as unknown as Event;
    }

    it("opens a new tab with the selected profile in the query string", () => {
        const openMock = mock((_url: string, _target: string) => {});
        (globalThis as Record<string, unknown>)["window"] = { location: { search: "" }, open: openMock };
        handleProfileChange(makeEvent("work"));
        expect(openMock).toHaveBeenCalledTimes(1);
        const [url, target] = openMock.mock.calls[0]!;
        expect(url).toContain("profile=work");
        expect(target).toBe("_blank");
    });

    it("preserves existing query parameters when opening the new tab", () => {
        const openMock = mock((_url: string, _target: string) => {});
        (globalThis as Record<string, unknown>)["window"] = { location: { search: "?token=abc123" }, open: openMock };
        handleProfileChange(makeEvent("dev"));
        const [url] = openMock.mock.calls[0]!;
        expect(url).toContain("token=abc123");
        expect(url).toContain("profile=dev");
    });

    it("URL-encodes special characters in the profile name", () => {
        const openMock = mock((_url: string, _target: string) => {});
        (globalThis as Record<string, unknown>)["window"] = { location: { search: "" }, open: openMock };
        handleProfileChange(makeEvent("my profile"));
        const [url] = openMock.mock.calls[0]!;
        expect(url).toContain("profile=my+profile");
    });

    it("opens relative to the root path", () => {
        const openMock = mock((_url: string, _target: string) => {});
        (globalThis as Record<string, unknown>)["window"] = { location: { search: "" }, open: openMock };
        handleProfileChange(makeEvent("work"));
        const [url] = openMock.mock.calls[0]!;
        expect(url).toMatch(/^\//);
    });
});

// ---------------------------------------------------------------------------
// handleThemeChange
// ---------------------------------------------------------------------------

describe("handleThemeChange", () => {
    let savedDocument: unknown;
    let savedFetch: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
        savedFetch = (globalThis as Record<string, unknown>)["fetch"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
        (globalThis as Record<string, unknown>)["fetch"] = savedFetch;
        mock.restore();
    });

    function makeEvent(name: string): Event {
        return { detail: { name } } as unknown as Event;
    }

    function makeMenuBar() {
        return { setup: mock(() => {}), updateColors: mock((_c: unknown) => {}) };
    }

    function stubFetchTheme(overrides: Record<string, unknown> = {}) {
        const response = { hasBackgroundImage: false, foreground: "#ffffff", background: "#14181d", ...overrides };
        (globalThis as Record<string, unknown>)["fetch"] = mock(() =>
            Promise.resolve({ ok: true, json: () => Promise.resolve(response) })
        );
    }

    it("returns early without a fetch when the selected name matches the active theme", async () => {
        const fetchMock = mock(() => Promise.resolve({ ok: true, json: () => Promise.resolve({}) }));
        (globalThis as Record<string, unknown>)["fetch"] = fetchMock;
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        const menuBar = makeMenuBar();
        await handleThemeChange(makeEvent("dracula"), term, menuBar, { current: "dracula" });
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it("calls updateColors with the new theme's foreground and background", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ foreground: "#cdd6f4", background: "#1e1e2e" });
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        const menuBar = makeMenuBar();
        const activeTheme = { current: "b3tty-dark" };
        await handleThemeChange(makeEvent("catppuccin-mocha"), term, menuBar, activeTheme);
        expect(menuBar.updateColors).toHaveBeenCalledWith({ bg: "#cdd6f4", fg: "#1e1e2e" });
    });

    it("updates activeTheme.current after a successful change", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme();
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        const activeTheme = { current: "b3tty-dark" };
        await handleThemeChange(makeEvent("dracula"), term, makeMenuBar(), activeTheme);
        expect(activeTheme.current).toBe("dracula");
    });

    it("applies the new theme to term.options.theme", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ foreground: "#f8f8f2", background: "#282a36" });
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        await handleThemeChange(makeEvent("dracula"), term, makeMenuBar(), { current: "b3tty-dark" });
        expect(term.options.theme?.foreground).toBe("#f8f8f2");
        expect(term.options.theme?.background).toBe("#282a36");
    });

    it("makes the theme background transparent once the background image loads", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ hasBackgroundImage: true, background: "#282a36" });
        const image = stubImage(true);
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        try {
            await handleThemeChange(makeEvent("dracula"), term, makeMenuBar(), { current: "b3tty-dark" });
            await flushPromises();
            expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
        } finally {
            image.restore();
        }
    });

    it("does not update activeTheme.current when the fetch throws", async () => {
        (globalThis as Record<string, unknown>)["fetch"] = mock(() => Promise.reject(new Error("network error")));
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        const activeTheme = { current: "b3tty-dark" };
        await handleThemeChange(makeEvent("dracula"), term, makeMenuBar(), activeTheme);
        expect(activeTheme.current).toBe("b3tty-dark");
    });
});

// ---------------------------------------------------------------------------
// handleThemeSelected
// ---------------------------------------------------------------------------

describe("handleThemeSelected", () => {
    let savedDocument: unknown;
    let savedFetch: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
        savedFetch = (globalThis as Record<string, unknown>)["fetch"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
        (globalThis as Record<string, unknown>)["fetch"] = savedFetch;
        mock.restore();
    });

    function makeEvent(name: string): Event {
        return { detail: { name } } as unknown as Event;
    }

    function makeMenuBar() {
        return {
            setup: mock((_t: string[], _p: string[], _c: unknown) => {}),
            updateColors: mock((_c: unknown) => {}),
        };
    }

    function makePicker() {
        return { open: mock((_names: string[]) => {}), close: mock(() => {}) };
    }

    function makeConfig(overrides: Record<string, unknown> = {}) {
        return {
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
            themeNames: ["b3tty-dark"],
            profileNames: [],
            ...overrides,
        };
    }

    function stubFetchTheme(overrides: Record<string, unknown> = {}) {
        const response = { hasBackgroundImage: false, foreground: "#ffffff", background: "#14181d", ...overrides };
        (globalThis as Record<string, unknown>)["fetch"] = mock(() =>
            Promise.resolve({ ok: true, json: () => Promise.resolve(response) })
        );
    }

    it("closes the picker after a successful theme selection", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme();
        const picker = makePicker();
        await handleThemeSelected(
            makeEvent("dracula"),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            picker,
            makeConfig(),
            { current: "b3tty-dark" }
        );
        expect(picker.close).toHaveBeenCalledTimes(1);
    });

    it("closes the picker without changing the theme when the fetch throws", async () => {
        (globalThis as Record<string, unknown>)["fetch"] = mock(() => Promise.reject(new Error("network error")));
        const picker = makePicker();
        const activeTheme = { current: "b3tty-dark" };
        await handleThemeSelected(
            makeEvent("dracula"),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            picker,
            makeConfig(),
            activeTheme
        );
        expect(picker.close).toHaveBeenCalledTimes(1);
        expect(activeTheme.current).toBe("b3tty-dark");
    });

    it("updates activeTheme.current after a successful selection", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme();
        const activeTheme = { current: "b3tty-dark" };
        await handleThemeSelected(
            makeEvent("dracula"),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            makePicker(),
            makeConfig(),
            activeTheme
        );
        expect(activeTheme.current).toBe("dracula");
    });

    it("applies the new theme to term.options.theme", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ foreground: "#f8f8f2", background: "#282a36" });
        const term = terminalFactory(makeConfig());
        await handleThemeSelected(makeEvent("dracula"), term, makeMenuBar(), makePicker(), makeConfig(), {
            current: "b3tty-dark",
        });
        expect(term.options.theme?.foreground).toBe("#f8f8f2");
        expect(term.options.theme?.background).toBe("#282a36");
    });

    it("makes the theme background transparent once the background image loads", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ hasBackgroundImage: true, background: "#282a36" });
        const image = stubImage(true);
        try {
            const term = terminalFactory(makeConfig());
            await handleThemeSelected(makeEvent("dracula"), term, makeMenuBar(), makePicker(), makeConfig(), {
                current: "b3tty-dark",
            });
            await flushPromises();
            expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
        } finally {
            image.restore();
        }
    });

    it("calls menuBar.setup with updated themeNames when the response includes them", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ themeNames: ["b3tty-dark", "dracula"], foreground: "#f8f8f2", background: "#282a36" });
        const menuBar = makeMenuBar();
        const config = makeConfig({ themeNames: ["b3tty-dark"] });
        await handleThemeSelected(makeEvent("dracula"), terminalFactory(makeConfig()), menuBar, makePicker(), config, {
            current: "b3tty-dark",
        });
        expect(menuBar.setup).toHaveBeenCalledTimes(1);
        expect(menuBar.setup.mock.calls[0]![0]).toEqual(["b3tty-dark", "dracula"]);
        expect(menuBar.updateColors).not.toHaveBeenCalled();
    });

    it("calls menuBar.updateColors when the response does not include themeNames", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ foreground: "#cdd6f4", background: "#1e1e2e" });
        const menuBar = makeMenuBar();
        await handleThemeSelected(
            makeEvent("catppuccin-mocha"),
            terminalFactory(makeConfig()),
            menuBar,
            makePicker(),
            makeConfig(),
            { current: "b3tty-dark" }
        );
        expect(menuBar.updateColors).toHaveBeenCalledWith({ bg: "#cdd6f4", fg: "#1e1e2e" });
        expect(menuBar.setup).not.toHaveBeenCalled();
    });

    it("updates config.themeNames in place when the response includes themeNames", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ themeNames: ["b3tty-dark", "dracula"] });
        const config = makeConfig({ themeNames: ["b3tty-dark"] });
        await handleThemeSelected(
            makeEvent("dracula"),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            makePicker(),
            config,
            { current: "b3tty-dark" }
        );
        expect(config.themeNames).toEqual(["b3tty-dark", "dracula"]);
    });
});

// ---------------------------------------------------------------------------
// isValidThemeColor
// ---------------------------------------------------------------------------

describe("isValidBackgroundImageTransparency", () => {
    it("accepts empty (use the default)", () => {
        expect(isValidBackgroundImageTransparency("")).toBe(true);
    });

    it("accepts whole numbers from 0 to 100", () => {
        for (const v of ["0", "50", "100"]) expect(isValidBackgroundImageTransparency(v)).toBe(true);
    });

    it("rejects out-of-range, fractional, negative, and non-numeric values", () => {
        for (const v of ["101", "1.5", "-1", "1e2", "abc", " 5"]) {
            expect(isValidBackgroundImageTransparency(v)).toBe(false);
        }
    });
});

describe("isValidThemeColor", () => {
    it("returns true for empty string", () => {
        expect(isValidThemeColor("")).toBe(true);
    });
    it("returns true for 3-digit hex", () => {
        expect(isValidThemeColor("#fff")).toBe(true);
    });
    it("returns true for 6-digit hex", () => {
        expect(isValidThemeColor("#aabbcc")).toBe(true);
    });
    it("returns true for 6-digit uppercase hex", () => {
        expect(isValidThemeColor("#AABBCC")).toBe(true);
    });
    it("returns true for named color 'red'", () => {
        expect(isValidThemeColor("red")).toBe(true);
    });
    it("returns true for named color 'cornflowerblue'", () => {
        expect(isValidThemeColor("cornflowerblue")).toBe(true);
    });
    it("returns false for 5-digit hex", () => {
        expect(isValidThemeColor("#14181")).toBe(false);
    });
    it("returns false for invalid hex chars", () => {
        expect(isValidThemeColor("#gggggg")).toBe(false);
    });
    it("returns false for CSS rgb() function", () => {
        expect(isValidThemeColor("rgb(0,0,0)")).toBe(false);
    });
    it("returns false for named color with space", () => {
        expect(isValidThemeColor("dark blue")).toBe(false);
    });
    it("returns false for hex-like string without leading #", () => {
        expect(isValidThemeColor("14181d")).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// handleThemeEdited
// ---------------------------------------------------------------------------

describe("handleThemeEdited", () => {
    let savedDocument: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
        mock.restore();
    });

    function makeEditedEvent(name: string, overrides: Record<string, unknown> = {}): Event {
        return {
            detail: {
                name,
                response: {
                    hasBackgroundImage: false,
                    foreground: "#f8f8f2",
                    background: "#282a36",
                    ...overrides,
                },
            },
        } as unknown as Event;
    }

    function makeMenuBar() {
        return {
            setup: mock((_t: string[], _p: string[], _c: unknown) => {}),
            updateColors: mock((_c: unknown) => {}),
        };
    }

    function makeConfig(overrides: Record<string, unknown> = {}) {
        return {
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
            themeNames: ["b3tty-dark"],
            profileNames: [],
            allThemeNames: ["b3tty-dark"],
            ...overrides,
        };
    }

    it("applies the new theme to term.options.theme", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const term = terminalFactory(makeConfig());
        await handleThemeEdited(
            makeEditedEvent("my-theme", { foreground: "#f8f8f2", background: "#282a36" }),
            term,
            makeMenuBar(),
            makeConfig(),
            { current: "b3tty-dark" }
        );
        expect(term.options.theme?.foreground).toBe("#f8f8f2");
        expect(term.options.theme?.background).toBe("#282a36");
    });

    it("makes the theme background transparent once the background image loads", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const image = stubImage(true);
        try {
            const term = terminalFactory(makeConfig());
            await handleThemeEdited(
                makeEditedEvent("my-theme", { hasBackgroundImage: true, background: "#282a36" }),
                term,
                makeMenuBar(),
                makeConfig(),
                { current: "b3tty-dark" }
            );
            await flushPromises();
            expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
        } finally {
            image.restore();
        }
    });

    it("updates activeTheme.current", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const activeTheme = { current: "b3tty-dark" };
        await handleThemeEdited(
            makeEditedEvent("my-theme"),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            makeConfig(),
            activeTheme
        );
        expect(activeTheme.current).toBe("my-theme");
    });

    it("calls menuBar.setup when response includes themeNames", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const menuBar = makeMenuBar();
        const config = makeConfig({ themeNames: ["b3tty-dark"] });
        await handleThemeEdited(
            makeEditedEvent("my-theme", { themeNames: ["b3tty-dark", "my-theme"] }),
            terminalFactory(makeConfig()),
            menuBar,
            config,
            { current: "b3tty-dark" }
        );
        expect(menuBar.setup).toHaveBeenCalledTimes(1);
        expect(menuBar.setup.mock.calls[0]![0]).toEqual(["b3tty-dark", "my-theme"]);
        expect(menuBar.updateColors).not.toHaveBeenCalled();
    });

    it("calls menuBar.updateColors when response does not include themeNames", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const menuBar = makeMenuBar();
        await handleThemeEdited(
            makeEditedEvent("my-theme", { foreground: "#cdd6f4", background: "#1e1e2e" }),
            terminalFactory(makeConfig()),
            menuBar,
            makeConfig(),
            { current: "b3tty-dark" }
        );
        expect(menuBar.updateColors).toHaveBeenCalledWith({ bg: "#cdd6f4", fg: "#1e1e2e" });
        expect(menuBar.setup).not.toHaveBeenCalled();
    });

    it("updates config.themeNames when response includes themeNames", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const config = makeConfig({ themeNames: ["b3tty-dark"] });
        await handleThemeEdited(
            makeEditedEvent("my-theme", { themeNames: ["b3tty-dark", "my-theme"] }),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            config,
            { current: "b3tty-dark" }
        );
        expect(config.themeNames).toEqual(["b3tty-dark", "my-theme"]);
    });

    it("adds new theme name to config.allThemeNames when not already present", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const config = makeConfig({ allThemeNames: ["b3tty-dark"], themeNames: ["b3tty-dark"] });
        await handleThemeEdited(
            makeEditedEvent("my-theme", { themeNames: ["b3tty-dark", "my-theme"] }),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            config,
            { current: "b3tty-dark" }
        );
        expect(config.allThemeNames).toContain("my-theme");
    });

    it("does not duplicate name in config.allThemeNames if already present", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        const config = makeConfig({
            allThemeNames: ["b3tty-dark", "my-theme"],
            themeNames: ["b3tty-dark", "my-theme"],
        });
        await handleThemeEdited(
            makeEditedEvent("my-theme", { themeNames: ["b3tty-dark", "my-theme"] }),
            terminalFactory(makeConfig()),
            makeMenuBar(),
            config,
            { current: "b3tty-dark" }
        );
        expect(config.allThemeNames!.filter((n) => n === "my-theme").length).toBe(1);
    });
});

// ---------------------------------------------------------------------------
// apiFetch
// ---------------------------------------------------------------------------

describe("apiFetch", () => {
    let savedWindow: unknown;
    let savedFetch: unknown;
    let fetchMock: ReturnType<typeof mock>;

    beforeEach(() => {
        savedWindow = (globalThis as Record<string, unknown>)["window"];
        savedFetch = globalThis.fetch;
        fetchMock = mock(() => Promise.resolve(new Response("{}")));
        (globalThis as Record<string, unknown>)["fetch"] = fetchMock;
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["window"] = savedWindow;
        (globalThis as Record<string, unknown>)["fetch"] = savedFetch;
    });

    const setSearch = (search: string) => {
        (globalThis as Record<string, unknown>)["window"] = { location: { search } };
    };
    const sentHeaders = () => new Headers((fetchMock.mock.calls[0] as [string, RequestInit])[1].headers);

    it("sends the page's token as a bearer Authorization header", async () => {
        setSearch("?profile=work&token=abc123");
        await apiFetch("/settings");
        expect(sentHeaders().get("Authorization")).toBe("Bearer abc123");
    });

    it("keeps the caller's method, body, and headers", async () => {
        setSearch("?token=abc123");
        await apiFetch("/edit-theme", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: "{}",
        });
        const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
        expect(init.method).toBe("POST");
        expect(init.body).toBe("{}");
        expect(sentHeaders().get("Content-Type")).toBe("application/json");
        expect(sentHeaders().get("Authorization")).toBe("Bearer abc123");
    });

    it("sends the request unchanged when the page has no token (no-auth mode)", async () => {
        setSearch("?profile=work");
        await apiFetch("/settings");
        expect(fetchMock).toHaveBeenCalledWith("/settings");
    });

    it("is what the API helpers use", async () => {
        setSearch("?token=abc123");
        await getSettings().catch(() => {});
        expect(fetchMock.mock.calls[0]?.[0]).toBe("/settings");
        expect(sentHeaders().get("Authorization")).toBe("Bearer abc123");
    });
});

// ---------------------------------------------------------------------------
// activateTheme
// ---------------------------------------------------------------------------

describe("activateTheme", () => {
    let savedDocument: unknown;
    let savedFetch: unknown;

    beforeEach(() => {
        savedDocument = (globalThis as Record<string, unknown>)["document"];
        savedFetch = (globalThis as Record<string, unknown>)["fetch"];
    });

    afterEach(() => {
        (globalThis as Record<string, unknown>)["document"] = savedDocument;
        (globalThis as Record<string, unknown>)["fetch"] = savedFetch;
        mock.restore();
    });

    it("applies the theme and updates activeTheme when there is no menu bar", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        (globalThis as Record<string, unknown>)["fetch"] = mock(() =>
            Promise.resolve({
                ok: true,
                json: () =>
                    Promise.resolve({ hasBackgroundImage: false, foreground: "#ffffff", background: "#14181d" }),
            })
        );
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            rows: 24,
            columns: 80,
            theme: {},
        });
        const activeTheme = { current: "b3tty-dark" };
        await activateTheme("dracula", term, null, activeTheme);
        expect(activeTheme.current).toBe("dracula");
    });
});

// ---------------------------------------------------------------------------
// handleProfileEdited
// ---------------------------------------------------------------------------

describe("handleProfileEdited", () => {
    it("updates config.profileNames when there is no menu bar", async () => {
        const config = { profileNames: ["default"] } as unknown as Parameters<typeof handleProfileEdited>[2];
        const e = { detail: { name: "work", response: { profileNames: ["default", "work"] } } } as unknown as Event;
        await handleProfileEdited(e, null, config);
        expect(config.profileNames).toEqual(["default", "work"]);
    });
});

// ---------------------------------------------------------------------------
// isCommandPaletteShortcut
// ---------------------------------------------------------------------------

describe("isCommandPaletteShortcut", () => {
    const base = { ctrlKey: true, shiftKey: true, altKey: false, metaKey: false, code: "Semicolon" };

    it("matches Ctrl+Shift+;", () => {
        expect(isCommandPaletteShortcut(base)).toBe(true);
    });

    it("requires both Ctrl and Shift", () => {
        expect(isCommandPaletteShortcut({ ...base, ctrlKey: false })).toBe(false);
        expect(isCommandPaletteShortcut({ ...base, shiftKey: false })).toBe(false);
    });

    it("rejects extra Alt or Meta modifiers", () => {
        expect(isCommandPaletteShortcut({ ...base, altKey: true })).toBe(false);
        expect(isCommandPaletteShortcut({ ...base, metaKey: true })).toBe(false);
    });

    it("rejects other keys", () => {
        expect(isCommandPaletteShortcut({ ...base, code: "Quote" })).toBe(false);
    });
});

// ---------------------------------------------------------------------------
// buildPaletteCommands
// ---------------------------------------------------------------------------

describe("buildPaletteCommands", () => {
    function makeActions() {
        return {
            openThemePicker: mock(() => {}),
            openThemeEditor: mock(() => {}),
            openProfileEditor: mock(() => {}),
            openSettings: mock(() => {}),
            openAbout: mock(() => {}),
            activateTheme: mock((_name: string) => {}),
            openProfile: mock((_name: string) => {}),
        } satisfies PaletteActions;
    }

    it("always lists the menu commands, even with no themes or profiles", () => {
        const ids = buildPaletteCommands({}, makeActions()).map((c) => c.id);
        expect(ids).toEqual([
            "open:theme-selector",
            "open:theme-editor",
            "open:profile-editor",
            "open:settings",
            "open:about",
        ]);
    });

    it("labels the menu commands by kind", () => {
        const labels = buildPaletteCommands({}, makeActions()).map((c) => c.label);
        expect(labels).toEqual(["select:theme", "edit:theme", "edit:profile", "b3tty:settings", "b3tty:about"]);
    });

    it("runs the matching action for each menu command", () => {
        const actions = makeActions();
        const commands = buildPaletteCommands({}, actions);
        const run = (id: string) => commands.find((c) => c.id === id)!.run();
        run("open:theme-selector");
        run("open:theme-editor");
        run("open:profile-editor");
        run("open:settings");
        run("open:about");
        expect(actions.openThemePicker).toHaveBeenCalledTimes(1);
        expect(actions.openThemeEditor).toHaveBeenCalledTimes(1);
        expect(actions.openProfileEditor).toHaveBeenCalledTimes(1);
        expect(actions.openSettings).toHaveBeenCalledTimes(1);
        expect(actions.openAbout).toHaveBeenCalledTimes(1);
    });

    it("adds one command per Themes-menu theme that activates it", () => {
        const actions = makeActions();
        const commands = buildPaletteCommands({ themeNames: ["dracula", "solarized-dark"] }, actions);
        const themes = commands.filter((c) => c.id.startsWith("theme:"));
        expect(themes.map((c) => [c.id, c.label, c.group])).toEqual([
            ["theme:dracula", "theme:dracula", "Themes"],
            ["theme:solarized-dark", "theme:solarized-dark", "Themes"],
        ]);
        themes[1]!.run();
        expect(actions.activateTheme).toHaveBeenCalledWith("solarized-dark");
    });

    it("adds one command per non-default profile that opens it", () => {
        const actions = makeActions();
        const commands = buildPaletteCommands({ profileNames: ["default", "work"] }, actions);
        const profiles = commands.filter((c) => c.id.startsWith("profile:"));
        expect(profiles.map((c) => [c.id, c.label, c.group])).toEqual([["profile:work", "profile:work", "Profiles"]]);
        profiles[0]!.run();
        expect(actions.openProfile).toHaveBeenCalledWith("work");
    });
});

// ---------------------------------------------------------------------------
// loadRecentCommandIds / recordRecentCommandId
// ---------------------------------------------------------------------------

describe("recent command ids", () => {
    function makeStorage(initial?: string) {
        const data = new Map<string, string>();
        if (initial !== undefined) data.set(RECENT_COMMANDS_KEY, initial);
        return {
            data,
            getItem: (k: string) => data.get(k) ?? null,
            setItem: (k: string, v: string) => void data.set(k, v),
        };
    }

    it("loads [] from null, empty, malformed, or non-array storage", () => {
        expect(loadRecentCommandIds(null)).toEqual([]);
        expect(loadRecentCommandIds(makeStorage())).toEqual([]);
        expect(loadRecentCommandIds(makeStorage("{not json"))).toEqual([]);
        expect(loadRecentCommandIds(makeStorage('{"a":1}'))).toEqual([]);
    });

    it("loads [] when storage throws", () => {
        const storage = {
            getItem: () => {
                throw new Error("blocked");
            },
        };
        expect(loadRecentCommandIds(storage)).toEqual([]);
    });

    it("drops non-string entries", () => {
        expect(loadRecentCommandIds(makeStorage('["a", 1, null, "b"]'))).toEqual(["a", "b"]);
    });

    it("records most recent first without duplicates", () => {
        const storage = makeStorage('["a", "b", "c"]');
        recordRecentCommandId(storage, "b");
        expect(loadRecentCommandIds(storage)).toEqual(["b", "a", "c"]);
    });

    it("keeps at most the maximum number of ids", () => {
        const storage = makeStorage();
        for (let i = 0; i < MAX_RECENT_COMMANDS + 3; i++) recordRecentCommandId(storage, `cmd${i}`);
        const ids = loadRecentCommandIds(storage);
        expect(ids).toHaveLength(MAX_RECENT_COMMANDS);
        expect(ids[0]).toBe(`cmd${MAX_RECENT_COMMANDS + 2}`);
    });

    it("ignores null storage and setItem failures", () => {
        expect(() => recordRecentCommandId(null, "a")).not.toThrow();
        const storage = {
            getItem: () => null,
            setItem: () => {
                throw new Error("quota");
            },
        };
        expect(() => recordRecentCommandId(storage, "a")).not.toThrow();
    });
});
