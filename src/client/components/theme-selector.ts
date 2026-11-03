import { getThemePalette, postSaveConfig } from "../api.ts";
import { BASE_STYLES, BUTTON_STYLES, DESIGN_TOKENS, OVERLAY_STYLES } from "../design.ts";
import type { B3ttyPaletteCard } from "./palette-card.ts";
import "./palette-card.ts";

if (typeof HTMLElement !== "undefined") {
    class B3ttyThemeSelectorImpl extends HTMLElement {
        #palettesRequested = false;
        #options: HTMLDivElement;

        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });

            shadow.innerHTML = `
                <style>
                    ${DESIGN_TOKENS}
                    ${BASE_STYLES}
                    ${OVERLAY_STYLES}
                    ${BUTTON_STYLES}
                    /* Always shown on the first-run setup page, never toggled via [open]. */
                    :host { display: block; }
                    .modal {
                        padding: var(--space-3xl);
                        display: flex; flex-direction: column; align-items: center;
                        gap: var(--space-2xl);
                    }
                    .subtitle { margin: 0; font-size: var(--font-size-md); color: var(--color-text-subtle); text-align: center; }
                    .options { display: flex; gap: var(--space-lg); flex-wrap: wrap; justify-content: center; }
                    .card {
                        display: flex; flex-direction: column; gap: var(--space-md);
                        padding: var(--space-lg); border-radius: var(--radius-lg);
                        border: 2px solid transparent;
                        background: var(--color-surface-card);
                        cursor: pointer;
                        transition: border-color var(--transition);
                        user-select: none;
                    }
                    .card:has(input:checked) { border-color: var(--color-accent); }
                    .card-header {
                        display: flex; align-items: center; gap: var(--space-xs);
                        font-size: var(--font-size-md); font-weight: var(--font-weight-semibold);
                    }
                    .skip-card { justify-content: center; min-width: 196px; }
                    .skip-note {
                        margin: 0; font-size: var(--font-size-sm); color: var(--color-muted);
                        max-width: 180px; line-height: var(--line-height-body);
                    }
                </style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true">
                        <p class="subtitle">Choose a default theme to get started, or skip to configure one later.</p>
                        <div class="options">
                            <label class="card skip-card">
                                <div class="card-header">
                                    <input type="radio" name="theme" id="skip" value="skip">
                                    <span>No theme</span>
                                </div>
                                <p class="skip-note">Configure a theme later in conf.yaml.</p>
                            </label>
                        </div>
                        <button class="ok-btn" id="ok-btn" disabled>OK</button>
                    </div>
                </div>
            `;
            this.#options = shadow.querySelector<HTMLDivElement>(".options")!;
            const okBtn = shadow.querySelector<HTMLButtonElement>("#ok-btn")!;

            let selectedValue: string | null = null;

            // Palette card selection — b3tty-card-select is composed so it crosses
            // the palette card's shadow boundary and reaches this listener.
            this.#options.addEventListener("b3tty-card-select", (e: Event) => {
                for (const card of Array.from(this.#options.querySelectorAll("b3tty-palette-card"))) {
                    if (card !== e.target) card.removeAttribute("selected");
                }
                const skipRadio = shadow.querySelector<HTMLInputElement>("#skip");
                if (skipRadio) skipRadio.checked = false;
                selectedValue = (e as CustomEvent<{ value: string }>).detail.value;
                okBtn.disabled = false;
            });

            // Skip card selection via its radio input.
            this.#options.addEventListener("change", (e: Event) => {
                const target = e.target as HTMLInputElement;
                if (target.id === "skip") {
                    for (const card of Array.from(this.#options.querySelectorAll("b3tty-palette-card"))) {
                        card.removeAttribute("selected");
                    }
                    selectedValue = "skip";
                    okBtn.disabled = false;
                }
            });

            okBtn.addEventListener("click", async () => {
                if (!selectedValue) return;
                await postSaveConfig(selectedValue);
                window.location.reload();
            });
        }

        connectedCallback(): void {
            // Runs on every attach, so moving the element must not fetch and add the cards again.
            if (this.#palettesRequested) return;
            this.#palettesRequested = true;

            Promise.all([getThemePalette("b3tty-dark"), getThemePalette("b3tty-light")])
                .then(([dark, light]) => {
                    const lightCard = document.createElement("b3tty-palette-card");
                    (lightCard as unknown as B3ttyPaletteCard).setup("b3tty-light", "B3tty Light", light);
                    const darkCard = document.createElement("b3tty-palette-card");
                    (darkCard as unknown as B3ttyPaletteCard).setup("b3tty-dark", "B3tty Dark", dark);
                    this.#options.prepend(lightCard);
                    this.#options.prepend(darkCard);
                })
                .catch(() => {
                    // Palette cards remain absent; the user can still select "No theme".
                });
        }
    }

    customElements.define("b3tty-theme-selector", B3ttyThemeSelectorImpl);
}
