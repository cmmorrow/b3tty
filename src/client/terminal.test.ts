import { describe, it, expect, beforeEach, afterEach, mock, spyOn } from "bun:test";
import { Terminal } from "@xterm/xterm";
import type { ITerminalInitOnlyOptions, ITerminalOptions } from "@xterm/xterm";
import {
    THEME_KEYS,
    getProtocols,
    buildTheme,
    buildTermOptions,
    buildSizeUrl,
    buildWsUrl,
    handleSocketMessage,
    handleSocketClose,
    sendResizeMessage,
    initTerm,
    hexToRgba,
    withAlpha,
    setLight,
    setDark,
    terminalFactory,
    buildDebugHooks,
    requireElement,
    disableCursor,
    applyThemeStyles,
    applyPageStyles,
    handleThemeChange,
    handleProfileChange,
    handleThemeSelected,
} from "./terminal.ts";
import { isValidHttpProtocol, isValidWsProtocol, isValidPort, isValidUri, MAX_UINT16 } from "./validators.ts";
import { isB3ttyDialog, isB3ttyMenuBar } from "./components.ts";
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
// buildTermOptions
// ---------------------------------------------------------------------------

describe("buildTermOptions", () => {
    const baseConfig = {
        cursorBlink: true,
        fontFamily: "Fira Code",
        fontSize: 14,
        rows: 0,
        columns: 0,
    };

    it("always includes cursorBlink, fontFamily, and fontSize", () => {
        const result = buildTermOptions(baseConfig, {});
        expect(result.cursorBlink).toBe(true);
        expect(result.fontSize).toBe(14);
        expect(result.fontFamily).toContain("Fira Code");
    });

    it("appends fallback font families to the configured font", () => {
        const result = buildTermOptions(baseConfig, {});
        expect(result.fontFamily).toContain("Menlo");
        expect(result.fontFamily).toContain("monospace");
    });

    it("does not set rows when config.rows is 0/falsy", () => {
        const result = buildTermOptions({ ...baseConfig, rows: 0 }, {});
        expect(result).not.toHaveProperty("rows");
    });

    it("sets rows when config.rows is truthy", () => {
        const result = buildTermOptions({ ...baseConfig, rows: 24 }, {});
        expect(result.rows).toBe(24);
    });

    it("does not set cols when config.columns is 0/falsy", () => {
        const result = buildTermOptions({ ...baseConfig, columns: 0 }, {});
        expect(result).not.toHaveProperty("cols");
    });

    it("sets cols when config.columns is truthy", () => {
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
// buildSizeUrl
// ---------------------------------------------------------------------------

describe("buildSizeUrl", () => {
    it("builds a correct HTTP size URL", () => {
        const url = buildSizeUrl("http", "localhost", 8080, 80, 24);
        expect(url).toBe("http://localhost:8080/size?cols=80&rows=24");
    });

    it("builds a correct HTTPS size URL", () => {
        const url = buildSizeUrl("https", "example.com", 8443, 132, 48);
        expect(url).toBe("https://example.com:8443/size?cols=132&rows=48");
    });

    it("handles non-standard ports", () => {
        const url = buildSizeUrl("http", "localhost", 3000, 80, 24);
        expect(url).toContain(":3000/");
    });

    it("reflects cols and rows exactly in the query string", () => {
        const url = buildSizeUrl("http", "localhost", 8080, 1, 1);
        expect(url).toContain("cols=1");
        expect(url).toContain("rows=1");
    });

    it("handles zero cols and rows", () => {
        const url = buildSizeUrl("http", "localhost", 8080, 0, 0);
        expect(url).toBe("http://localhost:8080/size?cols=0&rows=0");
    });
});

// ---------------------------------------------------------------------------
// buildWsUrl
// ---------------------------------------------------------------------------

describe("buildWsUrl", () => {
    it("builds a correct ws URL", () => {
        const url = buildWsUrl("ws", "localhost", 8080);
        expect(url.toString()).toBe("ws://localhost:8080/ws");
    });

    it("builds a correct wss URL", () => {
        const url = buildWsUrl("wss", "example.com", 8443);
        expect(url.toString()).toBe("wss://example.com:8443/ws");
    });

    it("handles non-standard ports", () => {
        const url = buildWsUrl("ws", "localhost", 3000);
        expect(url.toString()).toContain(":3000/");
    });

    it("returns a URL instance", () => {
        const url = buildWsUrl("ws", "localhost", 8080);
        expect(url).toBeInstanceOf(URL);
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
// buildSizeUrl — validation errors
// ---------------------------------------------------------------------------

describe("buildSizeUrl validation", () => {
    it("throws on invalid HTTP protocol", () => {
        expect(() => buildSizeUrl("ws", "localhost", 8080, 80, 24)).toThrow("Invalid HTTP protocol");
    });

    it("throws on invalid URI", () => {
        expect(() => buildSizeUrl("http", "bad uri", 8080, 80, 24)).toThrow("Invalid URI");
    });

    it("throws on invalid port", () => {
        expect(() => buildSizeUrl("http", "localhost", 0, 80, 24)).toThrow("Invalid port");
    });
});

// ---------------------------------------------------------------------------
// buildWsUrl — validation errors
// ---------------------------------------------------------------------------

describe("buildWsUrl validation", () => {
    it("throws on invalid WebSocket protocol", () => {
        expect(() => buildWsUrl("http", "localhost", 8080)).toThrow("Invalid WebSocket protocol");
    });

    it("throws on invalid URI", () => {
        expect(() => buildWsUrl("ws", "bad uri", 8080)).toThrow("Invalid URI");
    });

    it("throws on invalid port", () => {
        expect(() => buildWsUrl("ws", "localhost", 0)).toThrow("Invalid port");
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

    it("writes string data directly to the terminal without decoding", () => {
        const event = { data: "raw string" };
        handleSocketMessage(event, decoder, term);
        expect(term.write).toHaveBeenCalledWith("raw string");
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

    it("handles an empty string message", () => {
        handleSocketMessage({ data: "" }, decoder, term);
        expect(term.write).toHaveBeenCalledWith("");
    });

    it("calls write once per message", () => {
        handleSocketMessage({ data: "a" }, decoder, term);
        handleSocketMessage({ data: "b" }, decoder, term);
        expect(term.write).toHaveBeenCalledTimes(2);
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
        cursorBlink: true,
        rows: 0,
        columns: 0,
        theme: {},
    };

    it("returns a Terminal instance", () => {
        const term = terminalFactory(baseConfig);
        expect(term).toBeInstanceOf(Terminal);
    });

    it("applies cursorBlink from config", () => {
        const term = terminalFactory({ ...baseConfig, cursorBlink: false });
        expect(term.options.cursorBlink).toBe(false);
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

    it("overrides theme background to transparent when backgroundImage is true", () => {
        const term = terminalFactory({
            ...baseConfig,
            backgroundImage: true,
            theme: { background: "#14181d" },
        });
        expect(term.options.theme?.background).toBe("rgba(0, 0, 0, 0)");
    });

    it("sets background to transparent even when no theme background is configured", () => {
        const term = terminalFactory({ ...baseConfig, backgroundImage: true, theme: {} });
        expect(term.options.theme?.background).toBe("rgba(0, 0, 0, 0)");
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
        createElement: mock((_tag: string) => ({ id: "", textContent: "" })),
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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

    it("injects a b3tty-bg-style element into the head when hasBackgroundImage is true", () => {
        const { doc, head } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyThemeStyles({ background: "#14181d" }, true);
        expect(head.appendChild).toHaveBeenCalledTimes(1);
        expect(head._children[0]?.textContent).toContain("xterm-viewport");
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
        cursorBlink: true,
        rows: 0,
        columns: 0,
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

    it("delegates to applyThemeStyles with the config theme and backgroundImage flag", () => {
        const { doc, elements, bodyStyle } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        applyPageStyles({ ...baseConfig, backgroundImage: false });
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
            theme: {},
        });
        await handleThemeChange(makeEvent("dracula"), term, makeMenuBar(), { current: "b3tty-dark" });
        expect(term.options.theme?.foreground).toBe("#f8f8f2");
        expect(term.options.theme?.background).toBe("#282a36");
    });

    it("overrides theme background to transparent when hasBackgroundImage is true", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ hasBackgroundImage: true, background: "#282a36" });
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            cursorBlink: true,
            rows: 0,
            columns: 0,
            theme: {},
        });
        await handleThemeChange(makeEvent("dracula"), term, makeMenuBar(), { current: "b3tty-dark" });
        expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
    });

    it("does not update activeTheme.current when the fetch throws", async () => {
        (globalThis as Record<string, unknown>)["fetch"] = mock(() => Promise.reject(new Error("network error")));
        const term = terminalFactory({
            tls: false,
            uri: "localhost",
            port: 8080,
            fontSize: 14,
            fontFamily: "monospace",
            cursorBlink: true,
            rows: 0,
            columns: 0,
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
            cursorBlink: true,
            rows: 0,
            columns: 0,
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

    it("overrides theme background to transparent when hasBackgroundImage is true", async () => {
        const { doc } = makeDomStub();
        (globalThis as Record<string, unknown>)["document"] = doc;
        stubFetchTheme({ hasBackgroundImage: true, background: "#282a36" });
        const term = terminalFactory(makeConfig());
        await handleThemeSelected(makeEvent("dracula"), term, makeMenuBar(), makePicker(), makeConfig(), {
            current: "b3tty-dark",
        });
        expect(term.options.theme?.background).toBe("rgba(40, 42, 54, 0)");
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
