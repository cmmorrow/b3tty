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
    createPaletteCard,
    fetchThemePalettes,
    formatThemeName,
    THEME_SECTIONS,
} from "./palette-card.ts";
import { askToDiscardChanges, buildPromptLayer, PROMPT_STYLES, type PromptLayer } from "./prompt-layer.ts";
import {
    canCopyTheme,
    canDeleteTheme,
    canExportTheme,
    canResetTheme,
    canSaveTheme,
    cardPalette,
    copyName,
    copyNameProblem,
    hasAnyUnsavedChanges,
    hasUnsavedChanges,
    initialThemeEditorState,
    isEditable,
    isReadOnly,
    nameIsTaken,
    newThemeHasUnsavedChanges,
    reduceThemeEditor,
    selectedSource,
    themeDataFromForm,
    themeEntry,
    themeHasUnsavedChanges,
    unsavedChanges,
    type ThemeColorKey,
    type ThemeEditorAction,
    type ThemeEditorState,
} from "./theme-editor-state.ts";

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
 * the updated ThemeLists. Save dispatches b3tty-theme-edited and keeps the editor
 * open; Close asks before discarding unsaved changes. close() closes unconditionally.
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
        display: flex; flex-direction: column; gap: var(--space-lg);
        width: min(803px, 100%); height: min(661px, 90vh);
        overflow: hidden;
    }
    .panels { flex: 1; min-height: 0; display: flex; flex-direction: row; }
    ${SPLIT_PANEL_STYLES}
    .left-panel { width: 252px; overflow: hidden; }
    /* The list's scrollbar overlays its content, so the list keeps a gutter on the right
       for it, clear of the selected card's outline; everything else in the panel leaves
       the same gap, so all the cards line up. */
    .left-panel > :not(.theme-list) { margin-right: var(--space-md); }
    .theme-list {
        flex: 1; min-height: 0; overflow-y: auto;
        display: flex; flex-direction: column; gap: var(--space-xs);
        padding-right: var(--space-md);
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
        /* Matches .color-form's scrollbar gutter, so Export lines up with the swatches;
           padding rather than margin, so the divider below still spans the panel. */
        padding-right: var(--space-md);
        border-bottom: 1px solid var(--color-border-inner);
        margin-bottom: var(--space-sm);
    }
    .name-row {
        display: flex; align-items: center; gap: var(--space-sm);
    }
    .name-section label { white-space: nowrap; min-width: 80px; }
    /* Lets the name field shrink to make room for Export… and Reset. */
    .name-row .name-input { min-width: 0; }
    /* Like .theme-list, the form keeps a right gutter for its overlay scrollbar. */
    .color-form {
        flex: 1; overflow-y: auto; min-height: 0;
        display: flex; flex-direction: column; gap: var(--space-2xs);
        padding-right: var(--space-md);
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
    .create-card .badge { margin-left: auto; font-weight: var(--font-weight-semibold); }
    .create-card .badge[hidden] { display: none; }
    ${BUTTON_STYLES}
    ${SAVE_ERROR_STYLES}
    /* A successful save reuses the error's slot, in the accent color. */
    .save-error.notice { color: var(--color-accent); font-weight: var(--font-weight-semibold); }
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
            <span class="badge accent" hidden>Unsaved</span>
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

// The card badge for a theme with unsaved changes.
const UNSAVED_BADGE = "Unsaved";

// Export… exports the selected saved user theme, so it is disabled while "Create new
// theme" or a built-in or installed theme is selected. Reset, beside it, throws away
// the form's unsaved changes.
const THEME_EDITOR_NAME_SECTION = `
    <div class="name-section">
        <div class="name-row">
            <label class="field-label">Theme Name</label>
            <input type="text" class="name-input" placeholder="Enter theme name">
            <button class="inline-btn export-btn" disabled>Export…</button>
            <button class="inline-btn reset-btn" disabled>Reset</button>
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
            <span class="save-error" role="status" aria-live="polite"></span>
            <button class="cancel-btn close-btn">Close</button>
            <button class="ok-btn" disabled>Save</button>
        </div>
    </div>
`;

if (typeof HTMLElement !== "undefined") {
    class B3ttyThemeEditorImpl extends HTMLElement implements B3ttyThemeEditor {
        #leftPanel: HTMLDivElement;
        #themeList: HTMLDivElement;
        #createCard: HTMLDivElement;
        #createRadio: HTMLInputElement;
        #createBadge: HTMLSpanElement;
        #nameInput: HTMLInputElement;
        #nameError: HTMLSpanElement;
        #readOnlyNote: HTMLDivElement;
        #saveError: HTMLSpanElement;
        #colorInputs: Map<ThemeColorKey, HTMLInputElement> = new Map();
        #swatches: Map<ThemeColorKey, HTMLSpanElement> = new Map();
        #bgImageInput: HTMLInputElement;
        #bgTransparencyInput: HTMLInputElement;
        #bgTransparencyError: HTMLSpanElement;
        #saveBtn: HTMLButtonElement;
        #closeBtn: HTMLButtonElement;
        #resetBtn: HTMLButtonElement;
        #exportBtn: HTMLButtonElement;
        #copyBtn: HTMLButtonElement;
        #deleteBtn: HTMLButtonElement;
        #prompt: PromptLayer;
        #state: ThemeEditorState = initialThemeEditorState;
        /** The palettes the list was last built from, so it is only rebuilt when they change. */
        #renderedPalettes: Record<string, Palette> | null = null;
        /** The palette and badge each card last showed, serialized, so setup() only runs when they change. */
        #cardPalettes = new WeakMap<HTMLElement, string>();

        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });
            shadow.innerHTML = `
                <style>${THEME_EDITOR_STYLES}</style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="theme-editor-title">
                        <h2 class="modal-title" id="theme-editor-title">Edit Theme</h2>
                        <div class="panels">
                            ${THEME_EDITOR_LEFT_PANEL}
                            ${THEME_EDITOR_RIGHT_PANEL}
                        </div>
                    </div>
                </div>
            `;
            this.#leftPanel = shadow.querySelector<HTMLDivElement>(".left-panel")!;
            this.#createCard = shadow.querySelector<HTMLDivElement>(".create-card")!;
            this.#createRadio = this.#createCard.querySelector<HTMLInputElement>("input")!;
            this.#createBadge = this.#createCard.querySelector<HTMLSpanElement>(".badge")!;
            this.#themeList = shadow.querySelector<HTMLDivElement>(".theme-list")!;
            this.#copyBtn = shadow.querySelector<HTMLButtonElement>(".copy-btn")!;
            this.#deleteBtn = shadow.querySelector<HTMLButtonElement>(".delete-theme-btn")!;
            this.#nameInput = shadow.querySelector<HTMLInputElement>(".name-input")!;
            this.#nameError = shadow.querySelector<HTMLSpanElement>(".name-error")!;
            this.#exportBtn = shadow.querySelector<HTMLButtonElement>(".name-row .export-btn")!;
            this.#readOnlyNote = shadow.querySelector<HTMLDivElement>(".read-only-note")!;
            this.#bgImageInput = shadow.querySelector<HTMLInputElement>(".bg-image-input")!;
            this.#bgTransparencyInput = shadow.querySelector<HTMLInputElement>(".bg-transparency-input")!;
            this.#bgTransparencyError = shadow.querySelector<HTMLSpanElement>(".transparency-error")!;
            this.#saveError = shadow.querySelector<HTMLSpanElement>(".right-panel .save-error")!;
            this.#saveBtn = shadow.querySelector<HTMLButtonElement>(".right-panel .ok-btn")!;
            this.#closeBtn = shadow.querySelector<HTMLButtonElement>(".right-panel .close-btn")!;
            this.#resetBtn = shadow.querySelector<HTMLButtonElement>(".name-row .reset-btn")!;
            const importCard = shadow.querySelector<HTMLButtonElement>(".import-card")!;
            const colorForm = shadow.querySelector<HTMLDivElement>(".color-form")!;

            for (const input of Array.from(colorForm.querySelectorAll<HTMLInputElement>("[data-color]"))) {
                this.#colorInputs.set(input.dataset.color as ThemeColorKey, input);
            }
            for (const swatch of Array.from(colorForm.querySelectorAll<HTMLSpanElement>("[data-swatch]"))) {
                this.#swatches.set(swatch.dataset.swatch as ThemeColorKey, swatch);
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
                const state = this.#state;
                if (!canExportTheme(state) || state.selected === null) return;
                const detail: ThemeExportRequest = {
                    name: state.selected,
                    unsavedChanges: hasUnsavedChanges(state),
                    opener: this.#exportBtn,
                };
                this.dispatchEvent(
                    new CustomEvent("b3tty-open-theme-export", { bubbles: true, composed: true, detail })
                );
            });

            // One listener for every color input; each input's key names its field.
            colorForm.addEventListener("input", (e: Event) => {
                const input = e.target as HTMLInputElement;
                const key = input.dataset.color as ThemeColorKey | undefined;
                if (key) this.#dispatch({ type: "fieldChanged", field: key, value: input.value });
            });
            this.#bgImageInput.addEventListener("input", () =>
                this.#dispatch({ type: "fieldChanged", field: "backgroundImage", value: this.#bgImageInput.value })
            );
            this.#bgTransparencyInput.addEventListener("input", () =>
                this.#dispatch({ type: "fieldChanged", field: "transparency", value: this.#bgTransparencyInput.value })
            );
            this.#nameInput.addEventListener("input", () =>
                this.#dispatch({ type: "fieldChanged", field: "name", value: this.#nameInput.value })
            );
            this.#createCard.addEventListener("click", () => this.#dispatch({ type: "createSelected" }));
            this.#leftPanel.addEventListener("b3tty-card-select", (e: Event) => {
                void this.#selectTheme((e.target as unknown as B3ttyPaletteCard).value);
            });
            this.#resetBtn.addEventListener("click", () => this.#dispatch({ type: "resetRequested" }));
            this.#closeBtn.addEventListener("click", () => void this.#requestClose());
            this.#saveBtn.addEventListener("click", () => void this.#handleSave());
            this.#render();
        }

        open(themes: ThemeEntry[], options: ThemeEditorOpenOptions = {}): void {
            this.#dispatch({ type: "opened" });
            this.setAttribute("open", "");
            void this.refresh(themes, options.select).then(() => {
                if (!options.action || !options.select || this.#state.selected !== options.select) return;
                if (options.action === "copy") void this.#copySelected();
                else void this.#deleteSelected();
            });
        }

        close(): void {
            closeOverlay(this);
        }

        refresh(themes: ThemeEntry[], select?: string): Promise<void> {
            return this.#refreshList(themes, select, false);
        }

        /**
         * Rebuilds the list from themes; with keepSelection (after a save) the selection
         * and form stay as they are, and only the cards are updated.
         */
        async #refreshList(themes: ThemeEntry[], select: string | undefined, keepSelection: boolean): Promise<void> {
            this.#dispatch({ type: "refreshStarted", entries: themes });
            const listId = this.#state.listId;
            const loaded = await fetchThemePalettes(themes);
            const palettes = Object.fromEntries(loaded.map(({ entry, palette }) => [entry.name, palette]));
            const before = this.#state;
            this.#dispatch({ type: "refreshLoaded", listId, palettes, select, keepSelection });
            const state = this.#state;
            if (state === before || state.selected === null) return;
            // A theme just saved for the first time gets a card; bring it into view.
            if (keepSelection) {
                this.#cardFor(state.selected)?.scrollIntoView({ block: "nearest" });
                return;
            }
            if (state.phase !== "loading") return;
            void this.#loadTheme(state.selected);
            this.#cardFor(state.selected)?.scrollIntoView({ block: "nearest" });
        }

        #dispatch(action: ThemeEditorAction): void {
            this.#state = reduceThemeEditor(this.#state, action);
            this.#render();
        }

        /** Brings the DOM in line with #state. Safe to call at any time. */
        #render(): void {
            const state = this.#state;
            const { form } = state;
            const saving = state.phase === "saving";
            const readOnly = isReadOnly(state);
            const editable = isEditable(state);

            this.#renderList();
            this.#createRadio.checked = state.selected === null;
            this.#createCard.toggleAttribute("selected", state.selected === null);
            this.#createBadge.hidden = !newThemeHasUnsavedChanges(state);
            for (const card of this.#cards()) {
                const name = (card as unknown as B3ttyPaletteCard).value;
                // The selected card shows the live preview, a card with a draft that
                // draft, and every other card its own palette.
                const palette = cardPalette(state, name) ?? state.palettes[name];
                const unsaved = themeHasUnsavedChanges(state, name);
                const key = palette ? JSON.stringify([palette, unsaved]) : "";
                if (palette && this.#cardPalettes.get(card) !== key) {
                    const entry = themeEntry(state, name);
                    const details = entry ? cardDetails(entry) : undefined;
                    (card as unknown as B3ttyPaletteCard).setup(
                        name,
                        formatThemeName(name),
                        palette,
                        unsaved ? { ...details, badge: UNSAVED_BADGE, badgeTone: "accent" } : details
                    );
                    this.#cardPalettes.set(card, key);
                }
                card.toggleAttribute("selected", name === state.selected);
            }

            // Values are set only when they differ, so typing isn't disturbed.
            if (this.#nameInput.value !== form.name) this.#nameInput.value = form.name;
            this.#nameInput.readOnly = readOnly || saving;
            for (const [key, input] of this.#colorInputs) {
                const value = form.colors[key];
                if (input.value !== value) input.value = value;
                input.disabled = readOnly || saving;
                const swatch = this.#swatches.get(key);
                if (!swatch) continue;
                const valid = value !== "" && isValidThemeColor(value);
                swatch.classList.toggle("visible", valid);
                if (valid) swatch.style.backgroundColor = value;
            }
            if (this.#bgImageInput.value !== form.backgroundImage) this.#bgImageInput.value = form.backgroundImage;
            if (this.#bgTransparencyInput.value !== form.transparency) {
                this.#bgTransparencyInput.value = form.transparency;
            }
            this.#bgImageInput.disabled = readOnly || saving;
            this.#bgTransparencyInput.disabled = readOnly || saving;

            this.#nameError.classList.toggle("visible", editable && nameIsTaken(state));
            this.#bgTransparencyError.classList.toggle(
                "visible",
                editable && !isValidBackgroundImageTransparency(form.transparency)
            );
            const source = selectedSource(state);
            const note = source ? READ_ONLY_NOTES[source] : undefined;
            this.#readOnlyNote.textContent = note ?? "";
            this.#readOnlyNote.hidden = !note;
            const status = state.error ?? (state.notice ? `✓ ${state.notice}` : null);
            this.#saveError.textContent = status ?? "";
            this.#saveError.classList.toggle("visible", status !== null);
            this.#saveError.classList.toggle("notice", state.error === null && state.notice !== null);

            this.#copyBtn.disabled = !canCopyTheme(state);
            this.#deleteBtn.disabled = !canDeleteTheme(state);
            this.#exportBtn.disabled = !canExportTheme(state);
            // Close too, so the editor can't close and reopen under a request in flight.
            this.#closeBtn.disabled = saving;
            this.#resetBtn.disabled = !canResetTheme(state);
            this.#saveBtn.disabled = !canSaveTheme(state);
        }

        /**
         * Rebuilds the theme list when the loaded palettes change: Your themes,
         * Installed, and Built-in, each left out when empty.
         */
        #renderList(): void {
            const { entries, palettes } = this.#state;
            if (palettes === this.#renderedPalettes) return;
            this.#renderedPalettes = palettes;
            this.#themeList.replaceChildren();
            for (const { source, title } of THEME_SECTIONS) {
                const inSection = entries.filter((e) => e.source === source && palettes[e.name]);
                if (inSection.length === 0) continue;
                const section = document.createElement("div");
                section.className = "list-section";
                const body = document.createElement("div");
                body.className = "list-section";
                for (const entry of inSection) {
                    const palette = palettes[entry.name]!;
                    // #render then sets up any card whose palette or badge differs.
                    const card = createPaletteCard(entry, palette);
                    this.#cardPalettes.set(card, JSON.stringify([palette, false]));
                    body.appendChild(card);
                }
                const heading = document.createElement("div");
                heading.className = "section-title";
                heading.textContent = title;
                section.append(heading, body);
                this.#themeList.appendChild(section);
            }
        }

        #cards(): HTMLElement[] {
            return Array.from(this.#themeList.querySelectorAll<HTMLElement>("b3tty-palette-card"));
        }

        #cardFor(name: string): HTMLElement | undefined {
            return this.#cards().find((card) => (card as unknown as B3ttyPaletteCard).value === name);
        }

        async #selectTheme(name: string): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "themeSelected", name });
            if (this.#state !== before) await this.#loadTheme(name);
        }

        async #loadTheme(name: string): Promise<void> {
            try {
                const config = await getThemeConfig(name);
                this.#dispatch({ type: "themeLoaded", name, config });
            } catch (err) {
                const message = err instanceof Error ? err.message : String(err);
                this.#dispatch({ type: "themeLoadFailed", name, message });
            }
        }

        /** Tells terminal.ts that themes were added or removed, with the updated lists. */
        #themesChanged(lists: ThemeLists): void {
            this.dispatchEvent(
                new CustomEvent("b3tty-themes-changed", { detail: lists, bubbles: true, composed: true })
            );
        }

        async #copySelected(): Promise<void> {
            const name = this.#state.selected;
            if (name === null || !canCopyTheme(this.#state)) return;
            const bold = document.createElement("b");
            bold.textContent = name;
            await this.#prompt.ask({
                title: "Copy Theme",
                message: ["Save a copy of ", bold, " as a theme of your own, which you can edit and export."],
                input: {
                    label: "New theme name",
                    value: copyName(this.#state.entries, name),
                    validate: (value) => copyNameProblem(this.#state.entries, value),
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
            const entry = themeEntry(this.#state, this.#state.selected);
            if (!entry || !canDeleteTheme(this.#state)) return;
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

        /** Closes, first asking to discard unsaved changes when there are any. */
        async #requestClose(): Promise<void> {
            const state = this.#state;
            if (state.phase === "saving") return;
            if (!hasAnyUnsavedChanges(state)) {
                this.close();
                return;
            }
            const { names, newTheme } = unsavedChanges(state);
            const discard = await askToDiscardChanges(
                this.#prompt,
                names,
                newTheme ? "a new theme" : null,
                this.#closeBtn
            );
            if (discard) this.close();
        }

        async #handleSave(): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "saveSubmitted" });
            if (this.#state === before) return;
            const { form } = this.#state;
            const name = form.name.trim();
            try {
                const response = await postEditTheme(name, themeDataFromForm(form));
                this.dispatchEvent(
                    new CustomEvent("b3tty-theme-edited", {
                        detail: { name, response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.#dispatch({ type: "saveSucceeded", name });
                // The list picks up a new theme and the saved colors on its card; the
                // editor stays on the saved theme.
                if (response.themes) void this.#refreshList(response.themes, undefined, true);
            } catch (err) {
                // Keep editor open so the user can retry, but tell them why —
                // a save that silently does nothing looks indistinguishable
                // from a UI bug.
                const message = err instanceof Error ? err.message : "Failed to save theme.";
                this.#dispatch({ type: "saveFailed", message });
            }
        }
    }

    customElements.define("b3tty-theme-editor", B3ttyThemeEditorImpl);
}
