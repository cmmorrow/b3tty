import type { ThemeEntry } from "../types.ts";
import {
    BASE_STYLES,
    BUTTON_STYLES,
    DESIGN_TOKENS,
    OVERLAY_STYLES,
    PALETTE_CARD_VARS,
    TEXT_STYLES,
} from "../design.ts";
import { closeOverlay } from "./overlay.ts";
import { type B3ttyPaletteCard, fetchPaletteCards, THEME_SECTIONS } from "./palette-card.ts";

/**
 * Interface for the b3tty-theme-picker web component.
 */
export interface B3ttyThemePicker {
    open(themes: ThemeEntry[]): void;
    close(): void;
}

/**
 * Returns true when el exposes the B3ttyThemePicker contract (open/close methods).
 */
export function isB3ttyThemePicker(el: Element): el is HTMLElement & B3ttyThemePicker {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["open"] === "function" && typeof r["close"] === "function";
}

if (typeof HTMLElement !== "undefined") {
    class B3ttyThemePickerImpl extends HTMLElement implements B3ttyThemePicker {
        #shadow: ShadowRoot;
        #sections: HTMLDivElement;
        #okBtn: HTMLButtonElement;
        /** Bumped on every open, so palettes fetched for an earlier open are ignored. */
        #openId = 0;

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            this.#shadow.innerHTML = `
                <style>
                    ${DESIGN_TOKENS}
                    ${BASE_STYLES}
                    ${OVERLAY_STYLES}
                    ${TEXT_STYLES}
                    .modal {
                        padding: var(--space-3xl) var(--space-3xl) var(--space-2xl);
                        display: flex; flex-direction: column; gap: var(--space-xl);
                        max-height: 85vh; max-width: 1000px; width: 100%;
                        overflow: hidden;
                    }
                    .sections {
                        display: flex; flex-direction: column; gap: var(--space-xl);
                        overflow-y: auto; flex: 1; min-height: 0;
                        padding: var(--space-2xs) var(--space-3xs);
                    }
                    .section { display: flex; flex-direction: column; gap: var(--space-sm); }
                    .section > .section-title { margin: 0; }
                    .cards {
                        display: grid;
                        grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
                        gap: var(--space-lg);
                    }
                    ${PALETTE_CARD_VARS}
                    .loading {
                        font-size: var(--font-size-md); color: var(--color-text-subtle);
                        text-align: center; padding: var(--space-2xl);
                    }
                    ${BUTTON_STYLES}
                </style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true">
                        <h2 class="modal-title">Select a Theme</h2>
                        <div class="sections"></div>
                        <div class="actions">
                            <button class="cancel-btn">Cancel</button>
                            <button class="ok-btn" disabled>OK</button>
                        </div>
                    </div>
                </div>
            `;
            this.#sections = this.#shadow.querySelector<HTMLDivElement>(".sections")!;
            this.#okBtn = this.#shadow.querySelector<HTMLButtonElement>(".ok-btn")!;
            const cancelBtn = this.#shadow.querySelector<HTMLButtonElement>(".cancel-btn")!;

            // b3tty-card-select is composed, so it crosses the palette card's
            // shadow boundary and bubbles up to this listener on #sections.
            this.#sections.addEventListener("b3tty-card-select", (e: Event) => {
                for (const card of Array.from(this.#sections.querySelectorAll("b3tty-palette-card"))) {
                    if (card !== e.target) card.removeAttribute("selected");
                }
                this.#okBtn.disabled = false;
            });

            cancelBtn.addEventListener("click", () => this.close());
            this.#okBtn.addEventListener("click", () => {
                const selected = this.#sections.querySelector<HTMLElement>("b3tty-palette-card[selected]");
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

        /** Shows every theme, grouped into Your themes, Installed, and Built-in. */
        open(themes: ThemeEntry[]): void {
            const openId = ++this.#openId;
            this.#okBtn.disabled = true;
            this.#sections.innerHTML = `<div class="loading">Loading themes\u2026</div>`;
            this.setAttribute("open", "");

            void fetchPaletteCards(themes).then((cards) => {
                if (openId !== this.#openId) return;
                this.#sections.replaceChildren();
                for (const { source, title } of THEME_SECTIONS) {
                    const inSection = cards.filter((c) => c.entry.source === source);
                    if (inSection.length === 0) continue;
                    const section = document.createElement("section");
                    section.className = "section";
                    const heading = document.createElement("h3");
                    heading.className = "section-title";
                    heading.textContent = title;
                    const grid = document.createElement("div");
                    grid.className = "cards";
                    for (const { card } of inSection) grid.appendChild(card);
                    section.append(heading, grid);
                    this.#sections.appendChild(section);
                }
            });
        }

        close(): void {
            this.#okBtn.disabled = true;
            closeOverlay(this);
        }
    }

    customElements.define("b3tty-theme-picker", B3ttyThemePickerImpl);
}
