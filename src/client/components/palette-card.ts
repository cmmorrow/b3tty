import { getThemePalette } from "../api.ts";
import type { Palette, ThemeEntry, ThemeSource } from "../types.ts";
import { isValidRepoUrl } from "../validators.ts";
import { BASE_STYLES, DESIGN_TOKENS, TEXT_STYLES } from "../design.ts";

/**
 * Optional extras on a palette card, used for installed themes: `subtitle` is a muted
 * line under the name, `href` makes the name a link (only an https URL, opened in a new
 * tab, and clicking it doesn't select the card), and `badge` is a small pill after it.
 */
export interface PaletteCardDetails {
    subtitle?: string;
    href?: string;
    badge?: string;
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
 *   --palette-card-padding         (default: var(--space-lg))
 *   --palette-card-gap             (default: var(--space-md))
 *   --palette-card-overflow        (default: visible)
 *   --palette-card-header-bg       (default: transparent)
 *   --palette-card-header-padding  (default: 0)
 *   --palette-card-header-font-size (default: var(--font-size-md))
 *   --palette-card-terminal-gap    (default: var(--space-xs))
 *   --palette-card-terminal-shadow (default: var(--shadow-card))
 *   --palette-card-terminal-min-width (default: 196px)
 */
export interface B3ttyPaletteCard {
    setup(value: string, label: string, palette: Palette, details?: PaletteCardDetails): void;
    readonly selected: boolean;
    readonly value: string;
}

/**
 * Returns true when el exposes the B3ttyPaletteCard contract (setup/value/selected).
 */
export function isB3ttyPaletteCard(el: Element): el is HTMLElement & B3ttyPaletteCard {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["setup"] === "function" && "value" in r && "selected" in r;
}

export function formatThemeName(name: string): string {
    return name
        .split("-")
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(" ");
}

/**
 * The palette card extras for a theme: an installed theme shows its author and
 * version, links its name to its repository, and is badged when its package was
 * modified. Other themes have none.
 */
export function cardDetails(entry: ThemeEntry): PaletteCardDetails | undefined {
    if (entry.source !== "installed") return undefined;
    const subtitle = [entry.author ? `by ${entry.author}` : "", entry.themeVersion ? `v${entry.themeVersion}` : ""]
        .filter(Boolean)
        .join(" · ");
    return {
        subtitle: subtitle || undefined,
        href: entry.repo && isValidRepoUrl(entry.repo) ? entry.repo : undefined,
        badge: entry.modified ? "Modified" : undefined,
    };
}

/** The theme list sections, in display order. */
export const THEME_SECTIONS: Array<{ source: ThemeSource; title: string }> = [
    { source: "user", title: "Your themes" },
    { source: "installed", title: "Installed" },
    { source: "builtin", title: "Built-in" },
];

/** A loaded palette card for one theme. */
export interface PaletteCardEntry {
    card: HTMLElement;
    entry: ThemeEntry;
    palette: Palette;
}

/**
 * Fetches every theme's palette in parallel, keeping the order of `themes`. A theme
 * whose palette can't be fetched is left out.
 */
export function fetchThemePalettes(themes: ThemeEntry[]): Promise<Array<{ entry: ThemeEntry; palette: Palette }>> {
    return Promise.all(
        themes.map((entry) =>
            getThemePalette(entry.name)
                .then((p) => ({ entry, palette: p as Palette | null }))
                .catch(() => ({ entry, palette: null as Palette | null }))
        )
    ).then((results) => results.flatMap(({ entry, palette }) => (palette ? [{ entry, palette }] : [])));
}

/** Builds a theme's palette card, with the installed-theme extras from cardDetails. */
export function createPaletteCard(entry: ThemeEntry, palette: Palette): HTMLElement {
    const card = document.createElement("b3tty-palette-card");
    (card as unknown as B3ttyPaletteCard).setup(entry.name, formatThemeName(entry.name), palette, cardDetails(entry));
    return card;
}

/**
 * Fetches every theme's palette in parallel and builds its card, keeping the order of
 * `themes`. A theme whose palette can't be fetched is left out.
 */
export function fetchPaletteCards(themes: ThemeEntry[]): Promise<PaletteCardEntry[]> {
    return fetchThemePalettes(themes).then((loaded) =>
        loaded.map(({ entry, palette }) => ({ card: createPaletteCard(entry, palette), entry, palette }))
    );
}

/** The palette card's styles; theme colors are never part of them (see #render). */
const PALETTE_CARD_STYLES = `
    ${DESIGN_TOKENS}
    ${BASE_STYLES}
    ${TEXT_STYLES}
    :host {
        display: flex;
        flex-direction: column;
        gap: var(--palette-card-gap, var(--space-md));
        padding: var(--palette-card-padding, var(--space-lg));
        border-radius: var(--radius-lg);
        border: 2px solid transparent;
        background: var(--color-surface-card);
        cursor: pointer;
        transition: border-color var(--transition);
        user-select: none;
        overflow: var(--palette-card-overflow, visible);
        box-sizing: border-box;
    }
    :host([selected]) { border-color: var(--color-accent); }
    .card-header [hidden] { display: none; }
    .card-name { display: flex; flex-direction: column; flex: 1; min-width: 0; }
    .name-link, .name-text, .card-subtitle {
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    /* Size the link to its text so the rest of the header still selects the card. */
    .card-name a { color: inherit; text-decoration: none; align-self: flex-start; max-width: 100%; }
    .card-name a:hover, .card-name a:focus-visible { text-decoration: underline; }
    .card-subtitle {
        font-size: var(--font-size-xs); font-weight: var(--font-weight-regular);
        color: var(--color-text-subtle);
    }
    .card-header {
        display: flex; align-items: center; gap: var(--space-xs);
        padding: var(--palette-card-header-padding, 0);
        font-size: var(--palette-card-header-font-size, var(--font-size-md));
        font-weight: var(--font-weight-semibold);
        background: var(--palette-card-header-bg, transparent);
    }
    .terminal {
        border-radius: var(--radius-md);
        padding: var(--space-md) var(--space-md) var(--space-sm);
        display: flex; flex-direction: column;
        gap: var(--palette-card-terminal-gap, var(--space-xs));
        font-family: var(--font-family-mono); font-size: var(--font-size-xs);
        box-shadow: var(--palette-card-terminal-shadow, var(--shadow-card));
        min-width: var(--palette-card-terminal-min-width, 196px);
    }
    .titlebar { display: flex; gap: 5px; margin-bottom: 1px; }
    .dot { width: 9px; height: 9px; border-radius: var(--radius-full); }
    .preview-text { padding: 1px 2px; line-height: var(--line-height-body); letter-spacing: 0.01em; }
    .sel { padding: 0 2px; border-radius: 2px; }
    .swatch-row { display: flex; gap: 3px; }
    .swatch {
        width: 20px; height: 20px; border-radius: var(--radius-sm);
        box-shadow: inset 0 0 0 1px rgba(128,128,128,0.25);
    }
`;

/** The traffic-light dots in the preview's title bar: window chrome, not theme data. */
const TITLEBAR_DOTS = ["#ff5f57", "#ffbd2e", "#28c841"];

// Built only from hard-coded values. Everything that comes from a theme — its name,
// details, link, and colors — is set by setup() through text, attributes, and styles.
const PALETTE_CARD_TEMPLATE = `
    <style>${PALETTE_CARD_STYLES}</style>
    <div class="card-header">
        <input type="radio" name="theme">
        <div class="card-name">
            <a class="name-link" target="_blank" rel="noopener noreferrer" hidden></a>
            <span class="name-text"></span>
            <span class="card-subtitle" hidden></span>
        </div>
        <span class="badge" hidden></span>
    </div>
    <div class="terminal">
        <div class="titlebar">
            ${TITLEBAR_DOTS.map((color) => `<div class="dot" style="background: ${color}"></div>`).join("")}
        </div>
        <div class="preview-text">lorem <span class="sel">ipsum</span><span class="cursor">\u00a0</span></div>
        <div class="swatch-row normal">${'<div class="swatch"></div>'.repeat(8)}</div>
        <div class="swatch-row bright">${'<div class="swatch"></div>'.repeat(8)}</div>
    </div>
`;

if (typeof HTMLElement !== "undefined") {
    class B3ttyPaletteCardImpl extends HTMLElement implements B3ttyPaletteCard {
        #value: string = "";
        #radio: HTMLInputElement;
        #link: HTMLAnchorElement;
        #nameText: HTMLSpanElement;
        #subtitle: HTMLSpanElement;
        #badge: HTMLSpanElement;
        #terminal: HTMLDivElement;
        #preview: HTMLDivElement;
        #sel: HTMLSpanElement;
        #cursor: HTMLSpanElement;
        #normalSwatches: HTMLDivElement[];
        #brightSwatches: HTMLDivElement[];

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
            if (name === "selected") this.#radio.checked = newValue !== null;
        }

        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });
            shadow.innerHTML = PALETTE_CARD_TEMPLATE;
            this.#radio = shadow.querySelector<HTMLInputElement>("input[type=radio]")!;
            this.#link = shadow.querySelector<HTMLAnchorElement>(".name-link")!;
            this.#nameText = shadow.querySelector<HTMLSpanElement>(".name-text")!;
            this.#subtitle = shadow.querySelector<HTMLSpanElement>(".card-subtitle")!;
            this.#badge = shadow.querySelector<HTMLSpanElement>(".badge")!;
            this.#terminal = shadow.querySelector<HTMLDivElement>(".terminal")!;
            this.#preview = shadow.querySelector<HTMLDivElement>(".preview-text")!;
            this.#sel = shadow.querySelector<HTMLSpanElement>(".sel")!;
            this.#cursor = shadow.querySelector<HTMLSpanElement>(".cursor")!;
            this.#normalSwatches = Array.from(shadow.querySelectorAll<HTMLDivElement>(".normal .swatch"));
            this.#brightSwatches = Array.from(shadow.querySelectorAll<HTMLDivElement>(".bright .swatch"));

            // Following the link shouldn't also select the card.
            this.#link.addEventListener("click", (e) => e.stopPropagation());
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

        /**
         * Fills the card in. Can be called again at any time, e.g. by the theme editor's
         * live preview; it only updates text, attributes, and styles, never the structure.
         */
        setup(value: string, label: string, palette: Palette, details?: PaletteCardDetails): void {
            this.#value = value;
            this.#radio.id = value;
            this.#radio.value = value;
            this.#radio.checked = this.selected;

            // An https repository makes the name a link; anything else shows it as text.
            const href = details?.href && isValidRepoUrl(details.href) ? details.href : null;
            this.#link.hidden = href === null;
            this.#nameText.hidden = href !== null;
            if (href === null) {
                this.#link.removeAttribute("href");
                this.#link.removeAttribute("title");
            } else {
                this.#link.href = href;
                this.#link.title = href;
            }
            this.#link.textContent = label;
            this.#nameText.textContent = label;
            this.#subtitle.textContent = details?.subtitle ?? "";
            this.#subtitle.hidden = !details?.subtitle;
            this.#badge.textContent = details?.badge ?? "";
            this.#badge.hidden = !details?.badge;

            this.#terminal.style.background = palette.bg;
            this.#preview.style.color = palette.fg;
            this.#sel.style.background = palette.selBg;
            this.#sel.style.color = palette.fg;
            this.#cursor.style.background = palette.cursor;
            this.#normalSwatches.forEach((swatch, i) => (swatch.style.background = palette.normal[i] ?? ""));
            this.#brightSwatches.forEach((swatch, i) => (swatch.style.background = palette.bright[i] ?? ""));
        }
    }

    customElements.define("b3tty-palette-card", B3ttyPaletteCardImpl);
}
