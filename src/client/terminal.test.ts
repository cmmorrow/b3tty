import { describe, it, expect, beforeEach, mock, spyOn } from "bun:test";
import {
    THEME_KEYS,
    getProtocols,
    buildTheme,
    buildTermOptions,
    buildSizeUrl,
    buildWsUrl,
    buildBackgroundStyleContent,
    handleSocketMessage,
    handleSocketClose,
    sendResizeMessage,
    initTerm,
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

    it("has 20 entries", () => {
        expect(THEME_KEYS).toHaveLength(20);
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

    it("includes all 20 keys when every theme value is provided", () => {
        const full: Record<string, string> = {};
        for (const k of THEME_KEYS) full[k] = "#aabbcc";
        const result = buildTheme(full);
        expect(Object.keys(result)).toHaveLength(20);
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
// buildBackgroundStyleContent
// ---------------------------------------------------------------------------

describe("buildBackgroundStyleContent", () => {
    it("generates a valid CSS rule string", () => {
        const result = buildBackgroundStyleContent("#1e1e1e", 600, 1000);
        expect(result).toContain("#container::after");
        expect(result).toContain("linear-gradient");
        expect(result).toContain("#1e1e1e");
        expect(result).toContain("#000000");
    });

    it("calculates the remaining height percentage correctly", () => {
        // Terminal is 800px tall in a 1000px viewport → 80% used → 20% remaining
        const result = buildBackgroundStyleContent("#000", 800, 1000);
        expect(result).toContain("height: 20%");
    });

    it("rounds the percentage to 2 decimal places", () => {
        // 700 / 900 * 100 = 77.77... → 77.78% used → 22.22% remaining
        const result = buildBackgroundStyleContent("#000", 700, 900);
        expect(result).toContain("height: 22.22%");
    });

    it("produces height: 0% when terminal fills the entire viewport", () => {
        const result = buildBackgroundStyleContent("#000", 1000, 1000);
        expect(result).toContain("height: 0%");
    });

    it("produces height: 100% when terminal has zero height", () => {
        const result = buildBackgroundStyleContent("#000", 0, 1000);
        expect(result).toContain("height: 100%");
    });

    it("embeds the provided background color in the gradient", () => {
        const result = buildBackgroundStyleContent("rgb(30, 30, 46)", 400, 800);
        expect(result).toContain("rgb(30, 30, 46)");
    });

    it("includes required CSS positioning properties", () => {
        const result = buildBackgroundStyleContent("#fff", 300, 600);
        expect(result).toContain("position: absolute");
        expect(result).toContain("left: 0");
        expect(result).toContain("right: 0");
        expect(result).toContain("bottom: 0");
        expect(result).toContain("z-index: 1");
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
    it("writes the [exited] message to the terminal", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn);
        expect(term.writeln).toHaveBeenCalledWith("[exited]");
    });

    it("calls the alert function with 'Connection closed'", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn);
        expect(alertFn).toHaveBeenCalledWith("Connection closed");
    });

    it("calls both writeln and alertFn exactly once", () => {
        const term = makeMockTerm();
        const alertFn = mock((_msg: string) => {});
        handleSocketClose(term, alertFn);
        expect(term.writeln).toHaveBeenCalledTimes(1);
        expect(alertFn).toHaveBeenCalledTimes(1);
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
