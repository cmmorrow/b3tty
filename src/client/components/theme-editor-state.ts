/**
 * State for <b3tty-theme-editor> (theme-editor.ts), kept free of DOM access so it can
 * be tested under bun test. The component dispatches an action for each user event and
 * server response, and renders whatever state comes back. The Copy… / Delete… prompt
 * has its own state, in theme-editor-prompt-state.ts.
 *
 * Unsaved edits survive selecting another card: leaving a user theme (or "Create new
 * theme") with unsaved changes keeps its form as a draft, and selecting it again
 * restores the draft instead of reloading. Drafts live only in this state, so they
 * last until the editor is closed or the page is reloaded.
 */
import type { Palette, ThemeEntry, ThemeSource } from "../types.ts";
import { isValidBackgroundImageTransparency, isValidThemeColor } from "../validators.ts";

/** Every color the editor edits, by its Theme property. */
export const THEME_COLOR_KEYS = [
    "background",
    "foreground",
    "cursor",
    "cursorAccent",
    "selectionBackground",
    "selectionForeground",
    "black",
    "red",
    "yellow",
    "green",
    "cyan",
    "blue",
    "magenta",
    "white",
    "brightBlack",
    "brightRed",
    "brightYellow",
    "brightGreen",
    "brightCyan",
    "brightBlue",
    "brightMagenta",
    "brightWhite",
] as const;

export type ThemeColorKey = (typeof THEME_COLOR_KEYS)[number];

/** The ANSI colors in a palette's display order (see GET /theme). */
const NORMAL_KEYS: ThemeColorKey[] = ["black", "red", "yellow", "green", "cyan", "blue", "magenta", "white"];
const BRIGHT_KEYS: ThemeColorKey[] = [
    "brightBlack",
    "brightRed",
    "brightYellow",
    "brightGreen",
    "brightCyan",
    "brightBlue",
    "brightMagenta",
    "brightWhite",
];

/** A theme as GET /theme-config returns it; only these fields are read. */
export type LoadedTheme = Partial<Record<ThemeColorKey, string>> & {
    backgroundImage?: string;
    backgroundImageTransparency?: number;
};

/** The form's text exactly as typed. */
export interface ThemeForm {
    name: string;
    colors: Record<ThemeColorKey, string>;
    backgroundImage: string;
    /** The transparency field's text; empty means the default. */
    transparency: string;
}

/** A user theme's unsaved form, kept while another card is selected. */
export interface ThemeDraft {
    form: ThemeForm;
    /** The theme as loaded, to compare against. */
    saved: ThemeForm;
}

/** A field of the form: a color key, or one of the other inputs. */
export type ThemeFormField = ThemeColorKey | "name" | "backgroundImage" | "transparency";

/**
 * - editing: the form can be edited (unless the selected theme is read-only) and saved.
 * - loading: a theme was selected and its colors are being fetched.
 * - loadFailed: they couldn't be fetched, so the form is empty and saving is blocked
 *   (it would overwrite the theme with no colors).
 * - saving: an edit-theme request is in flight.
 */
export type ThemeEditorPhase = "editing" | "loading" | "loadFailed" | "saving";

export interface ThemeEditorState {
    /** Every theme, as last passed to open() or refresh(). */
    entries: ThemeEntry[];
    /** Bumped by every refresh, so palettes fetched for an earlier one are ignored. */
    listId: number;
    /** The loaded palettes, by theme name; a theme without one has no card. */
    palettes: Record<string, Palette>;
    /** The selected theme, or null for "Create new theme". */
    selected: string | null;
    phase: ThemeEditorPhase;
    form: ThemeForm;
    /** The selected theme's form as loaded, to tell whether it has unsaved changes. */
    saved: ThemeForm | null;
    /** A failed load or save, shown in the actions bar. */
    error: string | null;
    /** A success message ("Saved"), shown in the actions bar until the next change. */
    notice: string | null;
    /** Unsaved forms of user themes other than the selected one, by name. */
    drafts: Record<string, ThemeDraft>;
    /** The unsaved "Create new theme" form, while another card is selected. */
    newDraft: ThemeForm | null;
}

export type ThemeEditorAction =
    | { type: "opened" }
    | { type: "refreshStarted"; entries: ThemeEntry[] }
    | {
          type: "refreshLoaded";
          listId: number;
          palettes: Record<string, Palette>;
          select?: string;
          /** Keeps the current selection and form, e.g. after a save; `select` is ignored. */
          keepSelection?: boolean;
      }
    | { type: "createSelected" }
    | { type: "themeSelected"; name: string }
    | { type: "themeLoaded"; name: string; config: LoadedTheme }
    | { type: "themeLoadFailed"; name: string; message: string }
    | { type: "fieldChanged"; field: ThemeFormField; value: string }
    | { type: "resetRequested" }
    | { type: "saveSubmitted" }
    | { type: "saveSucceeded"; name: string }
    | { type: "saveFailed"; message: string };

function emptyForm(name = ""): ThemeForm {
    const colors = Object.fromEntries(THEME_COLOR_KEYS.map((key) => [key, ""])) as Record<ThemeColorKey, string>;
    return { name, colors, backgroundImage: "", transparency: "" };
}

export const initialThemeEditorState: ThemeEditorState = {
    entries: [],
    listId: 0,
    palettes: {},
    selected: null,
    phase: "editing",
    form: emptyForm(),
    saved: null,
    error: null,
    notice: null,
    drafts: {},
    newDraft: null,
};

/** The form for a theme as GET /theme-config returns it. */
export function formFromThemeConfig(name: string, config: LoadedTheme): ThemeForm {
    const colors = Object.fromEntries(THEME_COLOR_KEYS.map((key) => [key, config[key] ?? ""])) as Record<
        ThemeColorKey,
        string
    >;
    return {
        name,
        colors,
        backgroundImage: config.backgroundImage ?? "",
        // Left empty when unset, so the placeholder shows the default.
        transparency: config.backgroundImageTransparency?.toString() ?? "",
    };
}

/** The entry for a theme name, if there is one. */
export function themeEntry(state: ThemeEditorState, name: string | null): ThemeEntry | undefined {
    return name === null ? undefined : state.entries.find((e) => e.name === name);
}

/** The source of the selected theme, or null for "Create new theme". */
export function selectedSource(state: ThemeEditorState): ThemeSource | null {
    return themeEntry(state, state.selected)?.source ?? null;
}

/** Whether the selected theme is built-in or installed, which can't be edited. */
export function isReadOnly(state: ThemeEditorState): boolean {
    const source = selectedSource(state);
    return source !== null && source !== "user";
}

/** Whether the typed name belongs to a built-in or installed theme, which a user theme can't use. */
export function nameIsTaken(state: ThemeEditorState): boolean {
    const entry = themeEntry(state, state.form.name.trim());
    return !!entry && entry.source !== "user";
}

/** Whether the form can be changed: loaded, editable, and not being saved. */
export function isEditable(state: ThemeEditorState): boolean {
    return state.phase === "editing" && !isReadOnly(state);
}

/** Whether OK can be clicked: an editable form with a usable name and only valid values. */
export function canSaveTheme(state: ThemeEditorState): boolean {
    if (!isEditable(state)) return false;
    const { form } = state;
    if (form.name.trim() === "" || nameIsTaken(state)) return false;
    if (!isValidBackgroundImageTransparency(form.transparency)) return false;
    return THEME_COLOR_KEYS.every((key) => !form.colors[key] || isValidThemeColor(form.colors[key]));
}

/** Whether a theme is selected that Copy… can copy. */
export function canCopyTheme(state: ThemeEditorState): boolean {
    return state.phase !== "saving" && themeEntry(state, state.selected) !== undefined;
}

/** Whether a theme is selected that Delete… can act on; a built-in only when it's in the menu. */
export function canDeleteTheme(state: ThemeEditorState): boolean {
    const entry = themeEntry(state, state.selected);
    return state.phase !== "saving" && !!entry && (entry.source !== "builtin" || entry.inMenu);
}

/** Whether Export… can be clicked: a user theme is selected. */
export function canExportTheme(state: ThemeEditorState): boolean {
    return state.phase !== "saving" && selectedSource(state) === "user";
}

/** Whether the colors or background differ from the theme as loaded; that's what Export… exports. */
export function hasUnsavedChanges(state: ThemeEditorState): boolean {
    const { saved, form } = state;
    if (!saved) return false;
    return (
        form.backgroundImage !== saved.backgroundImage ||
        form.transparency !== saved.transparency ||
        THEME_COLOR_KEYS.some((key) => form.colors[key] !== saved.colors[key])
    );
}

/** Whether a form has anything typed in it. */
function formIsEmpty(form: ThemeForm): boolean {
    return (
        form.name.trim() === "" &&
        form.backgroundImage === "" &&
        form.transparency === "" &&
        THEME_COLOR_KEYS.every((key) => form.colors[key] === "")
    );
}

/**
 * Whether the form holds edits that Save hasn't saved: for "Create new theme", anything
 * typed; for a user theme, a changed name, color, or background. A read-only theme, or
 * one that is still loading or failed to load, never has any.
 */
export function formIsDirty(state: ThemeEditorState): boolean {
    if (state.phase === "loading" || state.phase === "loadFailed" || isReadOnly(state)) return false;
    if (state.selected === null) return !formIsEmpty(state.form);
    return hasUnsavedChanges(state) || state.form.name.trim() !== state.selected;
}

/**
 * Whether Reset can be clicked: the form has unsaved changes to throw away — back to
 * the saved theme, or to an empty form for "Create new theme".
 */
export function canResetTheme(state: ThemeEditorState): boolean {
    return isEditable(state) && formIsDirty(state);
}

/** Whether a theme's card should be badged as having unsaved changes. */
export function themeHasUnsavedChanges(state: ThemeEditorState, name: string): boolean {
    return name === state.selected ? formIsDirty(state) : name in state.drafts;
}

/** Whether the "Create new theme" card should be badged as having unsaved changes. */
export function newThemeHasUnsavedChanges(state: ThemeEditorState): boolean {
    return state.selected === null ? formIsDirty(state) : state.newDraft !== null;
}

/**
 * Everything Close would discard: the user themes with unsaved changes, sorted, and
 * whether "Create new theme" has any.
 */
export function unsavedChanges(state: ThemeEditorState): { names: string[]; newTheme: boolean } {
    const names = Object.keys(state.drafts);
    if (state.selected !== null && formIsDirty(state)) names.push(state.selected);
    return { names: names.sort(), newTheme: newThemeHasUnsavedChanges(state) };
}

/** Whether Close should ask before discarding unsaved changes. */
export function hasAnyUnsavedChanges(state: ThemeEditorState): boolean {
    const { names, newTheme } = unsavedChanges(state);
    return newTheme || names.length > 0;
}

/** A palette with every valid color typed into form laid over it. */
function overlayForm(base: Palette, form: ThemeForm): Palette {
    const typed = (key: ThemeColorKey): string | null => {
        const value = form.colors[key];
        return value && isValidThemeColor(value) ? value : null;
    };
    return {
        bg: typed("background") ?? base.bg,
        fg: typed("foreground") ?? base.fg,
        selBg: typed("selectionBackground") ?? base.selBg,
        cursor: typed("cursor") ?? base.cursor,
        normal: NORMAL_KEYS.map((key, i) => typed(key) ?? base.normal[i] ?? ""),
        bright: BRIGHT_KEYS.map((key, i) => typed(key) ?? base.bright[i] ?? ""),
    };
}

/**
 * The selected card's live preview: its loaded palette with every valid typed color
 * laid over it, or null when nothing is selected or its palette isn't loaded.
 */
export function previewPalette(state: ThemeEditorState): Palette | null {
    const base = state.selected === null ? undefined : state.palettes[state.selected];
    return base ? overlayForm(base, state.form) : null;
}

/** What a theme's card shows: the live preview, its draft laid over its palette, or its palette. */
export function cardPalette(state: ThemeEditorState, name: string): Palette | null {
    if (name === state.selected) return previewPalette(state);
    const base = state.palettes[name];
    if (!base) return null;
    const draft = state.drafts[name];
    return draft ? overlayForm(base, draft.form) : base;
}

/** The theme to send on save: set colors only, the trimmed image path (empty clears it), and the transparency when set. */
export function themeDataFromForm(form: ThemeForm): Record<string, string | number> {
    const data: Record<string, string | number> = {};
    for (const key of THEME_COLOR_KEYS) {
        if (form.colors[key]) data[key] = form.colors[key];
    }
    data["backgroundImage"] = form.backgroundImage.trim();
    if (form.transparency !== "") data["backgroundImageTransparency"] = Number(form.transparency);
    return data;
}

/** A name for a copy of name that no theme has yet: name-copy, name-copy-2, … */
export function copyName(entries: ThemeEntry[], name: string): string {
    const taken = new Set(entries.map((e) => e.name));
    let candidate = `${name}-copy`;
    for (let i = 2; taken.has(candidate); i++) candidate = `${name}-copy-${i}`;
    return candidate;
}

/** Why newName can't name a copy, or null; mirrors validateThemeName on the server. */
export function copyNameProblem(entries: ThemeEntry[], newName: string): string | null {
    if (!newName) return "Enter a name.";
    if (newName.trim() !== newName) return "The name can't start or end with a space.";
    if (newName.includes("/")) return "The name can't contain a /.";
    if (newName.length > 64) return "The name can't be longer than 64 characters.";
    if (entries.some((e) => e.name === newName)) return "A theme with this name already exists.";
    return null;
}

/** Keeps the selection's unsaved form as a draft, before another card is selected. */
function stashSelection(state: ThemeEditorState): ThemeEditorState {
    const dirty = formIsDirty(state);
    if (state.selected === null) return { ...state, newDraft: dirty ? state.form : null };
    if (!dirty || !state.saved) return state;
    return { ...state, drafts: { ...state.drafts, [state.selected]: { form: state.form, saved: state.saved } } };
}

/** Drops drafts of themes that are no longer user themes with a card, e.g. after a delete. */
function pruneDrafts(state: ThemeEditorState): ThemeEditorState {
    const kept = Object.entries(state.drafts).filter(
        ([name]) => name in state.palettes && themeEntry(state, name)?.source === "user"
    );
    if (kept.length === Object.keys(state.drafts).length) return state;
    return { ...state, drafts: Object.fromEntries(kept) };
}

/** Selects "Create new theme": its draft when it has one, else an empty, editable form. */
function selectCreate(state: ThemeEditorState): ThemeEditorState {
    const form = state.newDraft ?? emptyForm();
    return { ...state, selected: null, phase: "editing", form, saved: null, error: null, notice: null, newDraft: null };
}

/**
 * Selects a theme with a card: its draft when it has one, else starts loading it, with
 * the form holding only its name until then.
 */
function selectTheme(state: ThemeEditorState, name: string): ThemeEditorState {
    const base = { ...state, selected: name, error: null, notice: null };
    const draft = state.drafts[name];
    if (draft) {
        const drafts = { ...state.drafts };
        delete drafts[name];
        return { ...base, drafts, phase: "editing", form: draft.form, saved: draft.saved };
    }
    return { ...base, phase: "loading", form: emptyForm(name), saved: null };
}

/**
 * Returns the next state. An action that doesn't apply in the current state returns
 * that same object, so a caller can tell whether, e.g., "themeSelected" started a load.
 */
export function reduceThemeEditor(state: ThemeEditorState, action: ThemeEditorAction): ThemeEditorState {
    switch (action.type) {
        case "opened":
            return { ...initialThemeEditorState, listId: state.listId };
        case "refreshStarted":
            return { ...state, entries: action.entries, listId: state.listId + 1 };
        case "refreshLoaded": {
            if (action.listId !== state.listId) return state;
            const next = { ...state, palettes: action.palettes };
            // A save in flight keeps its selection; the list still updates.
            if (state.phase === "saving" || action.keepSelection) return pruneDrafts(next);
            const stashed = pruneDrafts(stashSelection(next));
            return action.select !== undefined && action.select in action.palettes
                ? selectTheme(stashed, action.select)
                : selectCreate(stashed);
        }
        case "createSelected":
            // Clicking it again keeps the form, like clicking the selected card.
            if (state.phase === "saving" || state.selected === null) return state;
            return selectCreate(stashSelection(state));
        case "themeSelected":
            if (state.phase === "saving" || !(action.name in state.palettes)) return state;
            // Clicking the selected card again keeps the edits in progress, and doesn't
            // reload them away; it only retries a load that failed.
            if (action.name === state.selected && state.phase !== "loadFailed") return state;
            return selectTheme(stashSelection(state), action.name);
        case "themeLoaded": {
            // Only the selection still waiting on this load counts.
            if (state.phase !== "loading" || state.selected !== action.name) return state;
            const form = formFromThemeConfig(action.name, action.config);
            return { ...state, phase: "editing", form, saved: form };
        }
        case "themeLoadFailed":
            if (state.phase !== "loading" || state.selected !== action.name) return state;
            return { ...state, phase: "loadFailed", error: `Couldn't load ${action.name}: ${action.message}` };
        case "fieldChanged": {
            if (!isEditable(state)) return state;
            const { field, value } = action;
            const form =
                field === "name" || field === "backgroundImage" || field === "transparency"
                    ? { ...state.form, [field]: value }
                    : { ...state.form, colors: { ...state.form.colors, [field]: value } };
            return { ...state, form, error: null, notice: null };
        }
        case "resetRequested": {
            if (!canResetTheme(state)) return state;
            const form = state.selected === null || !state.saved ? emptyForm() : state.saved;
            return { ...state, form, error: null, notice: null };
        }
        case "saveSubmitted":
            if (!canSaveTheme(state)) return state;
            return { ...state, phase: "saving", error: null, notice: null };
        case "saveSucceeded": {
            if (state.phase !== "saving") return state;
            // The saved theme is now selected, under the name it was saved as; a draft
            // it had under that name was just overwritten.
            const form = { ...state.form, name: action.name };
            const drafts = { ...state.drafts };
            delete drafts[action.name];
            return { ...state, phase: "editing", selected: action.name, form, saved: form, drafts, notice: "Saved" };
        }
        case "saveFailed":
            if (state.phase !== "saving") return state;
            return { ...state, phase: "editing", error: action.message };
    }
}
