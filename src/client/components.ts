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
}
