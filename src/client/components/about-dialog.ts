import { BASE_STYLES, BUTTON_STYLES, DESIGN_TOKENS, OVERLAY_STYLES, TEXT_STYLES } from "../design.ts";
import { closeOverlay } from "./overlay.ts";

/**
 * Version string baked in at build time by bun's --define (see Makefile's
 * client target). Under `bun test`, which runs this module unbundled with no
 * --define applied, __B3TTY_VERSION__ is an undeclared identifier; `typeof`
 * on an undeclared identifier never throws, so this falls back to "dev"
 * instead of raising a ReferenceError. In the built bundle, bun replaces
 * every __B3TTY_VERSION__ occurrence with the VERSION file's literal before
 * this code runs, so the whole expression constant-folds to that literal.
 */
const B3TTY_VERSION: string = typeof __B3TTY_VERSION__ !== "undefined" ? __B3TTY_VERSION__ : "dev";

/**
 * Interface for the b3tty-about-dialog web component.
 */
export interface B3ttyAboutDialog {
    open(): void;
    close(): void;
}

/**
 * Returns true when el is a b3tty-about-dialog element.
 */
export function isB3ttyAboutDialog(el: Element): el is HTMLElement & B3ttyAboutDialog {
    return el.tagName.toLowerCase() === "b3tty-about-dialog";
}

if (typeof HTMLElement !== "undefined") {
    class B3ttyAboutDialogImpl extends HTMLElement implements B3ttyAboutDialog {
        #shadow: ShadowRoot;

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
                        display: flex; flex-direction: column; align-items: center; gap: var(--space-xs);
                        max-width: 420px; width: 100%;
                        text-align: center;
                    }
                    .title { font-weight: var(--font-weight-bold); }
                    .byline {
                        margin: 0; font-size: var(--font-size-md); font-weight: var(--font-weight-regular);
                        color: var(--color-text-subtle);
                    }
                    .version {
                        margin: 0; font-size: var(--font-size-xs);
                        color: var(--color-muted);
                    }
                    .actions { justify-content: center; margin-top: var(--space-lg); }
                    ${BUTTON_STYLES}
                </style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true">
                        <p class="title modal-title">b3tty - A better, browser-based terminal emulator</p>
                        <p class="byline">By Chris Morrow</p>
                        <p class="version">Version ${B3TTY_VERSION}</p>
                        <div class="actions">
                            <button class="ok-btn">OK</button>
                        </div>
                    </div>
                </div>
            `;
            this.#shadow.querySelector(".ok-btn")!.addEventListener("click", () => this.close());
        }

        open(): void {
            this.setAttribute("open", "");
        }

        close(): void {
            closeOverlay(this);
        }
    }

    customElements.define("b3tty-about-dialog", B3ttyAboutDialogImpl);
}
