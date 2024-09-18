package src

var HtmlTemplate = `
<!doctype html>
<html>
    <head>
        <title>{{ if .Title }}{{ .Title }}{{ else }}b3tty{{ end }}</title>
        <link
            rel="stylesheet"
            href="https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.min.css"
        />
        <script src="https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.min.js"></script>
        <script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.min.js"></script>
        <style>
            html,
            body {
                height: 100%;
                margin: 0;
                padding: 0;
            }
            /* #container {
                position: relative;
                height: 50vh;
            } */
            /* #container::after {
                content: "";
                left: 0;
                right: 0;
                bottom: 0;
                height: 50vh;
                position: absolute;
                background: linear-gradient(to bottom, #15191e, #000000 100%);
                z-index: -1;
            } */
            /* html {
                background-image: linear-gradient(to top, #141414, #000000 25%);
                background-size: cover;
                background-repeat: no-repeat;
            } */
            /* body {
                background-color: #000000;
            } */
            body > div:nth-of-type(2) {
                width: 100% !important;
            }
        </style>
    </head>
    <body>
        <div id="container">
            <div id="terminal"></div>
        </div>
    </body>
    <script>
        function getPercentageOfViewport(elem) {
            const viewportHeight = window.innerHeight;
            const boundingBox = elem.getBoundingClientRect();
            const heightPercentage = (boundingBox.height / viewportHeight) * 100;

            return heightPercentage.toFixed(2);
        }

        function useTLS() {
            return {{ if .TLS.Enabled }}true{{ else }}false{{ end }};
        }

        const httProto = useTLS() ? "https": "http";
        const wsProtocol = useTLS() ? "wss" : "ws";

        const term = new window.Terminal({
            cursorBlink: "{{ .CursorBlink }}",
            fontFamily: "{{ .FontFamily }}",
            fontSize: "{{ .FontSize }}",
            {{ if .Rows }}rows: {{ .Rows }},{{ end }}
            {{ if .Columns }}cols: {{ .Columns }},{{ end }}
            // rows: "50",
            {{ if .Theme }}
            theme: {
                foreground: {{ .Theme.Foreground }},
                background: {{ .Theme.Background }},
                black: {{ .Theme.Black }},
                brightBlack: {{ .Theme.BrightBlack }},
                red: {{ .Theme.Red }},
                brightRed: {{ .Theme.BrightRed }},
                green: {{ .Theme.Green }},
                brightGreen: {{ .Theme.BrightGreen }},
                yellow: {{ .Theme.Yellow }},
                brightYellow: {{ .Theme.BrightYellow }},
                blue: {{ .Theme.Blue }},
                brightBlue: {{ .Theme.BrightBlue }},
                magenta: {{ .Theme.Magenta }},
                brightMagenta: {{ .Theme.BrightMagenta }},
                cyan: {{ .Theme.Cyan }},
                brightCyan: {{ .Theme.BrightCyan }},
                white: {{ .Theme.White }},
                brightWhite: {{ .Theme.BrightWhite }},
                selectionForeground: {{ .Theme.SelectionForeground }},
                selectionBackground: {{ .Theme.SelectionBackground }},
            },
            {{ end }}
        });

        const termElement = document.getElementById("terminal")
        term.open(termElement);

        {{ if not .Columns }}
        const fitAddon = new window.FitAddon.FitAddon();
        term.loadAddon(fitAddon);
        fitAddon.fit();
        {{ end }}

        const percentage = getPercentageOfViewport(termElement);
        const styleSheet = document.styleSheets[1];
        {{- if .Theme }}{{ if .Theme.Background -}}
        styleSheet.insertRule('#container::after { content: ""; left: 0; right: 0; bottom: 0; height: ' + (100 - percentage) + 'vh; position: absolute; background: linear-gradient(to bottom, ' + {{ .Theme.Background }} + ', #000000 120%); z-index: -1;}', styleSheet.cssRules.length - 1);
        {{- end }}{{ end -}}

        fetch(
          httProto + '://' + {{ .Uri }} + ':' + {{ .Port }} + '/size?cols=' +
            term.cols +
            "&rows=" +
            term.rows,
        );
        const socket = new WebSocket(wsProtocol + '://' + {{ .Uri }} + ':' + {{ .Port }} + '/ws');
        socket.binaryType = "arraybuffer";

        function init() {
            if (term._initialized) {
                return;
            }

            term._initialized = true;

            // term.prompt = () => {
            //     term.write("\r\n$ ");
            // };
            // prompt(term);
            //

            term.onKey((key) => {
                runCommand(term, key.key);
            });
        }

        socket.onclose = (event) => {
            console.log('Socket closed');
            term.writeln("[exited]");
            alert("Connection closed");
        };

        socket.onopen = (event) => {
            console.log("Socket opened");
        };

        socket.onmessage = (event) => {
            // if (event.data.length >= 512) {
            //     wait(500);
            // }
            if (event.data instanceof ArrayBuffer) {
                const decoder = new TextDecoder("utf-8");
                term.write(decoder.decode(event.data));
            } else {
                term.write(event.data);
            }
        };

        // Apply paste behavior
        term.textarea.addEventListener("paste", (event) => {
            const clipText = (
                event.clipboardData || window.clipboardData
            ).getData("text");
            runCommand(term, clipText);
        });

        // term.attachCustomKeyEventHandler((event) => {
        //     if (event.type === "keydown") {
        //         const sys = window.navigator.platform;
        //         const ctlKey = sys.includes("Mac")
        //             ? event.metaKey
        //             : event.ctrlKey;
        //         if (event.key === "v" && ctlKey == true) {
        //             term.focus();
        //             const text = window.navigator.clipboard
        //                 .readText()
        //                 .then((clipText) => runCommand(term, clipText));
        //         }
        //     }
        // });

        function runCommand(term, command) {
            socket.send(command);
        }

        // function wait(duration) {
        //     console.log("Throttling");
        //     return new Promise((resolve) => {
        //         setTimeout(resolve, duration);
        //     });
        // }

        init();
    </script>
</html>
`
