import { getThemePackage, postExportTheme } from "../api.ts";
import type { ThemeMetadata, ThemeSource } from "../types.ts";
import { isValidRepoUrl } from "../validators.ts";
import { closeStackedOverlay } from "./overlay.ts";
import { buildThemeFileDialog, focusPathInput, type ThemeFileDialogParts } from "./theme-file-dialog.ts";

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
        #themeName = "";
        #details: HTMLDivElement;
        #repoInput: HTMLInputElement;
        #repoError: HTMLSpanElement;
        #authorInput: HTMLInputElement;
        #themeVersionInput: HTMLInputElement;
        #versionNote: HTMLSpanElement;
        #descriptionInput: HTMLTextAreaElement;
        /** The theme version of the last export, or "" for a theme never exported. */
        #lastVersion = "";
        /** Bumped on every open, so a slow package-details fetch for an earlier open is ignored. */
        #prefillRequestId = 0;

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

            // Registered after buildThemeFileDialog's own listener on the path field, so this
            // one decides whether Export is enabled.
            for (const el of [this.#parts.input, ...this.#detailInputs()]) {
                el.addEventListener("input", () => {
                    this.#parts.saveError.classList.remove("visible");
                    this.#updateState();
                });
            }

            this.#parts.warning.textContent =
                "This theme has unsaved changes. The export will contain the last saved version.";
            this.#parts.hint.textContent =
                "An absolute path, or one starting with ~ for your home directory. .b3theme is added if missing, and an existing file is never overwritten.";
            this.#parts.actionBtn.addEventListener("click", () => void this.#export());
        }

        open(themeName: string, options: ThemeExportOptions = {}): void {
            this.#opener = options.opener ?? null;
            const source = options.source ?? "user";
            const exportable = !!themeName && source === "user";
            this.#themeName = exportable ? themeName : "";
            this.#lastVersion = "";
            for (const el of this.#detailInputs()) el.value = "";
            this.#details.hidden = !exportable;
            const { intro, warning, input, saveError, cancelBtn } = this.#parts;
            if (exportable) {
                const name = document.createElement("b");
                name.textContent = themeName;
                intro.replaceChildren("Save ", name, " as a .b3theme package you can share.");
                input.value = `~/${themeName}.b3theme`;
                input.placeholder = `~/${themeName}.b3theme`;
            } else {
                intro.textContent = !themeName
                    ? "There is no active theme to export."
                    : source === "installed"
                      ? "Installed themes can't be exported. Copy it to make your own version, then export that."
                      : "Built-in themes can't be exported. Select or create a user-defined theme to export.";
                input.value = "";
                input.placeholder = "";
            }
            input.disabled = !exportable;
            this.#setDetailsDisabled(false);
            warning.classList.toggle("visible", exportable && !!options.unsavedChanges);
            saveError.classList.remove("visible");
            this.#updateState();
            this.setAttribute("open", "");
            if (exportable) {
                focusPathInput(input);
                void this.#prefill(themeName);
            } else {
                cancelBtn.focus();
            }
        }

        close(): void {
            this.#parts.saveError.classList.remove("visible");
            this.#prefillRequestId++;
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

        #detailInputs(): Array<HTMLInputElement | HTMLTextAreaElement> {
            return [this.#repoInput, this.#authorInput, this.#themeVersionInput, this.#descriptionInput];
        }

        #setDetailsDisabled(disabled: boolean): void {
            for (const el of this.#detailInputs()) el.disabled = disabled;
        }

        /**
         * Fills in the fields from the theme's last export. A field the user has already
         * typed into is left alone, and a failed fetch just leaves the fields empty.
         */
        async #prefill(themeName: string): Promise<void> {
            const requestId = ++this.#prefillRequestId;
            let saved: Partial<ThemeMetadata>;
            try {
                saved = await getThemePackage(themeName);
            } catch (err) {
                console.warn(`[b3tty] could not load package details for "${themeName}":`, err);
                return;
            }
            if (requestId !== this.#prefillRequestId) return;
            const fill = (el: HTMLInputElement | HTMLTextAreaElement, value: string | undefined): void => {
                if (value && !el.value) el.value = value;
            };
            fill(this.#repoInput, saved.repo);
            fill(this.#authorInput, saved.author);
            fill(this.#themeVersionInput, saved.themeVersion);
            fill(this.#descriptionInput, saved.description);
            this.#lastVersion = saved.themeVersion ?? "";
            this.#updateState();
        }

        #metadata(): ThemeMetadata {
            return {
                repo: this.#repoInput.value.trim(),
                author: this.#authorInput.value.trim(),
                description: this.#descriptionInput.value.trim(),
                themeVersion: this.#themeVersionInput.value.trim(),
            };
        }

        /**
         * Enables Export only when there is a path and every detail is filled in with a
         * valid repo, and shows the repo error and the unchanged-version note. The repo
         * error waits until something has been typed, so an empty form isn't shown as wrong.
         */
        #updateState(): void {
            const meta = this.#metadata();
            const repoValid = isValidRepoUrl(meta.repo);
            this.#repoError.classList.toggle("visible", !!meta.repo && !repoValid);
            const sameVersion = !!this.#lastVersion && meta.themeVersion === this.#lastVersion;
            this.#versionNote.textContent = `Same as the last export (${this.#lastVersion}). Bump it so people who installed the theme see an upgrade.`;
            this.#versionNote.hidden = !sameVersion;
            const complete = !!meta.author && !!meta.description && !!meta.themeVersion && repoValid;
            const { input, actionBtn } = this.#parts;
            actionBtn.disabled = !this.#themeName || input.disabled || !input.value.trim() || !complete;
        }

        async #export(): Promise<void> {
            const { input, saveError, actionBtn } = this.#parts;
            const path = input.value.trim();
            if (!this.#themeName || !path) return;
            saveError.classList.remove("visible");
            input.disabled = true;
            actionBtn.disabled = true;
            this.#setDetailsDisabled(true);
            try {
                await postExportTheme(this.#themeName, path, this.#metadata());
                this.close();
            } catch (err) {
                saveError.textContent = err instanceof Error ? err.message : String(err);
                saveError.classList.add("visible");
            } finally {
                input.disabled = false;
                this.#setDetailsDisabled(false);
                this.#updateState();
            }
        }
    }

    customElements.define("b3tty-theme-export-dialog", B3ttyThemeExportDialogImpl);
}
