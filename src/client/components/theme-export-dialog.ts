import { getThemePackage, postExportTheme } from "../api.ts";
import type { ThemeMetadata, ThemeSource } from "../types.ts";
import { closeStackedOverlay } from "./overlay.ts";
import {
    canExportTheme,
    initialThemeExportState,
    reduceThemeExport,
    sameAsLastVersion,
    showRepoError,
    themeExportMetadata,
    type ThemeExportAction,
    type ThemeExportBlock,
    type ThemeExportState,
} from "./theme-export-dialog-state.ts";
import { buildThemeFileDialog, focusPathInput, type ThemeFileDialogParts } from "./theme-file-dialog.ts";

/** What the dialog says when there is nothing it can export. */
const UNAVAILABLE_MESSAGES: Record<ThemeExportBlock, string> = {
    noTheme: "There is no active theme to export.",
    builtin: "Built-in themes can't be exported. Select or create a user-defined theme to export.",
    installed: "Installed themes can't be exported. Copy it to make your own version, then export that.",
};

/**
 * Options for B3ttyThemeExportDialog.open(). `unsavedChanges` shows a note that the
 * saved version is what gets exported; `source` is themeName's source (default
 * "user"), since only user themes can be exported; `opener` is as for the import
 * dialog.
 */
export interface ThemeExportOptions {
    unsavedChanges?: boolean;
    source?: ThemeSource;
    opener?: HTMLElement | null;
}

/**
 * Interface for the b3tty-theme-export-dialog web component. An empty themeName
 * (no active theme) or a built-in or installed one shows a message and leaves Export
 * disabled.
 */
export interface B3ttyThemeExportDialog {
    open(themeName: string, options?: ThemeExportOptions): void;
    close(): void;
}

/**
 * Returns true when el is a b3tty-theme-export-dialog element.
 */
export function isB3ttyThemeExportDialog(el: Element): el is HTMLElement & B3ttyThemeExportDialog {
    return el.tagName.toLowerCase() === "b3tty-theme-export-dialog";
}

if (typeof HTMLElement !== "undefined") {
    class B3ttyThemeExportDialogImpl extends HTMLElement implements B3ttyThemeExportDialog {
        #parts: ThemeFileDialogParts;
        #opener: HTMLElement | null = null;
        #state: ThemeExportState = initialThemeExportState;
        /** The intro #render last built, so it is only rebuilt when it changes. */
        #renderedIntro = "";
        #details: HTMLDivElement;
        #repoInput: HTMLInputElement;
        #repoError: HTMLSpanElement;
        #authorInput: HTMLInputElement;
        #themeVersionInput: HTMLInputElement;
        #versionNote: HTMLSpanElement;
        #descriptionInput: HTMLTextAreaElement;

        constructor() {
            super();
            this.#parts = buildThemeFileDialog(this, "Export Theme", "Export");
            const style = document.createElement("style");
            style.textContent = `
                .details { display: flex; flex-direction: column; gap: var(--space-md); }
                .details[hidden], .details [hidden] { display: none; }
                .details-row { display: grid; grid-template-columns: 1fr 120px; gap: var(--space-md); }
                .details .field-input { font-family: var(--font-family-ui); font-size: var(--font-size-md); }
                .details .field-input.repo { font-family: var(--font-family-mono); font-size: var(--font-size-sm); }
                .details textarea.field-input {
                    resize: vertical; min-height: 56px; line-height: var(--line-height-body);
                }
            `;
            this.shadowRoot?.appendChild(style);

            const makeInput = (placeholder: string): HTMLInputElement => {
                const input = document.createElement("input");
                input.type = "text";
                input.className = "field-input";
                input.placeholder = placeholder;
                input.autocomplete = "off";
                input.spellcheck = false;
                input.addEventListener("keydown", (e) => {
                    if (e.key === "Enter" && !this.#parts.actionBtn.disabled) this.#parts.actionBtn.click();
                });
                return input;
            };
            this.#repoInput = makeInput("https://github.com/you/your-theme");
            this.#repoInput.classList.add("repo");
            this.#repoError = document.createElement("span");
            this.#repoError.className = "field-error";
            this.#repoError.textContent = "Must be an https:// URL.";
            this.#authorInput = makeInput("Your name");
            this.#themeVersionInput = makeInput("1.0.0");
            this.#versionNote = document.createElement("span");
            this.#versionNote.className = "field-desc";
            this.#versionNote.hidden = true;
            this.#descriptionInput = document.createElement("textarea");
            this.#descriptionInput.className = "field-input";
            this.#descriptionInput.rows = 3;
            this.#descriptionInput.placeholder = "What the theme looks like or was inspired by";

            const repoField = this.#makeField("Repository", this.#repoInput);
            repoField.append(this.#repoError);
            const versionField = this.#makeField("Theme Version", this.#themeVersionInput);
            const row = document.createElement("div");
            row.className = "details-row";
            row.append(this.#makeField("Author", this.#authorInput), versionField);
            this.#details = document.createElement("div");
            this.#details.className = "details";
            this.#details.append(
                repoField,
                row,
                this.#versionNote,
                this.#makeField("Description", this.#descriptionInput)
            );
            this.#parts.actions.before(this.#details);

            this.#parts.input.addEventListener("input", () =>
                this.#dispatch({ type: "pathChanged", path: this.#parts.input.value })
            );
            for (const [field, el] of this.#detailInputs()) {
                el.addEventListener("input", () => this.#dispatch({ type: "fieldChanged", field, value: el.value }));
            }

            this.#parts.warning.textContent =
                "This theme has unsaved changes. The export will contain the last saved version.";
            this.#parts.hint.textContent =
                "An absolute path, or one starting with ~ for your home directory. .b3theme is added if missing, and an existing file is never overwritten.";
            this.#parts.actionBtn.addEventListener("click", () => void this.#export());
            this.#render();
        }

        open(themeName: string, options: ThemeExportOptions = {}): void {
            this.#opener = options.opener ?? null;
            this.#dispatch({
                type: "opened",
                themeName,
                source: options.source ?? "user",
                unsavedChanges: !!options.unsavedChanges,
            });
            this.setAttribute("open", "");
            if (this.#state.phase === "unavailable") {
                this.#parts.cancelBtn.focus();
            } else {
                focusPathInput(this.#parts.input);
                void this.#prefill(themeName, this.#state.openId);
            }
        }

        close(): void {
            const opener = this.#opener;
            this.#opener = null;
            closeStackedOverlay(this, opener);
        }

        #makeField(label: string, control: HTMLInputElement | HTMLTextAreaElement): HTMLLabelElement {
            const field = document.createElement("label");
            field.className = "field";
            const text = document.createElement("span");
            text.className = "field-label";
            text.textContent = label;
            field.append(text, control);
            return field;
        }

        #detailInputs(): Array<[keyof ThemeMetadata, HTMLInputElement | HTMLTextAreaElement]> {
            return [
                ["repo", this.#repoInput],
                ["author", this.#authorInput],
                ["themeVersion", this.#themeVersionInput],
                ["description", this.#descriptionInput],
            ];
        }

        #dispatch(action: ThemeExportAction): void {
            this.#state = reduceThemeExport(this.#state, action);
            this.#render();
        }

        /** Brings the DOM in line with #state. Safe to call at any time. */
        #render(): void {
            const state = this.#state;
            const { intro, warning, input, saveError, cancelBtn, actionBtn } = this.#parts;

            if (state.phase === "unavailable") {
                const message = UNAVAILABLE_MESSAGES[state.reason];
                if (this.#renderedIntro !== message) intro.textContent = message;
                this.#renderedIntro = message;
                input.value = "";
                input.placeholder = "";
                input.disabled = true;
                this.#details.hidden = true;
                warning.classList.remove("visible");
                saveError.classList.remove("visible");
                cancelBtn.disabled = false;
                actionBtn.disabled = true;
                return;
            }

            const exporting = state.phase === "exporting";
            if (this.#renderedIntro !== state.themeName) {
                const name = document.createElement("b");
                name.textContent = state.themeName;
                intro.replaceChildren("Save ", name, " as a .b3theme package you can share.");
                this.#renderedIntro = state.themeName;
            }
            // Values are set only when they differ, so typing isn't disturbed.
            if (input.value !== state.path) input.value = state.path;
            input.placeholder = `~/${state.themeName}.b3theme`;
            input.disabled = exporting;
            this.#details.hidden = false;
            for (const [field, el] of this.#detailInputs()) {
                if (el.value !== state.fields[field]) el.value = state.fields[field];
                el.disabled = exporting;
            }
            // The repository error waits until something is typed, so an empty form isn't shown as wrong.
            this.#repoError.classList.toggle("visible", showRepoError(state));
            this.#versionNote.textContent = `Same as the last export (${state.lastVersion}). Bump it so people who installed the theme see an upgrade.`;
            this.#versionNote.hidden = !sameAsLastVersion(state);
            warning.classList.toggle("visible", state.unsavedChanges);
            saveError.textContent = state.error ?? "";
            saveError.classList.toggle("visible", state.error !== null);
            // Cancel too, so the dialog can't close and reopen under a request in flight.
            cancelBtn.disabled = exporting;
            actionBtn.disabled = !canExportTheme(state);
        }

        /** Fills in the fields from the theme's last export; a failed fetch just leaves them empty. */
        async #prefill(themeName: string, openId: number): Promise<void> {
            try {
                const saved = await getThemePackage(themeName);
                this.#dispatch({ type: "prefillLoaded", openId, saved });
            } catch (err) {
                console.warn(`[b3tty] could not load package details for "${themeName}":`, err);
            }
        }

        async #export(): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "submitted" });
            const state = this.#state;
            if (state === before || state.phase !== "exporting") return;
            try {
                await postExportTheme(state.themeName, state.path.trim(), themeExportMetadata(state.fields));
                this.close();
            } catch (err) {
                this.#dispatch({ type: "failed", message: err instanceof Error ? err.message : String(err) });
            }
        }
    }

    customElements.define("b3tty-theme-export-dialog", B3ttyThemeExportDialogImpl);
}
