import { postImportTheme, ThemeUpgradeRequiredError } from "../api.ts";
import type { ImportThemeResponse } from "../types.ts";
import { closeStackedOverlay } from "./overlay.ts";
import { buildThemeFileDialog, focusPathInput, type ThemeFileDialogParts } from "./theme-file-dialog.ts";

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
        /** Set once the server said the package upgrades an installed theme. */
        #replace = false;

        constructor() {
            super();
            this.#parts = buildThemeFileDialog(this, "Import Theme", "Import");
            this.#parts.intro.textContent =
                "Install a theme from a .b3theme package. It will appear in Select Theme, ready to use or copy.";
            this.#parts.input.placeholder = "~/Downloads/theme.b3theme";
            this.#parts.hint.textContent = "An absolute path, or one starting with ~ for your home directory.";
            // A different path is a different package, so any pending Replace is undone.
            this.#parts.input.addEventListener("input", () => this.#setReplace(false));
            this.#parts.actionBtn.addEventListener("click", () => void this.#import());
        }

        open(opener: HTMLElement | null = null): void {
            this.#opener = opener;
            const { input, saveError, actionBtn } = this.#parts;
            input.value = "";
            input.disabled = false;
            this.#setReplace(false);
            actionBtn.disabled = true;
            saveError.classList.remove("visible");
            this.setAttribute("open", "");
            focusPathInput(input);
        }

        close(): void {
            this.#parts.saveError.classList.remove("visible");
            const opener = this.#opener;
            this.#opener = null;
            closeStackedOverlay(this, opener);
        }

        /**
         * Switches between importing and replacing an installed version, which shows
         * the server's description of the upgrade (including any change of author or
         * repository) and relabels the action button.
         */
        #setReplace(replace: boolean, message = ""): void {
            this.#replace = replace;
            const { warning, actionBtn } = this.#parts;
            warning.textContent = replace ? `${message} Replace the installed version?` : "";
            warning.classList.toggle("visible", replace);
            actionBtn.textContent = replace ? "Replace" : "Import";
        }

        async #import(): Promise<void> {
            const { input, saveError, actionBtn } = this.#parts;
            const path = input.value.trim();
            if (!path) return;
            saveError.classList.remove("visible");
            input.disabled = true;
            actionBtn.disabled = true;
            try {
                const response = await postImportTheme(path, this.#replace);
                const opener = this.#opener;
                this.close();
                const detail: ThemeImportedDetail = { response, opener };
                this.dispatchEvent(new CustomEvent("b3tty-theme-imported", { detail, bubbles: true, composed: true }));
            } catch (err) {
                if (err instanceof ThemeUpgradeRequiredError) {
                    this.#setReplace(true, err.message);
                } else {
                    saveError.textContent = err instanceof Error ? err.message : String(err);
                    saveError.classList.add("visible");
                }
            } finally {
                input.disabled = false;
                actionBtn.disabled = !input.value.trim();
            }
        }
    }

    customElements.define("b3tty-theme-import-dialog", B3ttyThemeImportDialogImpl);
}
