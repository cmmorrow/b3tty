import { describe, it, expect, beforeEach, mock, spyOn } from "bun:test";
import { Terminal } from "@xterm/xterm";
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
    terminalFactory,
    buildDebugHooks,
} from "./terminal.ts";
import { isValidHttpProtocol, isValidWsProtocol, isValidPort, isValidUri, MAX_UINT16 } from "./validators.ts";

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
        expect(term.options.rows).toBe(30);
    });

    it("applies cols from config when non-zero", () => {
        const term = terminalFactory({ ...baseConfig, columns: 120 });
        expect(term.options.cols).toBe(120);
    });

    it("does not set allowTransparency when backgroundImage is absent", () => {
        const term = terminalFactory(baseConfig);
        expect(term.options.allowTransparency).toBeFalsy();
    });

    it("does not set allowTransparency when backgroundImage is false", () => {
        const term = terminalFactory({ ...baseConfig, backgroundImage: false });
        expect(term.options.allowTransparency).toBeFalsy();
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
