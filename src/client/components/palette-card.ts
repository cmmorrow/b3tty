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
 * Fetches every theme's palette in parallel and builds its card, keeping the order of
 * `themes`. A theme whose palette can't be fetched is left out.
 */
export function fetchPaletteCards(themes: ThemeEntry[]): Promise<PaletteCardEntry[]> {
    return Promise.all(
        themes.map((entry) =>
            getThemePalette(entry.name)
                .then((p) => ({ entry, palette: p as Palette | null }))
                .catch(() => ({ entry, palette: null as Palette | null }))
        )
    ).then((results) => {
        const cards: PaletteCardEntry[] = [];
        for (const { entry, palette } of results) {
            if (palette) {
                const card = document.createElement("b3tty-palette-card");
                (card as unknown as B3ttyPaletteCard).setup(
                    entry.name,
                    formatThemeName(entry.name),
                    palette,
                    cardDetails(entry)
                );
                cards.push({ card, entry, palette });
            }
        }
        return cards;
    });
}

if (typeof HTMLElement !== "undefined") {
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
                .card-name { display: flex; flex-direction: column; flex: 1; min-width: 0; }
                .card-name > :first-child, .card-subtitle {
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

        setup(value: string, label: string, palette: Palette, details?: PaletteCardDetails): void {
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
            const nameBlock = document.createElement("div");
            nameBlock.className = "card-name";
            let nameEl: HTMLElement;
            if (details?.href && isValidRepoUrl(details.href)) {
                const link = document.createElement("a");
                link.href = details.href;
                link.target = "_blank";
                link.rel = "noopener noreferrer";
                link.title = details.href;
                // Following the link shouldn't also select the card.
                link.addEventListener("click", (e) => e.stopPropagation());
                nameEl = link;
            } else {
                nameEl = document.createElement("span");
            }
            nameEl.textContent = label;
            nameBlock.appendChild(nameEl);
            if (details?.subtitle) {
                const subtitle = document.createElement("span");
                subtitle.className = "card-subtitle";
                subtitle.textContent = details.subtitle;
                nameBlock.appendChild(subtitle);
            }
            header.appendChild(radio);
            header.appendChild(nameBlock);
            if (details?.badge) {
                const badge = document.createElement("span");
                badge.className = "badge";
                badge.textContent = details.badge;
                header.appendChild(badge);
            }

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
}
