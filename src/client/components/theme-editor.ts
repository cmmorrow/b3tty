import { getThemeConfig, postCopyTheme, postDeleteTheme, postEditTheme } from "../api.ts";
import {
    DEFAULT_BACKGROUND_IMAGE_TRANSPARENCY,
    type Palette,
    type ThemeEntry,
    type ThemeLists,
    type ThemeSource,
} from "../types.ts";
import { isValidBackgroundImageTransparency, isValidThemeColor } from "../validators.ts";
import {
    BASE_STYLES,
    BUTTON_STYLES,
    DESIGN_TOKENS,
    EDITOR_CARD_STYLES,
    FORM_INPUT_STYLES,
    OVERLAY_STYLES,
    PALETTE_CARD_VARS,
    SAVE_ERROR_STYLES,
    SPLIT_PANEL_STYLES,
    TEXT_STYLES,
} from "../design.ts";
import { closeOverlay } from "./overlay.ts";
import {
    type B3ttyPaletteCard,
    cardDetails,
    fetchPaletteCards,
    formatThemeName,
    type PaletteCardEntry,
    THEME_SECTIONS,
} from "./palette-card.ts";

/**
 * Options for B3ttyThemeEditor.open(): `select` names a theme to select once the list
 * has loaded, and `action` then starts that theme's Copy… or Delete… right away (the
 * command palette's copy:theme and delete:theme).
 */
export interface ThemeEditorOpenOptions {
    select?: string;
    action?: "copy" | "delete";
}

/**
 * Interface for the b3tty-theme-editor web component. refresh() rebuilds the theme
 * list of an open editor, e.g. after an import, and selects `select` when given.
 * After a copy or delete it dispatches b3tty-themes-changed (bubbles + composed) with
 * the updated ThemeLists.
 */
export interface B3ttyThemeEditor {
    open(themes: ThemeEntry[], options?: ThemeEditorOpenOptions): void;
    refresh(themes: ThemeEntry[], select?: string): Promise<void>;
    close(): void;
}

/**
 * Returns true when el is a b3tty-theme-editor element.
 */
export function isB3ttyThemeEditor(el: Element): el is HTMLElement & B3ttyThemeEditor {
    return el.tagName.toLowerCase() === "b3tty-theme-editor";
}

/**
 * Detail of the b3tty-open-theme-import event the theme editor dispatches when its
 * "Import theme…" card is clicked. `opener` is the element to refocus on close.
 */
export interface ThemeImportRequest {
    opener: HTMLElement;
}

/**
 * Detail of the b3tty-open-theme-export event the theme editor dispatches when its
 * Export… button is clicked. `unsavedChanges` is true when the editor's inputs differ
 * from the saved theme, which is what gets exported.
 */
export interface ThemeExportRequest {
    name: string;
    unsavedChanges: boolean;
    opener: HTMLElement;
}

/** Options for PromptLayer.ask(). */
interface PromptOptions {
    title: string;
    message: Array<string | Node>;
    /** A text field to ask for a value; validate returns why a value is unusable, or null. */
    input?: { label: string; value: string; validate(value: string): string | null };
    actionLabel: string;
    /** Styles the action button as destructive (.delete-btn). */
    destructive?: boolean;
    /** Runs the action with the field's value; a thrown error is shown and the prompt stays open. */
    submit(value: string): Promise<void>;
    /** Where focus returns when the prompt closes. */
    opener: HTMLElement | null;
}

/** A small modal that asks for a value or a confirmation; see buildPromptLayer. */
interface PromptLayer {
    /** Resolves true once submit succeeded, or false when the prompt was cancelled. */
    ask(options: PromptOptions): Promise<boolean>;
}

/**
 * Styles for buildPromptLayer. The host also needs OVERLAY_STYLES (.modal, .actions),
 * TEXT_STYLES, FORM_INPUT_STYLES, BUTTON_STYLES, and SAVE_ERROR_STYLES.
 */
const PROMPT_STYLES = `
    .prompt-overlay {
        position: fixed; inset: 0; z-index: var(--z-overlay-stacked);
        display: none; align-items: center; justify-content: center;
        background: var(--color-overlay);
        padding: var(--space-2xl); box-sizing: border-box;
    }
    .prompt-overlay.open { display: flex; }
    /* .modal.prompt outranks the host's own .modal rule (the theme editor's is a
       fixed-height row), so the prompt sizes to its content like the import dialog. */
    .modal.prompt {
        width: min(460px, 100%); height: auto; overflow: visible; padding: var(--space-2xl);
        display: flex; flex-direction: column; gap: var(--space-lg);
    }
    .prompt-message {
        margin: 0; font-size: var(--font-size-md); line-height: var(--line-height-body);
        overflow-wrap: anywhere;
    }
    .prompt-field { display: flex; flex-direction: column; gap: var(--space-2xs); }
    .prompt-field[hidden] { display: none; }
    .prompt .field-input { width: 100%; }
    .prompt .delete-btn { margin-right: 0; }
`;

/**
 * Builds a prompt modal into a component's shadow root, stacked above the component's
 * own overlay: a title, a message, an optional text field with an inline validation
 * error, and an actions row of a .save-error, Cancel, and the action button. Enter in
 * the field submits; Escape cancels. The theme editor uses it for Copy… and Delete….
 */
function buildPromptLayer(root: ShadowRoot): PromptLayer {
    const overlay = document.createElement("div");
    overlay.className = "prompt-overlay";
    const box = document.createElement("div");
    box.className = "modal prompt";
    box.setAttribute("role", "alertdialog");
    box.setAttribute("aria-modal", "true");
    const heading = document.createElement("h2");
    heading.className = "modal-title";
    const message = document.createElement("p");
    message.className = "prompt-message";

    const field = document.createElement("label");
    field.className = "prompt-field";
    const fieldLabel = document.createElement("span");
    fieldLabel.className = "field-label";
    const input = document.createElement("input");
    input.type = "text";
    input.className = "field-input";
    input.spellcheck = false;
    input.autocomplete = "off";
    const fieldError = document.createElement("span");
    fieldError.className = "field-error";
    field.append(fieldLabel, input, fieldError);

    const actions = document.createElement("div");
    actions.className = "actions";
    const saveError = document.createElement("span");
    saveError.className = "save-error";
    const cancelBtn = document.createElement("button");
    cancelBtn.className = "cancel-btn";
    cancelBtn.textContent = "Cancel";
    const actionBtn = document.createElement("button");
    actions.append(saveError, cancelBtn, actionBtn);

    box.append(heading, message, field, actions);
    overlay.appendChild(box);
    root.appendChild(overlay);

    let current: { options: PromptOptions; resolve(ok: boolean): void } | null = null;
    let busy = false;

    const update = (): void => {
        const validate = current?.options.input?.validate;
        const reason = validate ? validate(input.value) : null;
        // An empty field is unusable but not yet an error worth showing.
        fieldError.textContent = reason ?? "";
        fieldError.classList.toggle("visible", !!reason && input.value !== "");
        actionBtn.disabled = busy || !!reason;
        cancelBtn.disabled = busy;
    };
    const finish = (ok: boolean): void => {
        const c = current;
        current = null;
        overlay.classList.remove("open");
        c?.options.opener?.focus();
        c?.resolve(ok);
    };
    const submit = async (): Promise<void> => {
        if (!current || actionBtn.disabled) return;
        busy = true;
        saveError.classList.remove("visible");
        update();
        try {
            await current.options.submit(input.value);
            busy = false;
            finish(true);
        } catch (err) {
            busy = false;
            saveError.textContent = err instanceof Error ? err.message : String(err);
            saveError.classList.add("visible");
            update();
        }
    };

    input.addEventListener("input", () => {
        saveError.classList.remove("visible");
        update();
    });
    cancelBtn.addEventListener("click", () => finish(false));
    actionBtn.addEventListener("click", () => void submit());
    box.addEventListener("keydown", (e) => {
        if (e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            if (!busy) finish(false);
        } else if (e.key === "Enter" && e.target === input) {
            e.preventDefault();
            void submit();
        }
    });

    return {
        ask(options: PromptOptions): Promise<boolean> {
            if (current) finish(false);
            heading.textContent = options.title;
            message.replaceChildren(...options.message);
            field.hidden = !options.input;
            fieldLabel.textContent = options.input?.label ?? "";
            input.value = options.input?.value ?? "";
            actionBtn.textContent = options.actionLabel;
            actionBtn.className = options.destructive ? "delete-btn" : "ok-btn";
            saveError.classList.remove("visible");
            busy = false;
            return new Promise((resolve) => {
                current = { options, resolve };
                overlay.classList.add("open");
                update();
                if (options.input) {
                    input.focus();
                    input.select();
                } else {
                    // A confirmation starts on Cancel, so Enter doesn't confirm by accident.
                    cancelBtn.focus();
                }
            });
        },
    };
}

/** The read-only note shown when a theme of each non-editable source is selected. */
const READ_ONLY_NOTES: Partial<Record<ThemeSource, string>> = {
    builtin: "Built-in themes can't be edited. Copy it to make your own.",
    installed: "Installed themes can't be edited. Copy it to make your own.",
};

const THEME_EDITOR_STYLES = `
    ${DESIGN_TOKENS}
    ${BASE_STYLES}
    ${OVERLAY_STYLES}
    ${TEXT_STYLES}
    .modal {
        padding: var(--space-2xl);
        display: flex; flex-direction: row;
        width: min(803px, 100%); height: min(617px, 90vh);
        overflow: hidden;
    }
    ${SPLIT_PANEL_STYLES}
    .left-panel { width: 240px; overflow: hidden; }
    .theme-list {
        flex: 1; min-height: 0; overflow-y: auto;
        display: flex; flex-direction: column; gap: var(--space-xs);
    }
    .list-section { display: flex; flex-direction: column; gap: var(--space-xs); }
    .list-section .section-title { margin: var(--space-xs) 0 0; }
    .list-footer {
        flex-shrink: 0; display: flex; gap: var(--space-xs);
        padding-top: var(--space-md); border-top: 1px solid var(--color-border-inner);
    }
    /* Same size as the actions row's Cancel and OK. */
    .list-footer .cancel-btn {
        flex: 1; padding-left: var(--space-sm); padding-right: var(--space-sm);
    }
    .list-footer .cancel-btn:disabled { background: var(--color-surface-2); }
    ${EDITOR_CARD_STYLES}
    ${PALETTE_CARD_VARS}
    b3tty-palette-card { flex-shrink: 0; }
    .name-section {
        display: flex; flex-direction: column; gap: var(--space-2xs);
        flex-shrink: 0; padding-bottom: var(--space-md);
        border-bottom: 1px solid var(--color-border-inner);
        margin-bottom: var(--space-sm);
    }
    .name-row {
        display: flex; align-items: center; gap: var(--space-sm);
    }
    .name-section label { white-space: nowrap; min-width: 80px; }
    .color-form {
        flex: 1; overflow-y: auto; min-height: 0;
        display: flex; flex-direction: column; gap: var(--space-2xs);
    }
    .color-form .section-title { padding: var(--space-xs) 0 var(--space-3xs); }
    .color-form input:disabled { opacity: var(--opacity-disabled); cursor: not-allowed; }
    .field-row {
        display: grid;
        grid-template-columns: 140px 1fr 22px;
        gap: var(--space-2xs); align-items: center;
    }
    .ansi-header, .ansi-row {
        display: grid;
        grid-template-columns: 68px 1fr 22px 1fr 22px;
        gap: var(--space-2xs); align-items: center;
    }
    .color-form .ansi-header { padding: 0 0 var(--space-3xs); }
    .field-row .field-desc, .field-row .field-error { grid-column: 2 / 4; }
    ${FORM_INPUT_STYLES}
    .swatch {
        width: 18px; height: 18px;
        border-radius: var(--radius-sm); border: 1px solid var(--color-border-subtle);
        visibility: hidden;
    }
    .swatch.visible { visibility: visible; }
    ${BUTTON_STYLES}
    ${SAVE_ERROR_STYLES}
    ${PROMPT_STYLES}
`;

/** The theme fields edited as colors, keyed by their Theme property. */
const CORE_COLORS = [
    { key: "background", label: "Background" },
    { key: "foreground", label: "Foreground" },
    { key: "cursor", label: "Cursor" },
    { key: "cursorAccent", label: "Cursor Accent" },
    { key: "selectionBackground", label: "Selection Background" },
    { key: "selectionForeground", label: "Selection Foreground" },
];
const ANSI_COLORS = [
    { label: "Black", normal: "black", bright: "brightBlack" },
    { label: "Red", normal: "red", bright: "brightRed" },
    { label: "Yellow", normal: "yellow", bright: "brightYellow" },
    { label: "Green", normal: "green", bright: "brightGreen" },
    { label: "Cyan", normal: "cyan", bright: "brightCyan" },
    { label: "Blue", normal: "blue", bright: "brightBlue" },
    { label: "Magenta", normal: "magenta", bright: "brightMagenta" },
    { label: "White", normal: "white", bright: "brightWhite" },
];

// These build markup from the hard-coded values in this file. Theme data must never
// pass through them; it reaches the form later, through input values.
const colorField = (key: string): string => `
    <input type="text" class="color-input" placeholder="#rrggbb" data-color="${key}">
    <span class="swatch" data-swatch="${key}"></span>
`;
const fieldNote = (className: string, text: string): string => `
    <div class="field-row"><span class="${className}">${text}</span></div>
`;

// .theme-list holds the theme cards, grouped by source (see #renderList), and scrolls
// between the cards above it and the footer below. The footer's Copy… and Delete… act
// on the selected theme, so both are disabled while "Create new theme" is selected.
// (HTML comments can't go in these templates: the design system inlines the bundle,
// which must not contain "<!--".)
const THEME_EDITOR_LEFT_PANEL = `
    <div class="left-panel">
        <div class="create-card">
            <input type="radio" name="theme" checked>
            <span>Create new theme</span>
        </div>
        <button class="action-card import-card">
            <span class="action-card-icon" aria-hidden="true">⤓</span>
            <span>Import theme…</span>
        </button>
        <div class="theme-list"></div>
        <div class="list-footer">
            <button class="cancel-btn copy-btn" disabled>Copy…</button>
            <button class="cancel-btn delete-theme-btn" disabled>Delete…</button>
        </div>
    </div>
`;

// Export… exports the selected saved user theme, so it is disabled while "Create new
// theme" or a built-in or installed theme is selected.
const THEME_EDITOR_NAME_SECTION = `
    <div class="name-section">
        <div class="name-row">
            <label class="field-label">Theme Name</label>
            <input type="text" class="name-input" placeholder="Enter theme name">
            <button class="inline-btn" disabled>Export…</button>
        </div>
        <span class="name-error field-error">Cannot use a built-in or installed theme name</span>
        <div class="read-only-note field-desc" role="status" hidden></div>
    </div>
`;

const THEME_EDITOR_COLOR_FORM = `
    <div class="color-form">
        <div class="section-title">Core Colors</div>
        ${CORE_COLORS.map(
            ({ key, label }) => `
                <div class="field-row">
                    <label class="field-label">${label}</label>
                    ${colorField(key)}
                </div>
            `
        ).join("")}

        <div class="section-title">ANSI Colors</div>
        <div class="ansi-header section-title">
            <span></span>
            <span>Normal</span>
            <span></span>
            <span>Bright</span>
            <span></span>
        </div>
        ${ANSI_COLORS.map(
            ({ label, normal, bright }) => `
                <div class="ansi-row">
                    <span class="color-label field-label">${label}</span>
                    ${colorField(normal)}
                    ${colorField(bright)}
                </div>
            `
        ).join("")}

        <div class="section-title">Background Image</div>
        <div class="field-row">
            <label class="field-label">Image Path</label>
            <input type="text" class="field-input bg-image-input" placeholder="/path/to/image.png or ~/image.png">
        </div>
        ${fieldNote(
            "field-desc",
            "Absolute path to a PNG, JPEG, GIF, or WebP file on the server. Leave blank for no image."
        )}
        <div class="field-row">
            <label class="field-label">Transparency</label>
            <input type="number" class="number-input bg-transparency-input" min="0" max="100" step="1"
                placeholder="${DEFAULT_BACKGROUND_IMAGE_TRANSPARENCY}">
        </div>
        ${fieldNote("field-error transparency-error", "Must be a whole number from 0 to 100")}
        ${fieldNote(
            "field-desc",
            `How strongly the theme background tints the image: 0 shows the image fully, 100 hides it. Default ${DEFAULT_BACKGROUND_IMAGE_TRANSPARENCY}.`
        )}
    </div>
`;

const THEME_EDITOR_RIGHT_PANEL = `
    <div class="right-panel">
        ${THEME_EDITOR_NAME_SECTION}
        ${THEME_EDITOR_COLOR_FORM}
        <div class="actions">
            <span class="save-error"></span>
            <button class="cancel-btn">Cancel</button>
            <button class="ok-btn" disabled>OK</button>
        </div>
    </div>
`;

if (typeof HTMLElement !== "undefined") {
    class B3ttyThemeEditorImpl extends HTMLElement implements B3ttyThemeEditor {
        #shadow: ShadowRoot;
        #leftPanel: HTMLDivElement;
        #themeList: HTMLDivElement;
        #nameInput: HTMLInputElement;
        #nameError: HTMLSpanElement;
        #readOnlyNote: HTMLDivElement;
        #saveError: HTMLSpanElement;
        #colorInputs: Map<string, HTMLInputElement> = new Map();
        #swatches: Map<string, HTMLSpanElement> = new Map();
        // Kept out of #colorInputs so the color loops, swatches, and palette
        // preview never see them; they take effect when the theme is saved.
        #bgImageInput: HTMLInputElement;
        #bgTransparencyInput: HTMLInputElement;
        #bgTransparencyError: HTMLSpanElement;
        #okBtn: HTMLButtonElement;
        #cancelBtn: HTMLButtonElement;
        #exportBtn: HTMLButtonElement;
        #copyBtn: HTMLButtonElement;
        #deleteBtn: HTMLButtonElement;
        #prompt: PromptLayer;
        // The selected theme's inputs as loaded, to tell whether the form has unsaved
        // changes; null when no saved theme is loaded.
        #savedSnapshot: string | null = null;
        #selectedName: string | null = null;
        #selectedCard: HTMLElement | null = null;
        #selectedPalette: Palette | null = null;
        #paletteCache: Map<string, Palette> = new Map();
        #createCard: HTMLDivElement;
        #createRadio: HTMLInputElement;
        #isLoading = false;
        /** True while an edit-theme request is in flight. */
        #saving = false;
        /** Every theme, by name, as last passed to open() or refresh(). */
        #entries: Map<string, ThemeEntry> = new Map();
        /** True while a built-in or installed theme is selected. */
        #readOnly = false;
        /** Bumped on every refresh, so palettes fetched for an earlier one are ignored. */
        #refreshId = 0;

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            const shadow = this.#shadow;
            shadow.innerHTML = `
                <style>${THEME_EDITOR_STYLES}</style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true">
                        ${THEME_EDITOR_LEFT_PANEL}
                        ${THEME_EDITOR_RIGHT_PANEL}
                    </div>
                </div>
            `;
            this.#leftPanel = shadow.querySelector<HTMLDivElement>(".left-panel")!;
            this.#createCard = shadow.querySelector<HTMLDivElement>(".create-card")!;
            this.#createRadio = this.#createCard.querySelector<HTMLInputElement>("input")!;
            this.#themeList = shadow.querySelector<HTMLDivElement>(".theme-list")!;
            this.#copyBtn = shadow.querySelector<HTMLButtonElement>(".copy-btn")!;
            this.#deleteBtn = shadow.querySelector<HTMLButtonElement>(".delete-theme-btn")!;
            this.#nameInput = shadow.querySelector<HTMLInputElement>(".name-input")!;
            this.#nameError = shadow.querySelector<HTMLSpanElement>(".name-error")!;
            this.#exportBtn = shadow.querySelector<HTMLButtonElement>(".name-row .inline-btn")!;
            this.#readOnlyNote = shadow.querySelector<HTMLDivElement>(".read-only-note")!;
            this.#bgImageInput = shadow.querySelector<HTMLInputElement>(".bg-image-input")!;
            this.#bgTransparencyInput = shadow.querySelector<HTMLInputElement>(".bg-transparency-input")!;
            this.#bgTransparencyError = shadow.querySelector<HTMLSpanElement>(".transparency-error")!;
            this.#saveError = shadow.querySelector<HTMLSpanElement>(".right-panel .save-error")!;
            this.#okBtn = shadow.querySelector<HTMLButtonElement>(".right-panel .ok-btn")!;
            const importCard = shadow.querySelector<HTMLButtonElement>(".import-card")!;
            const colorForm = shadow.querySelector<HTMLDivElement>(".color-form")!;
            this.#cancelBtn = shadow.querySelector<HTMLButtonElement>(".right-panel .actions .cancel-btn")!;

            for (const input of Array.from(colorForm.querySelectorAll<HTMLInputElement>("[data-color]"))) {
                this.#colorInputs.set(input.dataset.color!, input);
            }
            for (const swatch of Array.from(colorForm.querySelectorAll<HTMLSpanElement>("[data-swatch]"))) {
                this.#swatches.set(swatch.dataset.swatch!, swatch);
            }

            // Last, since setting the template would replace it.
            this.#prompt = buildPromptLayer(shadow);

            // Listeners
            importCard.addEventListener("click", () => {
                const detail: ThemeImportRequest = { opener: importCard };
                this.dispatchEvent(
                    new CustomEvent("b3tty-open-theme-import", { bubbles: true, composed: true, detail })
                );
            });
            this.#copyBtn.addEventListener("click", () => void this.#copySelected());
            this.#deleteBtn.addEventListener("click", () => void this.#deleteSelected());
            this.#exportBtn.addEventListener("click", () => {
                if (!this.#selectedName || this.#readOnly) return;
                const detail: ThemeExportRequest = {
                    name: this.#selectedName,
                    unsavedChanges: this.#hasUnsavedChanges(),
                    opener: this.#exportBtn,
                };
                this.dispatchEvent(
                    new CustomEvent("b3tty-open-theme-export", { bubbles: true, composed: true, detail })
                );
            });

            // One listener for every color input; each input's key names its swatch.
            colorForm.addEventListener("input", (e: Event) => {
                const input = e.target as HTMLInputElement;
                const key = input.dataset.color;
                if (!key) return;
                this.#updateSwatch(this.#swatches.get(key)!, input.value);
                this.#validateForm();
            });
            this.#bgTransparencyInput.addEventListener("input", () => this.#validateForm());

            this.#nameInput.addEventListener("input", () => this.#validateForm());

            this.#createCard.addEventListener("click", () => this.#selectCreate());

            this.#leftPanel.addEventListener("b3tty-card-select", (e: Event) => {
                this.#selectCard(e.target as HTMLElement);
            });

            this.#cancelBtn.addEventListener("click", () => this.close());
            this.#okBtn.addEventListener("click", () => void this.#handleOk());
        }

        #updateSwatch(swatch: HTMLSpanElement, value: string): void {
            if (value && isValidThemeColor(value)) {
                swatch.style.backgroundColor = value;
                swatch.classList.add("visible");
            } else {
                swatch.classList.remove("visible");
            }
        }

        /** Whether name belongs to a built-in or installed theme, which a user theme can't use. */
        #isReadOnlyName(name: string): boolean {
            const entry = this.#entries.get(name);
            return !!entry && entry.source !== "user";
        }

        #validateForm(): void {
            if (this.#isLoading || this.#readOnly || this.#saving) {
                this.#okBtn.disabled = true;
                this.#nameError.classList.remove("visible");
                this.#bgTransparencyError.classList.remove("visible");
                return;
            }
            this.#updatePreview();
            const transparencyValid = isValidBackgroundImageTransparency(this.#bgTransparencyInput.value);
            this.#bgTransparencyError.classList.toggle("visible", !transparencyValid);
            const name = this.#nameInput.value.trim();
            if (!name) {
                this.#okBtn.disabled = true;
                this.#nameError.classList.remove("visible");
                return;
            }
            if (this.#isReadOnlyName(name)) {
                this.#okBtn.disabled = true;
                this.#nameError.classList.add("visible");
                return;
            }
            this.#nameError.classList.remove("visible");
            for (const input of this.#colorInputs.values()) {
                if (input.value && !isValidThemeColor(input.value)) {
                    this.#okBtn.disabled = true;
                    return;
                }
            }
            this.#okBtn.disabled = !transparencyValid;
        }

        /**
         * Puts the right panel in or out of its read-only state: a built-in or installed
         * theme is shown with every field disabled and a note saying to copy it.
         */
        #setReadOnly(source: ThemeSource | null): void {
            const note = source ? READ_ONLY_NOTES[source] : undefined;
            this.#readOnly = !!note;
            for (const input of this.#colorInputs.values()) input.disabled = this.#readOnly;
            this.#bgImageInput.disabled = this.#readOnly;
            this.#bgTransparencyInput.disabled = this.#readOnly;
            this.#nameInput.readOnly = this.#readOnly;
            this.#readOnlyNote.textContent = note ?? "";
            this.#readOnlyNote.hidden = !note;
        }

        /** Enables Copy…, Delete…, and Export… for the selected theme. */
        #updateButtons(): void {
            const entry = this.#selectedName ? this.#entries.get(this.#selectedName) : undefined;
            this.#copyBtn.disabled = !entry;
            // A built-in can only be removed from the menu, so there's nothing to do
            // when it isn't in it.
            this.#deleteBtn.disabled = !entry || (entry.source === "builtin" && !entry.inMenu);
            this.#exportBtn.disabled = !entry || entry.source !== "user";
        }

        #restoreSelectedCard(): void {
            if (!this.#selectedCard || !this.#selectedName) return;
            const original = this.#paletteCache.get(this.#selectedName);
            const entry = this.#entries.get(this.#selectedName);
            if (original) {
                (this.#selectedCard as unknown as B3ttyPaletteCard).setup(
                    this.#selectedName,
                    formatThemeName(this.#selectedName),
                    original,
                    entry ? cardDetails(entry) : undefined
                );
            }
            this.#selectedCard = null;
            this.#selectedPalette = null;
        }

        #buildPreviewPalette(): Palette | null {
            if (!this.#selectedPalette) return null;
            const base = this.#selectedPalette;
            const get = (key: string): string | null => {
                const val = this.#colorInputs.get(key)?.value;
                return val && isValidThemeColor(val) ? val : null;
            };
            const normalKeys = ["black", "red", "yellow", "green", "cyan", "blue", "magenta", "white"];
            const brightKeys = [
                "brightBlack",
                "brightRed",
                "brightYellow",
                "brightGreen",
                "brightCyan",
                "brightBlue",
                "brightMagenta",
                "brightWhite",
            ];
            return {
                bg: get("background") ?? base.bg,
                fg: get("foreground") ?? base.fg,
                selBg: get("selectionBackground") ?? base.selBg,
                cursor: get("cursor") ?? base.cursor,
                normal: normalKeys.map((k, i) => get(k) ?? base.normal[i] ?? ""),
                bright: brightKeys.map((k, i) => get(k) ?? base.bright[i] ?? ""),
            };
        }

        #updatePreview(): void {
            if (!this.#selectedCard || !this.#selectedName || !this.#selectedPalette) return;
            const palette = this.#buildPreviewPalette();
            if (!palette) return;
            const entry = this.#entries.get(this.#selectedName);
            (this.#selectedCard as unknown as B3ttyPaletteCard).setup(
                this.#selectedName,
                formatThemeName(this.#selectedName),
                palette,
                entry ? cardDetails(entry) : undefined
            );
        }

        #snapshotInputs(): string {
            const values = Array.from(this.#colorInputs.values(), (input) => input.value);
            values.push(this.#bgImageInput.value, this.#bgTransparencyInput.value);
            return JSON.stringify(values);
        }

        #hasUnsavedChanges(): boolean {
            return this.#savedSnapshot !== null && this.#savedSnapshot !== this.#snapshotInputs();
        }

        #clearInputs(): void {
            this.#savedSnapshot = null;
            for (const [key, input] of this.#colorInputs) {
                input.value = "";
                const swatch = this.#swatches.get(key);
                if (swatch) swatch.classList.remove("visible");
            }
            this.#bgImageInput.value = "";
            this.#bgTransparencyInput.value = "";
            this.#bgTransparencyError.classList.remove("visible");
        }

        /** Selects "Create new theme": an empty, editable form. */
        #selectCreate(): void {
            for (const card of Array.from(this.#leftPanel.querySelectorAll("b3tty-palette-card"))) {
                card.removeAttribute("selected");
            }
            this.#createCard.setAttribute("selected", "");
            this.#createRadio.checked = true;
            this.#restoreSelectedCard();
            this.#selectedName = null;
            this.#setReadOnly(null);
            this.#updateButtons();
            this.#nameInput.value = "";
            this.#clearInputs();
            this.#isLoading = false;
            this.#validateForm();
        }

        /** Selects a theme's card and loads the theme into the right panel. */
        #selectCard(target: HTMLElement): void {
            for (const card of Array.from(this.#leftPanel.querySelectorAll("b3tty-palette-card"))) {
                if (card !== target) card.removeAttribute("selected");
            }
            target.setAttribute("selected", "");
            this.#createCard.removeAttribute("selected");
            this.#createRadio.checked = false;
            const name = (target as unknown as B3ttyPaletteCard).value;
            this.#restoreSelectedCard();
            this.#selectedName = name;
            this.#selectedCard = target;
            this.#selectedPalette = this.#paletteCache.get(name) ?? null;
            this.#setReadOnly(this.#entries.get(name)?.source ?? null);
            this.#updateButtons();
            this.#nameInput.value = name;
            this.#isLoading = true;
            this.#okBtn.disabled = true;
            this.#clearInputs();
            void this.#loadThemeColors(name);
        }

        /**
         * Rebuilds the theme list from #entries' cards: Your themes, Installed, and
         * Built-in, each left out when empty.
         */
        #renderList(cards: PaletteCardEntry[]): void {
            this.#themeList.replaceChildren();
            for (const { source, title } of THEME_SECTIONS) {
                const inSection = cards.filter((c) => c.entry.source === source);
                if (inSection.length === 0) continue;
                const section = document.createElement("div");
                section.className = "list-section";
                const body = document.createElement("div");
                body.className = "list-section";
                for (const { card } of inSection) body.appendChild(card);
                const heading = document.createElement("div");
                heading.className = "section-title";
                heading.textContent = title;
                section.append(heading, body);
                this.#themeList.appendChild(section);
            }
        }

        async refresh(themes: ThemeEntry[], select?: string): Promise<void> {
            const refreshId = ++this.#refreshId;
            this.#entries = new Map(themes.map((e) => [e.name, e]));
            const cards = await fetchPaletteCards(themes);
            if (refreshId !== this.#refreshId) return;
            this.#restoreSelectedCard();
            this.#paletteCache.clear();
            for (const { entry, palette } of cards) this.#paletteCache.set(entry.name, palette);
            this.#renderList(cards);
            const target = select ? cards.find((c) => c.entry.name === select)?.card : undefined;
            if (target) {
                this.#selectCard(target);
                target.scrollIntoView({ block: "nearest" });
            } else {
                this.#selectCreate();
            }
        }

        /** Tells terminal.ts that themes were added or removed, with the updated lists. */
        #themesChanged(lists: ThemeLists): void {
            this.dispatchEvent(
                new CustomEvent("b3tty-themes-changed", { detail: lists, bubbles: true, composed: true })
            );
        }

        /** A name for a copy of name that no theme has yet: name-copy, name-copy-2, … */
        #copyName(name: string): string {
            let candidate = `${name}-copy`;
            for (let i = 2; this.#entries.has(candidate); i++) candidate = `${name}-copy-${i}`;
            return candidate;
        }

        /** Why newName can't name a copy, or null; mirrors validateThemeName on the server. */
        #copyNameProblem(newName: string): string | null {
            if (!newName) return "Enter a name.";
            if (newName.trim() !== newName) return "The name can't start or end with a space.";
            if (newName.includes("/")) return "The name can't contain a /.";
            if (newName.length > 64) return "The name can't be longer than 64 characters.";
            if (this.#entries.has(newName)) return "A theme with this name already exists.";
            return null;
        }

        async #copySelected(): Promise<void> {
            const name = this.#selectedName;
            if (!name || this.#copyBtn.disabled) return;
            const bold = document.createElement("b");
            bold.textContent = name;
            await this.#prompt.ask({
                title: "Copy Theme",
                message: ["Save a copy of ", bold, " as a theme of your own, which you can edit and export."],
                input: {
                    label: "New theme name",
                    value: this.#copyName(name),
                    validate: (value) => this.#copyNameProblem(value),
                },
                actionLabel: "Copy",
                opener: this.#copyBtn,
                submit: async (newName) => {
                    const response = await postCopyTheme(name, newName);
                    const lists = { themeNames: response.themeNames ?? [], themes: response.themes ?? [] };
                    this.#themesChanged(lists);
                    await this.refresh(lists.themes, newName);
                },
            });
        }

        async #deleteSelected(): Promise<void> {
            const entry = this.#selectedName ? this.#entries.get(this.#selectedName) : undefined;
            if (!entry || this.#deleteBtn.disabled) return;
            const bold = document.createElement("b");
            bold.textContent = entry.name;
            const prompts: Record<ThemeSource, { title: string; message: Array<string | Node>; action: string }> = {
                user: {
                    title: "Delete Theme",
                    message: ["Delete ", bold, "? This can't be undone."],
                    action: "Delete",
                },
                installed: {
                    title: "Uninstall Theme",
                    message: [
                        "Uninstall ",
                        bold,
                        "? This can't be undone; you'll need the package file to install it again.",
                    ],
                    action: "Uninstall",
                },
                builtin: {
                    title: "Remove from Themes Menu",
                    message: ["Remove ", bold, " from the Themes menu? It stays available in Select Theme."],
                    action: "Remove",
                },
            };
            const { title, message, action } = prompts[entry.source];
            await this.#prompt.ask({
                title,
                message,
                actionLabel: action,
                destructive: true,
                opener: this.#deleteBtn,
                submit: async () => {
                    const lists = await postDeleteTheme(entry.name);
                    this.#themesChanged(lists);
                    // A built-in is still there, just no longer in the menu.
                    await this.refresh(lists.themes, entry.source === "builtin" ? entry.name : undefined);
                },
            });
        }

        async #loadThemeColors(name: string): Promise<void> {
            try {
                const config = await getThemeConfig(name);
                if (this.#selectedName !== name) return;
                const data = config as unknown as Record<string, string>;
                for (const [key, input] of this.#colorInputs) {
                    const value = data[key] ?? "";
                    input.value = value;
                    const swatch = this.#swatches.get(key);
                    if (swatch) this.#updateSwatch(swatch, value);
                }
                this.#bgImageInput.value = config.backgroundImage ?? "";
                // Left empty when unset so the placeholder shows the default.
                this.#bgTransparencyInput.value = config.backgroundImageTransparency?.toString() ?? "";
                this.#savedSnapshot = this.#snapshotInputs();
            } catch {
                // leave inputs as-is on failure
            } finally {
                if (this.#selectedName === name) {
                    this.#isLoading = false;
                    this.#validateForm();
                }
            }
        }

        async #handleOk(): Promise<void> {
            const name = this.#nameInput.value.trim();
            if (!name || this.#readOnly || this.#saving) return;
            const themeData: Record<string, string | number> = {};
            for (const [key, input] of this.#colorInputs) {
                if (input.value) themeData[key] = input.value;
            }
            // Always sent: an empty path clears the theme's background image.
            themeData["backgroundImage"] = this.#bgImageInput.value.trim();
            // Omitted when empty so the theme falls back to the default.
            if (this.#bgTransparencyInput.value !== "") {
                themeData["backgroundImageTransparency"] = Number(this.#bgTransparencyInput.value);
            }
            this.#saveError.classList.remove("visible");
            this.#setSaving(true);
            try {
                const response = await postEditTheme(name, themeData);
                this.dispatchEvent(
                    new CustomEvent("b3tty-theme-edited", {
                        detail: { name, response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.close();
            } catch (err) {
                // Keep editor open so the user can retry, but tell them why —
                // a save that silently does nothing looks indistinguishable
                // from a UI bug.
                this.#saveError.textContent = err instanceof Error ? err.message : "Failed to save theme.";
                this.#saveError.classList.add("visible");
            } finally {
                this.#setSaving(false);
            }
        }

        /**
         * Enters or leaves the saving state. While a request is in flight, OK and
         * Cancel stay disabled, so a double click can't send it twice and the editor can't
         * close under it. Leaving it re-checks the form, unless the save closed the editor.
         */
        #setSaving(saving: boolean): void {
            this.#saving = saving;
            this.#cancelBtn.disabled = saving;
            if (saving) this.#okBtn.disabled = true;
            else if (this.hasAttribute("open")) this.#validateForm();
        }

        open(themes: ThemeEntry[], options: ThemeEditorOpenOptions = {}): void {
            this.#selectedCard = null;
            this.#selectedPalette = null;
            this.#nameError.classList.remove("visible");
            this.#saveError.classList.remove("visible");
            this.#themeList.replaceChildren();
            this.#selectCreate();
            this.setAttribute("open", "");

            void this.refresh(themes, options.select).then(() => {
                if (!options.action || !options.select || this.#selectedName !== options.select) return;
                if (options.action === "copy") void this.#copySelected();
                else void this.#deleteSelected();
            });
        }

        close(): void {
            this.#restoreSelectedCard();
            this.#selectedName = null;
            this.#nameError.classList.remove("visible");
            this.#saveError.classList.remove("visible");
            closeOverlay(this);
        }
    }

    customElements.define("b3tty-theme-editor", B3ttyThemeEditorImpl);
}
