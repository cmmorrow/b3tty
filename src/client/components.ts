import { getThemePalette, postSaveConfig } from "./api.ts";
import type { Palette } from "./types.ts";

/**
 * Interface for the b3tty-dialog web component. The concrete class is defined
 * conditionally so that importing this module in non-browser environments (e.g.
 * bun test) does not throw a ReferenceError for HTMLElement.
 */
export interface B3ttyDialog {
    show(message: string): void;
    hide(): void;
}

/**
 * Colors used to style the menu bar: bg is applied as the bar's background,
 * fg as its text/icon color.
 */
export interface MenuBarColors {
    bg: string;
    fg: string;
}

/**
 * Interface for the b3tty-menu-bar web component.
 */
export interface B3ttyMenuBar {
    setup(themeNames: string[], profileNames: string[], colors: MenuBarColors): void;
    updateColors(colors: MenuBarColors): void;
}

/**
 * Interface for the b3tty-theme-picker web component.
 */
export interface B3ttyThemePicker {
    open(themeNames: string[]): void;
    close(): void;
}

/**
 * Interface for the b3tty-palette-card web component. Call setup() to
 * populate the card with a theme name, display label, and palette data.
 * The selected property reflects the [selected] attribute on the host.
 * When clicked, the card sets its own [selected] attribute and dispatches
 * a composed "b3tty-card-select" CustomEvent with detail { value: string }.
 *
 * Style the card from a parent shadow DOM by setting CSS custom properties
 * on b3tty-palette-card elements:
 *   --palette-card-padding         (default: 12px)
 *   --palette-card-gap             (default: 10px)
 *   --palette-card-overflow        (default: visible)
 *   --palette-card-header-bg       (default: transparent)
 *   --palette-card-header-padding  (default: 0)
 *   --palette-card-header-font-size (default: 13px)
 *   --palette-card-terminal-gap    (default: 7px)
 *   --palette-card-terminal-shadow (default: 0 2px 10px rgba(0,0,0,0.35))
 *   --palette-card-terminal-min-width (default: 196px)
 */
export interface B3ttyPaletteCard {
    setup(value: string, label: string, palette: Palette): void;
    readonly selected: boolean;
    readonly value: string;
}

/**
 * Returns true when el exposes the B3ttyDialog contract (show/hide methods).
 * Use this instead of `as unknown as B3ttyDialog` to preserve type safety.
 */
export function isB3ttyDialog(el: Element): el is HTMLElement & B3ttyDialog {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["show"] === "function" && typeof r["hide"] === "function";
}

/**
 * Returns true when el exposes the B3ttyMenuBar contract (setup/updateColors methods).
 * Use this instead of `as unknown as B3ttyMenuBar` to preserve type safety.
 */
export function isB3ttyMenuBar(el: Element): el is HTMLElement & B3ttyMenuBar {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["setup"] === "function" && typeof r["updateColors"] === "function";
}

/**
 * Returns true when el exposes the B3ttyThemePicker contract (open/close methods).
 */
export function isB3ttyThemePicker(el: Element): el is HTMLElement & B3ttyThemePicker {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["open"] === "function" && typeof r["close"] === "function";
}

/**
 * Returns true when el exposes the B3ttyPaletteCard contract (setup/value/selected).
 */
export function isB3ttyPaletteCard(el: Element): el is HTMLElement & B3ttyPaletteCard {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["setup"] === "function" && "value" in r && "selected" in r;
}

function formatThemeName(name: string): string {
    return name
        .split("-")
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(" ");
}

if (typeof HTMLElement !== "undefined") {
    // Defined first so it is available when B3ttyThemeSelectorImpl and
    // B3ttyThemePickerImpl constructors call document.createElement("b3tty-palette-card").
    class B3ttyPaletteCardImpl extends HTMLElement implements B3ttyPaletteCard {
        #shadow: ShadowRoot;
        #value: string = "";

        get selected(): boolean {
            return this.hasAttribute("selected");
        }

        get value(): string {
            return this.#value;
        }

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            const style = document.createElement("style");
            style.textContent = `
                :host {
                    display: flex;
                    flex-direction: column;
                    gap: var(--palette-card-gap, 10px);
                    padding: var(--palette-card-padding, 12px);
                    border-radius: 8px;
                    border: 2px solid transparent;
                    background: #cecece;
                    cursor: pointer;
                    transition: border-color 0.15s;
                    user-select: none;
                    overflow: var(--palette-card-overflow, visible);
                    box-sizing: border-box;
                }
                :host([selected]) { border-color: #444; }
                .card-header {
                    display: flex; align-items: center; gap: 7px;
                    padding: var(--palette-card-header-padding, 0);
                    font-family: sans-serif;
                    font-size: var(--palette-card-header-font-size, 13px);
                    font-weight: 600; color: #222;
                    background: var(--palette-card-header-bg, transparent);
                }
                input[type=radio] { cursor: pointer; accent-color: #444; }
                .terminal {
                    border-radius: 6px;
                    padding: 10px 10px 8px;
                    display: flex; flex-direction: column;
                    gap: var(--palette-card-terminal-gap, 7px);
                    font-family: monospace; font-size: 11px;
                    box-shadow: var(--palette-card-terminal-shadow, 0 2px 10px rgba(0,0,0,0.35));
                    min-width: var(--palette-card-terminal-min-width, 196px);
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
            `;
            this.#shadow.appendChild(style);

            this.addEventListener("click", () => {
                if (!this.#value) return;
                this.setAttribute("selected", "");
                this.dispatchEvent(
                    new CustomEvent("b3tty-card-select", {
                        detail: { value: this.#value },
                        bubbles: true,
                        composed: true,
                    })
                );
            });
        }

        setup(value: string, label: string, palette: Palette): void {
            this.#value = value;

            const style = this.#shadow.querySelector("style")!;
            while (this.#shadow.lastChild !== style) {
                this.#shadow.removeChild(this.#shadow.lastChild!);
            }

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
            terminal.style.background = palette.bg;

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
            preview.style.color = palette.fg;
            preview.appendChild(document.createTextNode("lorem "));
            const sel = document.createElement("span");
            sel.className = "sel";
            sel.style.background = palette.selBg;
            sel.style.color = palette.fg;
            sel.textContent = "ipsum";
            preview.appendChild(sel);
            const cursor = document.createElement("span");
            cursor.textContent = "\u00a0";
            cursor.style.background = palette.cursor;
            preview.appendChild(cursor);

            terminal.appendChild(titlebar);
            terminal.appendChild(preview);
            terminal.appendChild(this.#swatchRow(palette.normal));
            terminal.appendChild(this.#swatchRow(palette.bright));

            this.#shadow.appendChild(header);
            this.#shadow.appendChild(terminal);
        }

        #swatchRow(colors: string[]): HTMLDivElement {
            const row = document.createElement("div");
            row.className = "swatch-row";
            for (const color of colors) {
                const s = document.createElement("div");
                s.className = "swatch";
                s.style.background = color;
                row.appendChild(s);
            }
            return row;
        }
    }

    customElements.define("b3tty-palette-card", B3ttyPaletteCardImpl);

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

    class B3ttyThemeSelectorImpl extends HTMLElement {
        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });

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

            let selectedValue: string | null = null;

            // Palette card selection — b3tty-card-select is composed so it crosses
            // the palette card's shadow boundary and reaches this listener.
            options.addEventListener("b3tty-card-select", (e: Event) => {
                for (const card of Array.from(options.querySelectorAll("b3tty-palette-card"))) {
                    if (card !== e.target) card.removeAttribute("selected");
                }
                const skipRadio = shadow.querySelector<HTMLInputElement>("#skip");
                if (skipRadio) skipRadio.checked = false;
                selectedValue = (e as CustomEvent<{ value: string }>).detail.value;
                okBtn.disabled = false;
            });

            // Skip card selection via its radio input.
            options.addEventListener("change", (e: Event) => {
                const target = e.target as HTMLInputElement;
                if (target.id === "skip") {
                    for (const card of Array.from(options.querySelectorAll("b3tty-palette-card"))) {
                        card.removeAttribute("selected");
                    }
                    selectedValue = "skip";
                    okBtn.disabled = false;
                }
            });

            Promise.all([getThemePalette("b3tty-dark"), getThemePalette("b3tty-light")])
                .then(([dark, light]) => {
                    const lightCard = document.createElement("b3tty-palette-card");
                    (lightCard as unknown as B3ttyPaletteCard).setup("b3tty-light", "B3tty Light", light);
                    const darkCard = document.createElement("b3tty-palette-card");
                    (darkCard as unknown as B3ttyPaletteCard).setup("b3tty-dark", "B3tty Dark", dark);
                    options.prepend(lightCard);
                    options.prepend(darkCard);
                })
                .catch(() => {
                    // Palette cards remain absent; the user can still select "No theme".
                });

            okBtn.addEventListener("click", async () => {
                if (!selectedValue) return;
                await postSaveConfig(selectedValue);
                window.location.reload();
            });
        }
    }

    customElements.define("b3tty-theme-selector", B3ttyThemeSelectorImpl);

    class B3ttyMenuBarImpl extends HTMLElement implements B3ttyMenuBar {
        #hideTimer: ReturnType<typeof setTimeout> | null = null;
        #activeSection: string | null = null;
        #shadow: ShadowRoot;
        #menubar: HTMLDivElement;
        #trigger: HTMLDivElement;

        #onDocumentPointerDown = (e: PointerEvent): void => {
            if (this.#activeSection === null) return;
            if (!e.composedPath().includes(this)) {
                this.#toggleSection(this.#activeSection);
            }
        };

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            const style = document.createElement("style");
            style.textContent = `
                :host {
                    display: block;
                    height: 0;
                    overflow: visible;
                    flex-shrink: 0;
                }
                :host([open]) {
                    height: 32px;
                }
                .trigger {
                    position: fixed;
                    top: 0;
                    left: 50%;
                    transform: translateX(-50%);
                    width: 100px;
                    height: 6px;
                    background: #808080;
                    border-radius: 0 0 4px 4px;
                    cursor: pointer;
                    z-index: 1000;
                    opacity: 0.5;
                    transition: opacity 0.15s;
                }
                .trigger:hover {
                    opacity: 1;
                }
                :host([open]) .trigger {
                    display: none;
                }
                .menubar {
                    display: none;
                    width: 100%;
                    height: 32px;
                    box-sizing: border-box;
                    flex-direction: row;
                    align-items: stretch;
                    background: var(--menu-bg, #fff);
                    color: var(--menu-fg, #000);
                    font-family: sans-serif;
                    font-size: 13px;
                    user-select: none;
                    position: relative;
                }
                :host([open]) .menubar {
                    display: flex;
                }
                .section {
                    position: relative;
                }
                .section-label {
                    display: flex;
                    align-items: center;
                    padding: 0 14px;
                    height: 32px;
                    cursor: pointer;
                    box-sizing: border-box;
                    color: var(--menu-fg, #000);
                }
                .section-label:hover,
                .section.active .section-label {
                    filter: brightness(0.85);
                    background: var(--menu-bg, #fff);
                }
                .dropdown {
                    display: none;
                    position: absolute;
                    top: 32px;
                    left: 0;
                    min-width: 140px;
                    background: var(--menu-bg, #fff);
                    color: var(--menu-fg, #000);
                    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
                    flex-direction: column;
                    z-index: 1001;
                }
                .section.active .dropdown {
                    display: flex;
                }
                .menu-item {
                    padding: 7px 16px;
                    cursor: pointer;
                    white-space: nowrap;
                    color: var(--menu-fg, #000);
                }
                .menu-item:hover {
                    filter: brightness(0.85);
                    background: var(--menu-bg, #fff);
                }
                .menu-separator {
                    height: 1px;
                    background: var(--menu-fg, #000);
                    opacity: 0.2;
                    margin: 2px 8px;
                }
            `;

            this.#trigger = document.createElement("div");
            this.#trigger.className = "trigger";

            this.#menubar = document.createElement("div");
            this.#menubar.className = "menubar";

            this.#shadow.appendChild(style);
            this.#shadow.appendChild(this.#trigger);
            this.#shadow.appendChild(this.#menubar);

            this.#trigger.addEventListener("mouseenter", () => this.#open());
            this.#menubar.addEventListener("mouseenter", () => this.#resetTimer());
        }

        connectedCallback(): void {
            document.addEventListener("pointerdown", this.#onDocumentPointerDown);
        }

        disconnectedCallback(): void {
            document.removeEventListener("pointerdown", this.#onDocumentPointerDown);
        }

        setup(themeNames: string[], profileNames: string[], colors: MenuBarColors): void {
            this.updateColors(colors);
            this.#menubar.innerHTML = "";
            this.#activeSection = null;

            // Always add the Themes section — even when themeNames is empty — so
            // "Select Theme…" is always accessible from the menu bar.
            this.#menubar.appendChild(this.#buildSection("themes", "Themes", themeNames));
            if (profileNames.length > 0) {
                this.#menubar.appendChild(this.#buildSection("profiles", "Profiles", profileNames));
            }
        }

        updateColors(colors: MenuBarColors): void {
            (this.#shadow.host as HTMLElement).style.setProperty("--menu-bg", colors.bg);
            (this.#shadow.host as HTMLElement).style.setProperty("--menu-fg", colors.fg);
        }

        #buildSection(type: string, label: string, items: string[]): HTMLDivElement {
            const section = document.createElement("div");
            section.className = "section";
            section.dataset["section"] = type;

            const sectionLabel = document.createElement("span");
            sectionLabel.className = "section-label";
            sectionLabel.textContent = label;

            const dropdown = document.createElement("div");
            dropdown.className = "dropdown";

            if (type === "themes") {
                const selectItem = document.createElement("div");
                selectItem.className = "menu-item";
                selectItem.textContent = "Select Theme\u2026";
                selectItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(new CustomEvent("b3tty-open-theme-selector", { bubbles: true, composed: true }));
                    this.#close();
                });
                dropdown.appendChild(selectItem);
                if (items.length > 0) {
                    const sep = document.createElement("div");
                    sep.className = "menu-separator";
                    dropdown.appendChild(sep);
                }
            }

            for (const name of items) {
                const item = document.createElement("div");
                item.className = "menu-item";
                item.textContent = name;
                item.addEventListener("click", (e) => {
                    e.stopPropagation();
                    const eventName = type === "themes" ? "b3tty-theme-change" : "b3tty-profile-change";
                    this.dispatchEvent(
                        new CustomEvent(eventName, {
                            detail: { name },
                            bubbles: true,
                            composed: true,
                        })
                    );
                    if (type === "themes") {
                        this.#toggleSection(type);
                    } else {
                        this.#close();
                    }
                });
                dropdown.appendChild(item);
            }

            sectionLabel.addEventListener("click", (e) => {
                e.stopPropagation();
                this.#toggleSection(type);
            });

            section.appendChild(sectionLabel);
            section.appendChild(dropdown);
            return section;
        }

        #open(): void {
            this.style.height = "32px";
            this.setAttribute("open", "");
            this.dispatchEvent(new CustomEvent("b3tty-menubar-open", { bubbles: true, composed: true }));
            this.#resetTimer();
        }

        #close(): void {
            if (this.#hideTimer !== null) {
                clearTimeout(this.#hideTimer);
                this.#hideTimer = null;
            }
            this.#activeSection = null;
            for (const s of Array.from(this.#menubar.querySelectorAll(".section.active"))) {
                s.classList.remove("active");
            }
            this.style.height = "0px";
            this.removeAttribute("open");
            this.dispatchEvent(new CustomEvent("b3tty-menubar-close", { bubbles: true, composed: true }));
        }

        #resetTimer(): void {
            if (this.#hideTimer !== null) clearTimeout(this.#hideTimer);
            this.#hideTimer = setTimeout(() => this.#close(), 5000);
        }

        #toggleSection(type: string): void {
            this.#resetTimer();
            const section = this.#menubar.querySelector<HTMLDivElement>(`.section[data-section="${type}"]`);
            if (!section) return;
            if (this.#activeSection === type) {
                section.classList.remove("active");
                this.#activeSection = null;
            } else {
                for (const s of Array.from(this.#menubar.querySelectorAll(".section.active"))) {
                    s.classList.remove("active");
                }
                section.classList.add("active");
                this.#activeSection = type;
            }
        }
    }

    customElements.define("b3tty-menu-bar", B3ttyMenuBarImpl);

    class B3ttyThemePickerImpl extends HTMLElement implements B3ttyThemePicker {
        #shadow: ShadowRoot;
        #cards: HTMLDivElement;
        #okBtn: HTMLButtonElement;

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            const style = document.createElement("style");
            style.textContent = `
                :host { display: none; }
                :host([open]) { display: block; }
                .overlay {
                    position: fixed; inset: 0;
                    background: rgba(0,0,0,0.72);
                    z-index: 10000;
                    display: flex; align-items: center; justify-content: center;
                    padding: 20px; box-sizing: border-box;
                }
                .modal {
                    background: #e0e0e0;
                    border-radius: 10px;
                    padding: 24px 28px 20px;
                    display: flex; flex-direction: column; gap: 16px;
                    max-height: 85vh; max-width: 1000px; width: 100%;
                    box-sizing: border-box;
                    box-shadow: 0 8px 40px rgba(0,0,0,0.55);
                }
                h2 { margin: 0; font-family: sans-serif; font-size: 16px; font-weight: 600; color: #111; }
                .cards {
                    display: grid;
                    grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
                    gap: 12px;
                    overflow-y: auto; flex: 1; min-height: 0;
                    padding: 4px 2px;
                }
                b3tty-palette-card {
                    --palette-card-padding: 0;
                    --palette-card-gap: 0;
                    --palette-card-overflow: hidden;
                    --palette-card-header-bg: #c8c8c8;
                    --palette-card-header-padding: 8px 10px;
                    --palette-card-header-font-size: 12px;
                    --palette-card-terminal-gap: 6px;
                    --palette-card-terminal-shadow: none;
                    --palette-card-terminal-min-width: 0;
                }
                .loading {
                    font-family: sans-serif; font-size: 13px; color: #555;
                    text-align: center; padding: 20px; grid-column: 1 / -1;
                }
                .actions { display: flex; justify-content: flex-end; gap: 10px; }
                .cancel-btn {
                    padding: 8px 20px; border-radius: 5px;
                    border: 1px solid #aaa; background: #c8c8c8;
                    font-size: 14px; font-family: sans-serif; cursor: pointer;
                }
                .cancel-btn:hover { background: #b8b8b8; }
                .ok-btn {
                    padding: 8px 28px; border-radius: 5px; border: none;
                    background: #444; color: #fff;
                    font-size: 14px; font-family: sans-serif; cursor: pointer;
                }
                .ok-btn:disabled { background: #aaa; cursor: not-allowed; }
                .ok-btn:not(:disabled):hover { background: #222; }
            `;

            const overlay = document.createElement("div");
            overlay.className = "overlay";
            const modal = document.createElement("div");
            modal.className = "modal";
            modal.setAttribute("role", "dialog");
            modal.setAttribute("aria-modal", "true");

            const title = document.createElement("h2");
            title.textContent = "Select a Theme";

            this.#cards = document.createElement("div");
            this.#cards.className = "cards";
            const loading = document.createElement("div");
            loading.className = "loading";
            loading.textContent = "Loading themes\u2026";
            this.#cards.appendChild(loading);

            const actions = document.createElement("div");
            actions.className = "actions";
            const cancelBtn = document.createElement("button");
            cancelBtn.className = "cancel-btn";
            cancelBtn.textContent = "Cancel";
            this.#okBtn = document.createElement("button");
            this.#okBtn.className = "ok-btn";
            this.#okBtn.textContent = "OK";
            this.#okBtn.disabled = true;
            actions.appendChild(cancelBtn);
            actions.appendChild(this.#okBtn);

            modal.appendChild(title);
            modal.appendChild(this.#cards);
            modal.appendChild(actions);
            overlay.appendChild(modal);
            this.#shadow.appendChild(style);
            this.#shadow.appendChild(overlay);

            // b3tty-card-select is composed, so it crosses the palette card's
            // shadow boundary and bubbles up to this listener on #cards.
            this.#cards.addEventListener("b3tty-card-select", (e: Event) => {
                for (const card of Array.from(this.#cards.querySelectorAll("b3tty-palette-card"))) {
                    if (card !== e.target) card.removeAttribute("selected");
                }
                this.#okBtn.disabled = false;
            });

            cancelBtn.addEventListener("click", () => this.close());
            this.#okBtn.addEventListener("click", () => {
                const selected = this.#cards.querySelector<HTMLElement>("b3tty-palette-card[selected]");
                if (!selected) return;
                this.dispatchEvent(
                    new CustomEvent("b3tty-theme-selected", {
                        detail: { name: (selected as unknown as B3ttyPaletteCard).value },
                        bubbles: true,
                        composed: true,
                    })
                );
            });
        }

        open(themeNames: string[]): void {
            this.#okBtn.disabled = true;
            this.#cards.innerHTML = "";
            const loading = document.createElement("div");
            loading.className = "loading";
            loading.textContent = "Loading themes\u2026";
            this.#cards.appendChild(loading);
            this.setAttribute("open", "");

            Promise.all(
                themeNames.map((name) =>
                    getThemePalette(name)
                        .then((p) => ({ name, palette: p as Palette | null }))
                        .catch(() => ({ name, palette: null as Palette | null }))
                )
            ).then((results) => {
                this.#cards.innerHTML = "";
                for (const { name, palette } of results) {
                    if (palette) {
                        const card = document.createElement("b3tty-palette-card");
                        (card as unknown as B3ttyPaletteCard).setup(name, formatThemeName(name), palette);
                        this.#cards.appendChild(card);
                    }
                }
            });
        }

        close(): void {
            this.removeAttribute("open");
            this.#okBtn.disabled = true;
        }
    }

    customElements.define("b3tty-theme-picker", B3ttyThemePickerImpl);
}
