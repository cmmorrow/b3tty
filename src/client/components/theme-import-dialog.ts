import { postImportTheme, ThemeUpgradeRequiredError } from "../api.ts";
import type { ImportThemeResponse } from "../types.ts";
import { closeStackedOverlay } from "./overlay.ts";
import { buildThemeFileDialog, focusPathInput, type ThemeFileDialogParts } from "./theme-file-dialog.ts";
import {
    canSubmitThemeImport,
    initialThemeImportState,
    reduceThemeImport,
    type ThemeImportAction,
    type ThemeImportState,
} from "./theme-import-dialog-state.ts";

/**
 * Detail of the b3tty-theme-imported event the import dialog dispatches (bubbles +
 * composed) after a successful import: the server's response, and the element the
 * dialog was opened from (the theme editor's Import card), or null.
 */
export interface ThemeImportedDetail {
    response: ImportThemeResponse;
    opener: HTMLElement | null;
}

/**
 * Interface for the b3tty-theme-import-dialog web component. `opener` is the
 * element to return focus to on close; without one, closing dispatches
 * b3tty-overlay-close like the other overlays. A new version of an installed theme
 * shows the server's description of the upgrade and turns Import into Replace.
 */
export interface B3ttyThemeImportDialog {
    open(opener?: HTMLElement | null): void;
    close(): void;
}

/**
 * Returns true when el is a b3tty-theme-import-dialog element.
 */
export function isB3ttyThemeImportDialog(el: Element): el is HTMLElement & B3ttyThemeImportDialog {
    return el.tagName.toLowerCase() === "b3tty-theme-import-dialog";
}

if (typeof HTMLElement !== "undefined") {
    class B3ttyThemeImportDialogImpl extends HTMLElement implements B3ttyThemeImportDialog {
        #parts: ThemeFileDialogParts;
        #opener: HTMLElement | null = null;
        #state: ThemeImportState = initialThemeImportState;

        constructor() {
            super();
            this.#parts = buildThemeFileDialog(this, "Import Theme", "Import");
            this.#parts.intro.textContent =
                "Install a theme from a .b3theme package. It will appear in Select Theme, ready to use or copy.";
            this.#parts.input.placeholder = "~/Downloads/theme.b3theme";
            this.#parts.hint.textContent = "An absolute path, or one starting with ~ for your home directory.";
            this.#parts.input.addEventListener("input", () =>
                this.#dispatch({ type: "pathChanged", path: this.#parts.input.value })
            );
            this.#parts.actionBtn.addEventListener("click", () => void this.#import());
            this.#render();
        }

        open(opener: HTMLElement | null = null): void {
            this.#opener = opener;
            this.#dispatch({ type: "opened" });
            this.setAttribute("open", "");
            focusPathInput(this.#parts.input);
        }

        close(): void {
            const opener = this.#opener;
            this.#opener = null;
            closeStackedOverlay(this, opener);
        }

        #dispatch(action: ThemeImportAction): void {
            this.#state = reduceThemeImport(this.#state, action);
            this.#render();
        }

        /** Brings the DOM in line with #state. Safe to call at any time. */
        #render(): void {
            const { input, warning, saveError, cancelBtn, actionBtn } = this.#parts;
            const state = this.#state;
            const importing = state.phase === "importing";
            if (input.value !== state.path) input.value = state.path;
            input.disabled = importing;
            // The server's description of the upgrade names any change of author or repository.
            warning.textContent = state.upgrade === null ? "" : `${state.upgrade} Replace the installed version?`;
            warning.classList.toggle("visible", state.upgrade !== null);
            const error = state.phase === "editing" ? state.error : null;
            saveError.textContent = error ?? "";
            saveError.classList.toggle("visible", error !== null);
            // Cancel too, so the dialog can't close and reopen under a request in flight.
            cancelBtn.disabled = importing;
            actionBtn.textContent = state.upgrade === null ? "Import" : "Replace";
            actionBtn.disabled = !canSubmitThemeImport(state);
        }

        async #import(): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "submitted" });
            const state = this.#state;
            if (state === before || state.phase !== "importing") return;
            try {
                const response = await postImportTheme(state.path.trim(), state.upgrade !== null);
                const opener = this.#opener;
                this.close();
                const detail: ThemeImportedDetail = { response, opener };
                this.dispatchEvent(new CustomEvent("b3tty-theme-imported", { detail, bubbles: true, composed: true }));
            } catch (err) {
                if (err instanceof ThemeUpgradeRequiredError) {
                    this.#dispatch({ type: "upgradeRequired", message: err.message });
                } else {
                    this.#dispatch({ type: "failed", message: err instanceof Error ? err.message : String(err) });
                }
            }
        }
    }

    customElements.define("b3tty-theme-import-dialog", B3ttyThemeImportDialogImpl);
}
