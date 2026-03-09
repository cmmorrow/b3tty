import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { ImageAddon } from "@xterm/addon-image";

const config = window.B3TTY;

const wsProtocol = config.tls ? "wss" : "ws";
const httpProto = config.tls ? "https" : "http";

document.documentElement.style.setProperty("--b3tty-font-size", `${config.fontSize}pt`);

const termOptions = {
    cursorBlink: config.cursorBlink,
    fontFamily: `${config.fontFamily}, Menlo, DejaVu Sans Mono, Ubuntu Mono, Inconsolata, Fira, monospace`,
    fontSize: config.fontSize,
};

if (config.rows) termOptions.rows = config.rows;
if (config.columns) termOptions.cols = config.columns;

const themeKeys = [
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
];
const theme = {};
for (const k of themeKeys) {
    if (config.theme[k]) theme[k] = config.theme[k];
}
if (Object.keys(theme).length > 0) termOptions.theme = theme;

const term = new Terminal(termOptions);
const termElement = document.getElementById("terminal");
term.open(termElement);

term.loadAddon(new WebLinksAddon());
term.loadAddon(new ImageAddon());

let fitAddon;
if (!config.columns) {
    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    fitAddon.fit();
}

if (config.theme.background) {
    const viewportHeight = window.innerHeight;
    const boundingBox = termElement.getBoundingClientRect();
    const percentage = ((boundingBox.height / viewportHeight) * 100).toFixed(2);
    const style = document.createElement("style");
    style.textContent = `#container::after { content: ""; left: 0; right: 0; bottom: 0; height: ${100 - percentage}%; position: absolute; background: linear-gradient(to bottom, ${config.theme.background}, #000000 120%); z-index: 1; }`;
    document.head.appendChild(style);
}

fetch(`${httpProto}://${config.uri}:${config.port}/size?cols=${term.cols}&rows=${term.rows}`, {
    method: "POST",
});

const socket = new WebSocket(`${wsProtocol}://${config.uri}:${config.port}/ws`);
socket.binaryType = "arraybuffer";

function init() {
    if (term._initialized) return;
    term._initialized = true;

    term.onData((chunk) => socket.send(chunk));

    term.onBell(() => {
        const bell = document.getElementById("bell");
        bell.style.display = "block";
        setTimeout(() => {
            bell.style.display = "none";
        }, 500);
    });
}

socket.onclose = () => {
    console.log("Socket closed");
    term.writeln("[exited]");
    alert("Connection closed");
};

socket.onerror = (event) => {
    console.log("A socket error occurred: ", event);
};

socket.onopen = () => {
    console.log("Socket opened");
};

const decoder = new TextDecoder("utf-8");
socket.onmessage = (event) => {
    if (socket.readyState !== 1) {
        console.log("websocket not ready!");
    }
    if (event.data instanceof ArrayBuffer) {
        term.write(decoder.decode(event.data, { stream: true }));
    } else {
        term.write(event.data);
    }
};

if (!config.columns) {
    term.onResize(({ cols, rows }) => {
        if (socket.readyState === 1) {
            socket.send(JSON.stringify({ type: "resize", cols, rows }));
        }
    });

    window.addEventListener("resize", () => {
        fitAddon.fit();
    });
}

init();
