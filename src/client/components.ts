import {
    getThemePalette,
    getThemeConfig,
    postEditTheme,
    postSaveConfig,
    getProfileConfig,
    postEditProfile,
    postDeleteProfile,
} from "./api.ts";
import type { Palette, ProfileConfig } from "./types.ts";
import { isValidThemeColor } from "./validators.ts";

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
 * Interface for the b3tty-theme-editor web component.
 */
export interface B3ttyThemeEditor {
    open(themeNames: string[], builtinThemeNames?: string[]): void;
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
 * Returns true when el is a b3tty-theme-editor element.
 */
export function isB3ttyThemeEditor(el: Element): el is HTMLElement & B3ttyThemeEditor {
    return el.tagName.toLowerCase() === "b3tty-theme-editor";
}

/**
 * Interface for the b3tty-profile-editor web component.
 */
export interface B3ttyProfileEditor {
    open(profileNames: string[]): void;
    close(): void;
}

/**
 * Returns true when el is a b3tty-profile-editor element.
 */
export function isB3ttyProfileEditor(el: Element): el is HTMLElement & B3ttyProfileEditor {
    return el.tagName.toLowerCase() === "b3tty-profile-editor";
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

const BUTTON_STYLES = `
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
    .ok-btn:not(:disabled):hover { background: #222; }`;

const PALETTE_CARD_VARS = `
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
    }`;

function fetchPaletteCards(
    themeNames: string[]
): Promise<Array<{ card: HTMLElement; name: string; palette: Palette }>> {
    return Promise.all(
        themeNames.map((name) =>
            getThemePalette(name)
                .then((p) => ({ name, palette: p as Palette | null }))
                .catch(() => ({ name, palette: null as Palette | null }))
        )
    ).then((results) => {
        const entries: Array<{ card: HTMLElement; name: string; palette: Palette }> = [];
        for (const { name, palette } of results) {
            if (palette) {
                const card = document.createElement("b3tty-palette-card");
                (card as unknown as B3ttyPaletteCard).setup(name, formatThemeName(name), palette);
                entries.push({ card, name, palette });
            }
        }
        return entries;
    });
}

if (typeof HTMLElement !== "undefined") {
    // Defined first so it is available when B3ttyThemeSelectorImpl and
    // B3ttyThemePickerImpl constructors call document.createElement("b3tty-palette-card").
    class B3ttyPaletteCardImpl extends HTMLElement implements B3ttyPaletteCard {
        #shadow: ShadowRoot;
        #value: string = "";
        #radio: HTMLInputElement | null = null;

        static get observedAttributes() {
            return ["selected"];
        }

        get selected(): boolean {
            return this.hasAttribute("selected");
        }

        get value(): string {
            return this.#value;
        }

        attributeChangedCallback(name: string, _oldValue: string | null, newValue: string | null): void {
            if (name === "selected" && this.#radio) {
                this.#radio.checked = newValue !== null;
            }
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
            radio.checked = this.hasAttribute("selected");
            this.#radio = radio;
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

            // Always add both sections so "Edit Theme…" and "Edit Profile…" are
            // always accessible regardless of how many themes/profiles are configured.
            this.#menubar.appendChild(this.#buildSection("themes", "Themes", themeNames));
            this.#menubar.appendChild(this.#buildSection("profiles", "Profiles", profileNames));
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
                const editItem = document.createElement("div");
                editItem.className = "menu-item";
                editItem.textContent = "Edit Theme…";
                editItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(new CustomEvent("b3tty-open-theme-editor", { bubbles: true, composed: true }));
                    this.#close();
                });
                dropdown.appendChild(editItem);
                if (items.length > 0) {
                    const sep = document.createElement("div");
                    sep.className = "menu-separator";
                    dropdown.appendChild(sep);
                }
            }

            if (type === "profiles") {
                const editProfileItem = document.createElement("div");
                editProfileItem.className = "menu-item";
                editProfileItem.textContent = "Edit Profile…";
                editProfileItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(new CustomEvent("b3tty-open-profile-editor", { bubbles: true, composed: true }));
                    this.#close();
                });
                dropdown.appendChild(editProfileItem);
                const switchable = items.filter((n) => n !== "default");
                if (switchable.length > 0) {
                    const sep = document.createElement("div");
                    sep.className = "menu-separator";
                    dropdown.appendChild(sep);
                    for (const name of switchable) {
                        const item = document.createElement("div");
                        item.className = "menu-item";
                        item.textContent = name;
                        item.addEventListener("click", (e) => {
                            e.stopPropagation();
                            this.dispatchEvent(
                                new CustomEvent("b3tty-profile-change", {
                                    detail: { name },
                                    bubbles: true,
                                    composed: true,
                                })
                            );
                            this.#close();
                        });
                        dropdown.appendChild(item);
                    }
                }
            } else {
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
                ${PALETTE_CARD_VARS}
                .loading {
                    font-family: sans-serif; font-size: 13px; color: #555;
                    text-align: center; padding: 20px; grid-column: 1 / -1;
                }
                .actions { display: flex; justify-content: flex-end; gap: 10px; }
                ${BUTTON_STYLES}
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

            fetchPaletteCards(themeNames).then((entries) => {
                this.#cards.innerHTML = "";
                for (const { card } of entries) this.#cards.appendChild(card);
            });
        }

        close(): void {
            this.removeAttribute("open");
            this.#okBtn.disabled = true;
        }
    }

    customElements.define("b3tty-theme-picker", B3ttyThemePickerImpl);

    class B3ttyThemeEditorImpl extends HTMLElement implements B3ttyThemeEditor {
        #shadow: ShadowRoot;
        #leftPanel: HTMLDivElement;
        #nameInput: HTMLInputElement;
        #nameError: HTMLSpanElement;
        #colorInputs: Map<string, HTMLInputElement> = new Map();
        #swatches: Map<string, HTMLSpanElement> = new Map();
        #okBtn: HTMLButtonElement;
        #selectedName: string | null = null;
        #selectedCard: HTMLElement | null = null;
        #selectedPalette: Palette | null = null;
        #paletteCache: Map<string, Palette> = new Map();
        #createCard: HTMLDivElement;
        #createRadio: HTMLInputElement;
        #isLoading = false;
        #builtinThemeNames: Set<string> = new Set();

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
                    padding: 20px;
                    display: flex; flex-direction: row;
                    width: min(780px, 100%); height: min(560px, 90vh);
                    box-sizing: border-box;
                    box-shadow: 0 8px 40px rgba(0,0,0,0.55);
                    overflow: hidden;
                }
                .left-panel {
                    width: 240px; flex-shrink: 0;
                    display: flex; flex-direction: column; gap: 6px;
                    overflow-y: auto; min-height: 0;
                    padding-right: 10px;
                    border-right: 1px solid #c0c0c0;
                }
                .create-card {
                    display: flex; align-items: center; gap: 7px;
                    padding: 8px 10px;
                    background: #c8c8c8;
                    border: 2px solid transparent;
                    border-radius: 4px;
                    cursor: pointer;
                    font-family: sans-serif; font-size: 13px; font-weight: 600; color: #222;
                    user-select: none; flex-shrink: 0;
                }
                .create-card:hover { background: #bbb; }
                .create-card[selected] { border-color: #444; background: #b8b8b8; }
                .create-card input[type=radio] { cursor: pointer; accent-color: #444; }
                ${PALETTE_CARD_VARS}
                b3tty-palette-card { flex-shrink: 0; }
                .right-panel {
                    flex: 1; display: flex; flex-direction: column;
                    padding-left: 16px; min-width: 0;
                }
                .name-section {
                    display: flex; flex-direction: column; gap: 4px;
                    flex-shrink: 0; padding-bottom: 10px;
                    border-bottom: 1px solid #c8c8c8;
                    margin-bottom: 8px;
                }
                .name-row {
                    display: flex; align-items: center; gap: 8px;
                }
                .name-section label {
                    font-family: sans-serif; font-size: 12px; color: #444;
                    white-space: nowrap; min-width: 80px;
                }
                .name-input {
                    flex: 1; font-family: sans-serif; font-size: 13px;
                    padding: 4px 8px; border: 1px solid #aaa; border-radius: 3px;
                    background: #f5f5f5;
                }
                .name-input:read-only { background: #e8e8e8; color: #555; }
                .name-error {
                    font-family: sans-serif; font-size: 11px; color: #c00;
                    display: none;
                }
                .name-error.visible { display: block; }
                .color-form {
                    flex: 1; overflow-y: auto; min-height: 0;
                    display: flex; flex-direction: column; gap: 4px;
                }
                .section-title {
                    font-family: sans-serif; font-size: 11px; font-weight: 600;
                    text-transform: uppercase; letter-spacing: 0.05em;
                    color: #666; padding: 6px 0 2px;
                }
                .field-row {
                    display: grid;
                    grid-template-columns: 140px 1fr 22px;
                    gap: 4px; align-items: center;
                }
                .field-row label {
                    font-family: sans-serif; font-size: 12px; color: #444;
                }
                .ansi-header {
                    display: grid;
                    grid-template-columns: 68px 1fr 22px 1fr 22px;
                    gap: 4px;
                    font-family: sans-serif; font-size: 11px; font-weight: 600;
                    color: #666; padding-bottom: 2px;
                }
                .ansi-row {
                    display: grid;
                    grid-template-columns: 68px 1fr 22px 1fr 22px;
                    gap: 4px; align-items: center;
                }
                .ansi-row .color-label {
                    font-family: sans-serif; font-size: 12px; color: #444;
                }
                .color-input {
                    font-family: monospace; font-size: 12px;
                    padding: 3px 6px; border: 1px solid #aaa; border-radius: 3px;
                    background: #f5f5f5; width: 100%; box-sizing: border-box;
                    min-width: 0;
                }
                .swatch {
                    width: 18px; height: 18px;
                    border-radius: 3px; border: 1px solid rgba(0,0,0,0.2);
                    visibility: hidden;
                }
                .swatch.visible { visibility: visible; }
                .actions {
                    display: flex; justify-content: flex-end; gap: 10px;
                    padding-top: 10px; flex-shrink: 0;
                    border-top: 1px solid #c8c8c8; margin-top: 8px;
                }
                ${BUTTON_STYLES}
            `;

            const overlay = document.createElement("div");
            overlay.className = "overlay";
            const modal = document.createElement("div");
            modal.className = "modal";
            modal.setAttribute("role", "dialog");
            modal.setAttribute("aria-modal", "true");

            // --- Left panel ---
            this.#leftPanel = document.createElement("div");
            this.#leftPanel.className = "left-panel";

            this.#createCard = document.createElement("div");
            this.#createCard.className = "create-card";
            this.#createRadio = document.createElement("input");
            this.#createRadio.type = "radio";
            this.#createRadio.name = "theme";
            this.#createRadio.checked = true;
            const createLabel = document.createElement("span");
            createLabel.textContent = "Create new theme";
            this.#createCard.appendChild(this.#createRadio);
            this.#createCard.appendChild(createLabel);
            this.#leftPanel.appendChild(this.#createCard);

            // --- Right panel ---
            const rightPanel = document.createElement("div");
            rightPanel.className = "right-panel";

            const nameSection = document.createElement("div");
            nameSection.className = "name-section";
            const nameRow = document.createElement("div");
            nameRow.className = "name-row";
            const nameLabel = document.createElement("label");
            nameLabel.textContent = "Theme Name";
            this.#nameInput = document.createElement("input");
            this.#nameInput.type = "text";
            this.#nameInput.className = "name-input";
            this.#nameInput.placeholder = "Enter theme name";
            this.#nameError = document.createElement("span");
            this.#nameError.className = "name-error";
            this.#nameError.textContent = "Cannot use a built-in theme name";
            nameRow.appendChild(nameLabel);
            nameRow.appendChild(this.#nameInput);
            nameSection.appendChild(nameRow);
            nameSection.appendChild(this.#nameError);

            const colorForm = document.createElement("div");
            colorForm.className = "color-form";

            const coreTitle = document.createElement("div");
            coreTitle.className = "section-title";
            coreTitle.textContent = "Core Colors";
            colorForm.appendChild(coreTitle);

            for (const { key, label } of [
                { key: "background", label: "Background" },
                { key: "foreground", label: "Foreground" },
                { key: "cursor", label: "Cursor" },
                { key: "cursorAccent", label: "Cursor Accent" },
                { key: "selectionBackground", label: "Selection Background" },
                { key: "selectionForeground", label: "Selection Foreground" },
            ]) {
                const [row, input, swatch] = this.#makeColorRow(label);
                this.#colorInputs.set(key, input);
                this.#swatches.set(key, swatch);
                colorForm.appendChild(row);
            }

            const ansiTitle = document.createElement("div");
            ansiTitle.className = "section-title";
            ansiTitle.textContent = "ANSI Colors";
            colorForm.appendChild(ansiTitle);

            const ansiHeader = document.createElement("div");
            ansiHeader.className = "ansi-header";
            ansiHeader.appendChild(document.createElement("span")); // color name column
            const hNormal = document.createElement("span");
            hNormal.textContent = "Normal";
            ansiHeader.appendChild(hNormal);
            ansiHeader.appendChild(document.createElement("span")); // swatch placeholder
            const hBright = document.createElement("span");
            hBright.textContent = "Bright";
            ansiHeader.appendChild(hBright);
            ansiHeader.appendChild(document.createElement("span")); // swatch placeholder
            colorForm.appendChild(ansiHeader);

            for (const [colorName, normalKey, brightKey] of [
                ["Black", "black", "brightBlack"],
                ["Red", "red", "brightRed"],
                ["Yellow", "yellow", "brightYellow"],
                ["Green", "green", "brightGreen"],
                ["Cyan", "cyan", "brightCyan"],
                ["Blue", "blue", "brightBlue"],
                ["Magenta", "magenta", "brightMagenta"],
                ["White", "white", "brightWhite"],
            ] as [string, string, string][]) {
                const ansiRow = document.createElement("div");
                ansiRow.className = "ansi-row";
                const colorLabel = document.createElement("span");
                colorLabel.className = "color-label";
                colorLabel.textContent = colorName;
                ansiRow.appendChild(colorLabel);
                for (const key of [normalKey, brightKey]) {
                    const input = document.createElement("input");
                    input.type = "text";
                    input.className = "color-input";
                    input.placeholder = "#rrggbb";
                    const swatch = document.createElement("span");
                    swatch.className = "swatch";
                    input.addEventListener("input", () => {
                        this.#updateSwatch(swatch, input.value);
                        this.#validateForm();
                    });
                    this.#colorInputs.set(key, input);
                    this.#swatches.set(key, swatch);
                    ansiRow.appendChild(input);
                    ansiRow.appendChild(swatch);
                }
                colorForm.appendChild(ansiRow);
            }

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

            rightPanel.appendChild(nameSection);
            rightPanel.appendChild(colorForm);
            rightPanel.appendChild(actions);

            modal.appendChild(this.#leftPanel);
            modal.appendChild(rightPanel);
            overlay.appendChild(modal);
            this.#shadow.appendChild(style);
            this.#shadow.appendChild(overlay);

            // Listeners
            this.#nameInput.addEventListener("input", () => this.#validateForm());

            this.#createCard.addEventListener("click", () => {
                for (const card of Array.from(this.#leftPanel.querySelectorAll("b3tty-palette-card"))) {
                    card.removeAttribute("selected");
                }
                this.#createCard.setAttribute("selected", "");
                this.#createRadio.checked = true;
                this.#restoreSelectedCard();
                this.#selectedName = null;
                this.#nameInput.value = "";
                this.#clearInputs();
                this.#isLoading = false;
                this.#validateForm();
            });

            this.#leftPanel.addEventListener("b3tty-card-select", (e: Event) => {
                const target = e.target as HTMLElement;
                for (const card of Array.from(this.#leftPanel.querySelectorAll("b3tty-palette-card"))) {
                    if (card !== target) card.removeAttribute("selected");
                }
                this.#createCard.removeAttribute("selected");
                this.#createRadio.checked = false;
                const name = (target as unknown as B3ttyPaletteCard).value;
                this.#restoreSelectedCard();
                this.#selectedName = name;
                this.#selectedCard = target;
                this.#selectedPalette = this.#paletteCache.get(name) ?? null;
                this.#nameInput.value = name;
                this.#isLoading = true;
                this.#okBtn.disabled = true;
                this.#clearInputs();
                this.#loadThemeColors(name);
            });

            cancelBtn.addEventListener("click", () => this.close());
            this.#okBtn.addEventListener("click", () => void this.#handleOk());
        }

        #makeColorRow(label: string): [HTMLDivElement, HTMLInputElement, HTMLSpanElement] {
            const row = document.createElement("div");
            row.className = "field-row";
            const lbl = document.createElement("label");
            lbl.textContent = label;
            const input = document.createElement("input");
            input.type = "text";
            input.className = "color-input";
            input.placeholder = "#rrggbb";
            const swatch = document.createElement("span");
            swatch.className = "swatch";
            input.addEventListener("input", () => {
                this.#updateSwatch(swatch, input.value);
                this.#validateForm();
            });
            row.appendChild(lbl);
            row.appendChild(input);
            row.appendChild(swatch);
            return [row, input, swatch];
        }

        #updateSwatch(swatch: HTMLSpanElement, value: string): void {
            if (value && isValidThemeColor(value)) {
                swatch.style.backgroundColor = value;
                swatch.classList.add("visible");
            } else {
                swatch.classList.remove("visible");
            }
        }

        #validateForm(): void {
            if (this.#isLoading) {
                this.#okBtn.disabled = true;
                this.#nameError.classList.remove("visible");
                return;
            }
            this.#updatePreview();
            const name = this.#nameInput.value.trim();
            if (!name) {
                this.#okBtn.disabled = true;
                this.#nameError.classList.remove("visible");
                return;
            }
            if (this.#builtinThemeNames.has(name)) {
                this.#okBtn.disabled = true;
                this.#nameError.classList.add("visible");
                return;
            }
            this.#nameError.classList.remove("visible");
            for (const input of this.#colorInputs.values()) {
                if (input.value && !isValidThemeColor(input.value)) {
                    this.#okBtn.disabled = true;
                    return;
                }
            }
            this.#okBtn.disabled = false;
        }

        #restoreSelectedCard(): void {
            if (!this.#selectedCard || !this.#selectedName) return;
            const original = this.#paletteCache.get(this.#selectedName);
            if (original) {
                (this.#selectedCard as unknown as B3ttyPaletteCard).setup(
                    this.#selectedName,
                    formatThemeName(this.#selectedName),
                    original
                );
            }
            this.#selectedCard = null;
            this.#selectedPalette = null;
        }

        #buildPreviewPalette(): Palette | null {
            if (!this.#selectedPalette) return null;
            const base = this.#selectedPalette;
            const get = (key: string): string | null => {
                const val = this.#colorInputs.get(key)?.value;
                return val && isValidThemeColor(val) ? val : null;
            };
            const normalKeys = ["black", "red", "yellow", "green", "cyan", "blue", "magenta", "white"];
            const brightKeys = [
                "brightBlack",
                "brightRed",
                "brightYellow",
                "brightGreen",
                "brightCyan",
                "brightBlue",
                "brightMagenta",
                "brightWhite",
            ];
            return {
                bg: get("background") ?? base.bg,
                fg: get("foreground") ?? base.fg,
                selBg: get("selectionBackground") ?? base.selBg,
                cursor: get("cursor") ?? base.cursor,
                normal: normalKeys.map((k, i) => get(k) ?? base.normal[i]),
                bright: brightKeys.map((k, i) => get(k) ?? base.bright[i]),
            };
        }

        #updatePreview(): void {
            if (!this.#selectedCard || !this.#selectedName || !this.#selectedPalette) return;
            const palette = this.#buildPreviewPalette();
            if (!palette) return;
            (this.#selectedCard as unknown as B3ttyPaletteCard).setup(
                this.#selectedName,
                formatThemeName(this.#selectedName),
                palette
            );
        }

        #clearInputs(): void {
            for (const [key, input] of this.#colorInputs) {
                input.value = "";
                const swatch = this.#swatches.get(key);
                if (swatch) swatch.classList.remove("visible");
            }
        }

        async #loadThemeColors(name: string): Promise<void> {
            try {
                const config = await getThemeConfig(name);
                const data = config as unknown as Record<string, string>;
                for (const [key, input] of this.#colorInputs) {
                    const value = data[key] ?? "";
                    input.value = value;
                    const swatch = this.#swatches.get(key);
                    if (swatch) this.#updateSwatch(swatch, value);
                }
            } catch {
                // leave inputs as-is on failure
            } finally {
                this.#isLoading = false;
                this.#validateForm();
            }
        }

        async #handleOk(): Promise<void> {
            const name = this.#nameInput.value.trim();
            if (!name) return;
            const themeData: Record<string, string> = {};
            for (const [key, input] of this.#colorInputs) {
                if (input.value) themeData[key] = input.value;
            }
            try {
                const response = await postEditTheme(name, themeData);
                this.dispatchEvent(
                    new CustomEvent("b3tty-theme-edited", {
                        detail: { name, response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.close();
            } catch {
                // keep editor open so the user can retry
            }
        }

        open(themeNames: string[], builtinThemeNames: string[] = []): void {
            this.#builtinThemeNames = new Set(builtinThemeNames);
            this.#selectedName = null;
            this.#selectedCard = null;
            this.#selectedPalette = null;
            this.#paletteCache.clear();
            this.#isLoading = false;
            this.#nameInput.value = "";
            this.#nameError.classList.remove("visible");
            this.#clearInputs();
            this.#okBtn.disabled = true;

            // rebuild card list (keep create-card)
            const existingCards = Array.from(this.#leftPanel.querySelectorAll("b3tty-palette-card"));
            for (const card of existingCards) card.remove();
            this.#createCard.setAttribute("selected", "");
            this.#createRadio.checked = true;

            this.setAttribute("open", "");

            fetchPaletteCards(themeNames).then((entries) => {
                for (const c of Array.from(this.#leftPanel.querySelectorAll("b3tty-palette-card"))) c.remove();
                for (const { card, name, palette } of entries) {
                    this.#paletteCache.set(name, palette);
                    this.#leftPanel.appendChild(card);
                }
            });
        }

        close(): void {
            this.removeAttribute("open");
            this.#restoreSelectedCard();
            this.#selectedName = null;
            this.#nameError.classList.remove("visible");
        }
    }

    customElements.define("b3tty-theme-editor", B3ttyThemeEditorImpl);

    class B3ttyProfileEditorImpl extends HTMLElement implements B3ttyProfileEditor {
        #shadow: ShadowRoot;
        #leftPanel: HTMLDivElement;
        #createCard: HTMLDivElement;
        #createRadio: HTMLInputElement;
        #nameInput: HTMLInputElement;
        #nameError: HTMLSpanElement;
        #shellInput: HTMLInputElement;
        #titleInput: HTMLInputElement;
        #wdInput: HTMLInputElement;
        #rootInput: HTMLInputElement;
        #commandsArea: HTMLTextAreaElement;
        #lineNumbers: HTMLDivElement;
        #okBtn: HTMLButtonElement;
        #deleteBtn: HTMLButtonElement;
        #selectedName: string | null = null;
        #selectedCard: HTMLDivElement | null = null;
        #isLoading = false;

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
                    padding: 20px;
                    display: flex; flex-direction: row;
                    width: min(680px, 100%); height: min(500px, 90vh);
                    box-sizing: border-box;
                    box-shadow: 0 8px 40px rgba(0,0,0,0.55);
                    overflow: hidden;
                }
                .left-panel {
                    width: 200px; flex-shrink: 0;
                    display: flex; flex-direction: column; gap: 6px;
                    overflow-y: auto; min-height: 0;
                    padding-right: 10px;
                    border-right: 1px solid #c0c0c0;
                }
                .create-card {
                    display: flex; align-items: center; gap: 7px;
                    padding: 8px 10px;
                    background: #c8c8c8;
                    border: 2px solid transparent;
                    border-radius: 4px;
                    cursor: pointer;
                    font-family: sans-serif; font-size: 13px; font-weight: 600; color: #222;
                    user-select: none; flex-shrink: 0;
                }
                .create-card:hover { background: #bbb; }
                .create-card[selected] { border-color: #444; background: #b8b8b8; }
                .create-card input[type=radio] { cursor: pointer; accent-color: #444; }
                .profile-card {
                    display: flex; align-items: center; gap: 7px;
                    padding: 8px 10px;
                    background: #c8c8c8;
                    border: 2px solid transparent;
                    border-radius: 4px;
                    cursor: pointer;
                    font-family: sans-serif; font-size: 13px; color: #222;
                    user-select: none; flex-shrink: 0;
                }
                .profile-card:hover { background: #bbb; }
                .profile-card[selected] { border-color: #444; background: #b8b8b8; }
                .profile-card input[type=radio] { cursor: pointer; accent-color: #444; }
                .right-panel {
                    flex: 1; display: flex; flex-direction: column;
                    padding-left: 16px; min-width: 0;
                }
                .fields-section {
                    display: flex; flex-direction: column; gap: 6px;
                    flex-shrink: 0; padding-bottom: 10px;
                    border-bottom: 1px solid #c8c8c8;
                    margin-bottom: 8px;
                }
                .field-row {
                    display: grid;
                    grid-template-columns: 120px 1fr;
                    gap: 6px; align-items: center;
                }
                .field-row label {
                    font-family: sans-serif; font-size: 12px; color: #444;
                    white-space: nowrap;
                }
                .field-row label .required {
                    color: #c00;
                }
                .field-input {
                    font-family: sans-serif; font-size: 13px;
                    padding: 4px 8px; border: 1px solid #aaa; border-radius: 3px;
                    background: #f5f5f5; box-sizing: border-box;
                }
                .field-input:read-only { background: #e8e8e8; color: #555; }
                .name-error {
                    grid-column: 2;
                    font-family: sans-serif; font-size: 11px; color: #c00;
                    display: none;
                }
                .name-error.visible { display: block; }
                .commands-section {
                    flex: 1; display: flex; flex-direction: column; gap: 4px;
                    min-height: 0;
                }
                .commands-label {
                    font-family: sans-serif; font-size: 12px; color: #444;
                    flex-shrink: 0;
                }
                .commands-wrapper {
                    display: flex;
                    border: 1px solid #aaa;
                    border-radius: 3px;
                    overflow: hidden;
                    flex: 1; min-height: 0;
                    background: #f5f5f5;
                    font-family: monospace;
                    font-size: 12px;
                    line-height: 1.5em;
                }
                .line-numbers {
                    width: 32px;
                    padding: 4px 4px;
                    background: #ddd;
                    color: #888;
                    text-align: right;
                    user-select: none;
                    overflow: hidden;
                    white-space: pre;
                    line-height: inherit;
                    flex-shrink: 0;
                    box-sizing: border-box;
                }
                .commands-area {
                    flex: 1;
                    padding: 4px 6px;
                    border: none;
                    outline: none;
                    resize: none;
                    background: transparent;
                    font-family: inherit;
                    font-size: inherit;
                    line-height: inherit;
                    overflow: auto;
                    white-space: pre;
                    box-sizing: border-box;
                }
                .actions {
                    display: flex; justify-content: flex-end; gap: 10px;
                    align-items: center;
                    padding-top: 10px; flex-shrink: 0;
                    border-top: 1px solid #c8c8c8; margin-top: 8px;
                }
                .delete-btn {
                    margin-right: auto;
                    padding: 8px 16px; border-radius: 5px;
                    border: 1px solid #c44; background: #f5d5d5;
                    font-size: 14px; font-family: sans-serif; cursor: pointer; color: #c00;
                }
                .delete-btn:hover { background: #f0b8b8; }
                ${BUTTON_STYLES}
            `;

            const overlay = document.createElement("div");
            overlay.className = "overlay";
            const modal = document.createElement("div");
            modal.className = "modal";
            modal.setAttribute("role", "dialog");
            modal.setAttribute("aria-modal", "true");

            // --- Left panel ---
            this.#leftPanel = document.createElement("div");
            this.#leftPanel.className = "left-panel";

            this.#createCard = document.createElement("div");
            this.#createCard.className = "create-card";
            this.#createCard.setAttribute("selected", "");
            this.#createRadio = document.createElement("input");
            this.#createRadio.type = "radio";
            this.#createRadio.name = "profile";
            this.#createRadio.checked = true;
            const createLabel = document.createElement("span");
            createLabel.textContent = "Create new profile";
            this.#createCard.appendChild(this.#createRadio);
            this.#createCard.appendChild(createLabel);
            this.#leftPanel.appendChild(this.#createCard);

            // --- Right panel ---
            const rightPanel = document.createElement("div");
            rightPanel.className = "right-panel";

            const fieldsSection = document.createElement("div");
            fieldsSection.className = "fields-section";

            const makeFieldRow = (
                labelText: string,
                required: boolean,
                placeholder: string
            ): [HTMLDivElement, HTMLInputElement] => {
                const row = document.createElement("div");
                row.className = "field-row";
                const lbl = document.createElement("label");
                if (required) {
                    lbl.innerHTML = `${labelText} <span class="required">*</span>`;
                } else {
                    lbl.textContent = labelText;
                }
                const input = document.createElement("input");
                input.type = "text";
                input.className = "field-input";
                input.placeholder = placeholder;
                row.appendChild(lbl);
                row.appendChild(input);
                return [row, input];
            };

            const [nameRow, nameInput] = makeFieldRow("Profile Name", true, "Enter profile name");
            this.#nameInput = nameInput;
            this.#nameError = document.createElement("span");
            this.#nameError.className = "name-error";
            this.#nameError.textContent = "Cannot use 'default' as a profile name";
            nameRow.appendChild(this.#nameError);
            fieldsSection.appendChild(nameRow);

            const [shellRow, shellInput] = makeFieldRow("Shell", false, "e.g. /bin/zsh");
            this.#shellInput = shellInput;
            fieldsSection.appendChild(shellRow);

            const [titleRow, titleInput] = makeFieldRow("Title", false, "Terminal window title");
            this.#titleInput = titleInput;
            fieldsSection.appendChild(titleRow);

            const [wdRow, wdInput] = makeFieldRow("Working Directory", false, "e.g. ~/projects");
            this.#wdInput = wdInput;
            fieldsSection.appendChild(wdRow);

            const [rootRow, rootInput] = makeFieldRow("Root", false, "e.g. /");
            this.#rootInput = rootInput;
            fieldsSection.appendChild(rootRow);

            // --- Commands editor ---
            const commandsSection = document.createElement("div");
            commandsSection.className = "commands-section";

            const commandsLabel = document.createElement("div");
            commandsLabel.className = "commands-label";
            commandsLabel.textContent = "Commands (one per line, run on startup):";
            commandsSection.appendChild(commandsLabel);

            const commandsWrapper = document.createElement("div");
            commandsWrapper.className = "commands-wrapper";

            this.#lineNumbers = document.createElement("div");
            this.#lineNumbers.className = "line-numbers";
            this.#lineNumbers.textContent = "1";

            this.#commandsArea = document.createElement("textarea");
            this.#commandsArea.className = "commands-area";
            this.#commandsArea.placeholder = "npm start";
            this.#commandsArea.spellcheck = false;
            this.#commandsArea.setAttribute("wrap", "off");

            commandsWrapper.appendChild(this.#lineNumbers);
            commandsWrapper.appendChild(this.#commandsArea);
            commandsSection.appendChild(commandsWrapper);

            // --- Actions ---
            const actions = document.createElement("div");
            actions.className = "actions";

            this.#deleteBtn = document.createElement("button");
            this.#deleteBtn.className = "delete-btn";
            this.#deleteBtn.textContent = "Delete Profile";
            this.#deleteBtn.style.display = "none";

            const cancelBtn = document.createElement("button");
            cancelBtn.className = "cancel-btn";
            cancelBtn.textContent = "Cancel";

            this.#okBtn = document.createElement("button");
            this.#okBtn.className = "ok-btn";
            this.#okBtn.textContent = "OK";
            this.#okBtn.disabled = true;

            actions.appendChild(this.#deleteBtn);
            actions.appendChild(cancelBtn);
            actions.appendChild(this.#okBtn);

            // --- Assemble ---
            rightPanel.appendChild(fieldsSection);
            rightPanel.appendChild(commandsSection);
            rightPanel.appendChild(actions);
            modal.appendChild(this.#leftPanel);
            modal.appendChild(rightPanel);
            overlay.appendChild(modal);
            this.#shadow.appendChild(style);
            this.#shadow.appendChild(overlay);

            // --- Event listeners ---
            this.#nameInput.addEventListener("input", () => this.#validateForm());

            this.#commandsArea.addEventListener("input", () => this.#syncLineNumbers());
            this.#commandsArea.addEventListener("scroll", () => {
                this.#lineNumbers.scrollTop = this.#commandsArea.scrollTop;
            });

            this.#createCard.addEventListener("click", () => {
                this.#selectCreateCard();
            });

            this.#leftPanel.addEventListener("click", (e) => {
                const card = (e.target as HTMLElement).closest(".profile-card") as HTMLDivElement | null;
                if (!card) return;
                const name = (card as HTMLDivElement).dataset["profileName"];
                if (!name) return;
                this.#selectProfileCard(card, name);
            });

            cancelBtn.addEventListener("click", () => this.close());
            this.#deleteBtn.addEventListener("click", () => void this.#handleDelete());
            this.#okBtn.addEventListener("click", () => void this.#handleOk());
        }

        open(profileNames: string[]): void {
            this.#selectedName = null;
            this.#selectedCard = null;
            this.#isLoading = false;
            this.#clearForm();
            this.#okBtn.disabled = true;
            this.#deleteBtn.style.display = "none";
            this.#nameInput.readOnly = false;

            for (const card of Array.from(this.#leftPanel.querySelectorAll(".profile-card"))) {
                card.remove();
            }
            this.#selectCreateCard();

            const filtered = profileNames.filter((n) => n !== "default");
            for (const name of filtered) {
                this.#leftPanel.appendChild(this.#makeProfileCard(name));
            }
            this.setAttribute("open", "");
        }

        close(): void {
            this.removeAttribute("open");
            this.#selectedName = null;
            this.#nameError.classList.remove("visible");
        }

        #makeProfileCard(name: string): HTMLDivElement {
            const card = document.createElement("div");
            card.className = "profile-card";
            card.dataset["profileName"] = name;
            const radio = document.createElement("input");
            radio.type = "radio";
            radio.name = "profile";
            const lbl = document.createElement("span");
            lbl.textContent = name;
            card.appendChild(radio);
            card.appendChild(lbl);
            return card;
        }

        #selectCreateCard(): void {
            for (const c of Array.from(this.#leftPanel.querySelectorAll(".profile-card"))) {
                c.removeAttribute("selected");
                (c.querySelector("input[type=radio]") as HTMLInputElement | null)!.checked = false;
            }
            this.#createCard.setAttribute("selected", "");
            this.#createRadio.checked = true;
            this.#selectedName = null;
            this.#selectedCard = null;
            this.#nameInput.readOnly = false;
            this.#deleteBtn.style.display = "none";
            this.#clearForm();
        }

        #selectProfileCard(card: HTMLDivElement, name: string): void {
            this.#createCard.removeAttribute("selected");
            this.#createRadio.checked = false;
            for (const c of Array.from(this.#leftPanel.querySelectorAll(".profile-card"))) {
                c.removeAttribute("selected");
                (c.querySelector("input[type=radio]") as HTMLInputElement | null)!.checked = false;
            }
            card.setAttribute("selected", "");
            (card.querySelector("input[type=radio]") as HTMLInputElement | null)!.checked = true;
            this.#selectedName = name;
            this.#selectedCard = card;
            this.#nameInput.value = name;
            this.#nameInput.readOnly = true;
            this.#deleteBtn.style.display = "";
            this.#isLoading = true;
            this.#okBtn.disabled = true;
            void this.#loadProfileConfig(name);
        }

        async #loadProfileConfig(name: string): Promise<void> {
            try {
                const cfg = await getProfileConfig(name);
                this.#shellInput.value = cfg.shell ?? "";
                this.#titleInput.value = cfg.title ?? "";
                this.#wdInput.value = cfg.workingDirectory ?? "";
                this.#rootInput.value = cfg.root ?? "";
                this.#commandsArea.value = (cfg.commands ?? []).join("\n");
                this.#syncLineNumbers();
            } catch {
                // Silently keep existing form state on load failure.
            } finally {
                this.#isLoading = false;
                this.#validateForm();
            }
        }

        #clearForm(): void {
            this.#nameInput.value = "";
            this.#shellInput.value = "";
            this.#titleInput.value = "";
            this.#wdInput.value = "";
            this.#rootInput.value = "";
            this.#commandsArea.value = "";
            this.#syncLineNumbers();
            this.#nameError.classList.remove("visible");
        }

        #syncLineNumbers(): void {
            const lineCount = this.#commandsArea.value === "" ? 1 : this.#commandsArea.value.split("\n").length;
            this.#lineNumbers.textContent = Array.from({ length: lineCount }, (_, i) => i + 1).join("\n");
            this.#lineNumbers.scrollTop = this.#commandsArea.scrollTop;
        }

        #validateForm(): void {
            if (this.#isLoading) {
                this.#okBtn.disabled = true;
                return;
            }
            const name = this.#nameInput.value.trim();
            if (name === "default") {
                this.#nameError.classList.add("visible");
                this.#okBtn.disabled = true;
                return;
            }
            this.#nameError.classList.remove("visible");
            this.#okBtn.disabled = name === "";
        }

        async #handleOk(): Promise<void> {
            const name = this.#nameInput.value.trim();
            if (!name) return;
            const rawCommands = this.#commandsArea.value.split("\n");
            const commands = rawCommands.filter((line) => line !== "");
            const profile: ProfileConfig = {
                shell: this.#shellInput.value.trim(),
                title: this.#titleInput.value.trim(),
                workingDirectory: this.#wdInput.value.trim(),
                root: this.#rootInput.value.trim(),
                commands,
            };
            try {
                const response = await postEditProfile(name, profile);
                this.dispatchEvent(
                    new CustomEvent("b3tty-profile-edited", {
                        detail: { name, response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.close();
            } catch {
                // Keep editor open so user can retry.
            }
        }

        async #handleDelete(): Promise<void> {
            const name = this.#selectedName;
            if (!name) return;
            try {
                const response = await postDeleteProfile(name);
                this.dispatchEvent(
                    new CustomEvent("b3tty-profile-edited", {
                        detail: { name: null, response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.close();
            } catch {
                // Keep editor open so user can retry.
            }
        }
    }

    customElements.define("b3tty-profile-editor", B3ttyProfileEditorImpl);
}
