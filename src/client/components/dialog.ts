import { BASE_STYLES, BUTTON_STYLES, DESIGN_TOKENS, OVERLAY_STYLES } from "../design.ts";

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
 * Returns true when el exposes the B3ttyDialog contract (show/hide methods).
 * Use this instead of `as unknown as B3ttyDialog` to preserve type safety.
 */
export function isB3ttyDialog(el: Element): el is HTMLElement & B3ttyDialog {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["show"] === "function" && typeof r["hide"] === "function";
}

if (typeof HTMLElement !== "undefined") {
    class B3ttyDialogImpl extends HTMLElement implements B3ttyDialog {
        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });
            shadow.innerHTML = `
                <style>
                    ${DESIGN_TOKENS}
                    ${BASE_STYLES}
                    ${OVERLAY_STYLES}
                    ${BUTTON_STYLES}
                    .modal {
                        padding: var(--space-3xl);
                        display: flex;
                        flex-direction: column;
                        align-items: center;
                        gap: var(--space-2xl);
                    }
                    p {
                        margin: 0;
                        font-size: var(--font-size-lg);
                    }
                </style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true">
                        <p></p>
                        <button class="ok-btn">OK</button>
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
}
