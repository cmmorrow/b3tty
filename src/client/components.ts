/**
 * Interface for the b3tty-dialog web component. The concrete class is defined
 * conditionally so that importing this module in non-browser environments (e.g.
 * bun test) does not throw a ReferenceError for HTMLElement.
 */
export interface B3ttyDialog {
    show(message: string): void;
    hide(): void;
}

if (typeof HTMLElement !== "undefined") {
    class B3ttyDialogImpl extends HTMLElement implements B3ttyDialog {
        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });
            shadow.innerHTML = `
                <style>
                    :host { display: none; }
                    :host([open]) { display: block; }
                    .backdrop {
                        position: fixed;
                        inset: 0;
                        background: rgba(0, 0, 0, 0.5);
                        z-index: 10000;
                        display: flex;
                        align-items: center;
                        justify-content: center;
                    }
                    .modal {
                        background: #d0d0d0;
                        border-radius: 8px;
                        padding: 28px 36px;
                        display: flex;
                        flex-direction: column;
                        align-items: center;
                        gap: 20px;
                    }
                    p {
                        margin: 0;
                        color: #111;
                        font-family: sans-serif;
                        font-size: 14px;
                    }
                    button {
                        padding: 5px 20px;
                        border-radius: 4px;
                        border: 1px solid #999;
                        background: #bbb;
                        cursor: pointer;
                        font-size: 14px;
                        font-family: sans-serif;
                    }
                    button:hover {
                        background: #a8a8a8;
                    }
                </style>
                <div class="backdrop">
                    <div class="modal" role="dialog" aria-modal="true">
                        <p></p>
                        <button>OK</button>
                    </div>
                </div>
            `;
            shadow.querySelector("button")!.addEventListener("click", () => this.hide());
        }

        show(message: string): void {
            this.shadowRoot!.querySelector("p")!.textContent = message;
            this.setAttribute("open", "");
        }

        hide(): void {
            this.removeAttribute("open");
        }
    }

    customElements.define("b3tty-dialog", B3ttyDialogImpl);

    type Palette = {
        bg: string;
        fg: string;
        selBg: string;
        normal: string[];
        bright: string[];
    };

    async function fetchPalette(name: string): Promise<Palette> {
        const res = await fetch(`/theme?name=${name}`);
        return res.json() as Promise<Palette>;
    }

    class B3ttyThemeSelectorImpl extends HTMLElement {
        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });

            const swatchRow = (colors: string[]): HTMLDivElement => {
                const row = document.createElement("div");
                row.className = "swatch-row";
                for (const color of colors) {
                    const s = document.createElement("div");
                    s.className = "swatch";
                    s.style.background = color;
                    row.appendChild(s);
                }
                return row;
            };

            const paletteCard = (value: string, label: string, p: Palette): HTMLLabelElement => {
                const card = document.createElement("label");
                card.className = "card";

                const header = document.createElement("div");
                header.className = "card-header";
                const radio = document.createElement("input");
                radio.type = "radio";
                radio.name = "theme";
                radio.id = value;
                radio.value = value;
                const labelSpan = document.createElement("span");
                labelSpan.textContent = label;
                header.appendChild(radio);
                header.appendChild(labelSpan);

                const terminal = document.createElement("div");
                terminal.className = "terminal";
                terminal.style.background = p.bg;

                const titlebar = document.createElement("div");
                titlebar.className = "titlebar";
                for (const color of ["#ff5f57", "#ffbd2e", "#28c841"]) {
                    const dot = document.createElement("div");
                    dot.className = "dot";
                    dot.style.background = color;
                    titlebar.appendChild(dot);
                }

                const preview = document.createElement("div");
                preview.className = "preview-text";
                preview.style.color = p.fg;
                preview.appendChild(document.createTextNode("lorem "));
                const sel = document.createElement("span");
                sel.className = "sel";
                sel.style.background = p.selBg;
                sel.style.color = p.fg;
                sel.textContent = "ipsum";
                preview.appendChild(sel);

                terminal.appendChild(titlebar);
                terminal.appendChild(preview);
                terminal.appendChild(swatchRow(p.normal));
                terminal.appendChild(swatchRow(p.bright));

                card.appendChild(header);
                card.appendChild(terminal);
                return card;
            };

            const skipCard = (): HTMLLabelElement => {
                const card = document.createElement("label");
                card.className = "card skip-card";
                const header = document.createElement("div");
                header.className = "card-header";
                const radio = document.createElement("input");
                radio.type = "radio";
                radio.name = "theme";
                radio.id = "skip";
                radio.value = "skip";
                const labelSpan = document.createElement("span");
                labelSpan.textContent = "No theme";
                header.appendChild(radio);
                header.appendChild(labelSpan);
                const note = document.createElement("p");
                note.className = "skip-note";
                note.textContent = "Configure a theme later in conf.yaml.";
                card.appendChild(header);
                card.appendChild(note);
                return card;
            };

            const style = document.createElement("style");
            style.textContent = `
                :host { display: block; }
                .backdrop {
                    position: fixed; inset: 0;
                    background: rgba(0,0,0,0.72);
                    z-index: 10000;
                    display: flex; align-items: center; justify-content: center;
                }
                .modal {
                    background: #e0e0e0;
                    border-radius: 10px;
                    padding: 28px 32px;
                    display: flex; flex-direction: column; align-items: center;
                    gap: 20px;
                    box-shadow: 0 8px 40px rgba(0,0,0,0.55);
                }
                .subtitle { margin: 0; font-size: 13px; font-family: sans-serif; color: #555; text-align: center; }
                .options { display: flex; gap: 14px; flex-wrap: wrap; justify-content: center; }
                .card {
                    display: flex; flex-direction: column; gap: 10px;
                    padding: 12px; border-radius: 8px;
                    border: 2px solid transparent;
                    background: #cecece;
                    cursor: pointer;
                    transition: border-color 0.15s;
                    user-select: none;
                }
                .card:has(input:checked) { border-color: #444; }
                .card-header {
                    display: flex; align-items: center; gap: 7px;
                    font-family: sans-serif; font-size: 13px; font-weight: 600; color: #222;
                }
                input[type=radio] { cursor: pointer; accent-color: #444; }
                .terminal {
                    border-radius: 6px;
                    padding: 10px 10px 8px;
                    display: flex; flex-direction: column; gap: 7px;
                    font-family: monospace; font-size: 11px;
                    box-shadow: 0 2px 10px rgba(0,0,0,0.35);
                    min-width: 196px;
                }
                .titlebar { display: flex; gap: 5px; margin-bottom: 1px; }
                .dot { width: 9px; height: 9px; border-radius: 50%; }
                .preview-text { padding: 1px 2px; line-height: 1.5; letter-spacing: 0.01em; }
                .sel { padding: 0 2px; border-radius: 2px; }
                .swatch-row { display: flex; gap: 3px; }
                .swatch {
                    width: 20px; height: 20px; border-radius: 4px;
                    box-shadow: inset 0 0 0 1px rgba(128,128,128,0.25);
                }
                .skip-card { justify-content: center; min-width: 196px; }
                .skip-note {
                    margin: 0; font-family: sans-serif; font-size: 12px; color: #666;
                    max-width: 180px; line-height: 1.5;
                }
                .ok-btn {
                    padding: 9px 36px; border-radius: 5px; border: none;
                    background: #444; color: #fff;
                    font-size: 14px; font-family: sans-serif;
                    cursor: pointer; transition: background 0.15s;
                }
                .ok-btn:disabled { background: #aaa; cursor: not-allowed; }
                .ok-btn:not(:disabled):hover { background: #222; }
            `;

            const backdrop = document.createElement("div");
            backdrop.className = "backdrop";
            const modal = document.createElement("div");
            modal.className = "modal";
            modal.setAttribute("role", "dialog");
            modal.setAttribute("aria-modal", "true");

            const subtitle = document.createElement("p");
            subtitle.className = "subtitle";
            subtitle.textContent = "Choose a default theme to get started, or skip to configure one later.";

            const options = document.createElement("div");
            options.className = "options";
            options.appendChild(skipCard());

            Promise.all([fetchPalette("dark"), fetchPalette("light")]).then(([dark, light]) => {
                options.prepend(paletteCard("light", "Light", light));
                options.prepend(paletteCard("dark", "Dark", dark));
            });

            const okBtn = document.createElement("button");
            okBtn.className = "ok-btn";
            okBtn.id = "ok-btn";
            okBtn.textContent = "OK";
            okBtn.disabled = true;

            modal.appendChild(subtitle);
            modal.appendChild(options);
            modal.appendChild(okBtn);
            backdrop.appendChild(modal);
            shadow.appendChild(style);
            shadow.appendChild(backdrop);

            options.addEventListener("change", () => {
                okBtn.disabled = false;
            });

            okBtn.addEventListener("click", async () => {
                const checked = shadow.querySelector<HTMLInputElement>("input[name=theme]:checked");
                if (!checked) return;
                await fetch("/save-config", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ theme: checked.value }),
                });
                window.location.reload();
            });
        }
    }

    customElements.define("b3tty-theme-selector", B3ttyThemeSelectorImpl);
}
