/**
 * State for <b3tty-settings-editor> (settings-editor.ts), kept free of DOM access so it
 * can be tested under bun test. The component dispatches an action for each user event
 * and server response, and renders whatever state comes back.
 */
import type { SettingsConfig, SettingsServerConfig } from "../types.ts";

export type SettingsTab = "server" | "terminal";

/** The form's values as the inputs hold them: number fields as typed text. */
export interface SettingsForm {
    port: string;
    noAuth: boolean;
    noBrowser: boolean;
    showMenubar: SettingsServerConfig["showMenubar"];
    fontFamily: string;
    fontSize: string;
    ligatures: boolean;
    autoResize: boolean;
    rows: string;
    columns: string;
}

export interface SettingsEditorState {
    tab: SettingsTab;
    /** The settings as opened, to tell which fields changed. */
    original: SettingsForm;
    form: SettingsForm;
    phase: "editing" | "saving";
    /** A failed save, shown in the footer. */
    error: string | null;
}

/** One field's change, typed so the value matches the field. */
export type SettingsFieldChange = {
    [K in keyof SettingsForm]: { type: "fieldChanged"; field: K; value: SettingsForm[K] };
}[keyof SettingsForm];

export type SettingsEditorAction =
    | { type: "opened"; settings: SettingsConfig }
    | { type: "tabSelected"; tab: SettingsTab }
    | SettingsFieldChange
    | { type: "submitted" }
    | { type: "failed"; message: string };

/** The form for a set of settings. */
export function formFromSettings(settings: SettingsConfig): SettingsForm {
    return {
        port: String(settings.server.port),
        noAuth: settings.server.noAuth,
        noBrowser: settings.server.noBrowser,
        showMenubar: settings.server.showMenubar,
        fontFamily: settings.terminal.fontFamily,
        fontSize: String(settings.terminal.fontSize),
        ligatures: settings.terminal.ligatures,
        autoResize: settings.terminal.autoResize,
        rows: String(settings.terminal.rows),
        columns: String(settings.terminal.columns),
    };
}

const BLANK_FORM: SettingsForm = formFromSettings({
    server: { port: 0, noAuth: false, noBrowser: false, showMenubar: "hover" },
    terminal: { fontFamily: "", fontSize: 0, autoResize: false, rows: 0, columns: 0, ligatures: false },
});

export const initialSettingsEditorState: SettingsEditorState = {
    tab: "server",
    original: BLANK_FORM,
    form: BLANK_FORM,
    phase: "editing",
    error: null,
};

/** A whole number typed into a field, or NaN for anything else (including blanks and decimals). */
function wholeNumber(text: string): number {
    return /^\s*\d+\s*$/.test(text) ? Number(text) : NaN;
}

/**
 * Why the form can't be saved, or null. Mirrors the server's checks on POST /settings,
 * so a bad value is caught before the request.
 */
export function settingsProblem(form: SettingsForm): string | null {
    const port = wholeNumber(form.port);
    if (!(port >= 1 && port <= 65535)) return "Port must be a whole number from 1 to 65535.";
    if (!(wholeNumber(form.fontSize) >= 1)) return "Font size must be a whole number of at least 1.";
    if (!(wholeNumber(form.rows) >= 10)) return "Rows must be a whole number of at least 10.";
    if (!(wholeNumber(form.columns) >= 10)) return "Columns must be a whole number of at least 10.";
    return null;
}

/** Whether a server setting changed; those only take effect after a restart. */
export function serverSettingsChanged(state: SettingsEditorState): boolean {
    const { form, original } = state;
    return (
        form.port !== original.port ||
        form.noAuth !== original.noAuth ||
        form.noBrowser !== original.noBrowser ||
        form.showMenubar !== original.showMenubar
    );
}

/**
 * Whether Auto-Resize changed, which needs a restart. Font and ligature changes apply
 * immediately, and so do rows and columns while auto-resize stays off.
 */
export function autoResizeChanged(state: SettingsEditorState): boolean {
    return state.form.autoResize !== state.original.autoResize;
}

/** Whether any field differs from the settings as opened. */
export function settingsDirty(state: SettingsEditorState): boolean {
    const { form, original } = state;
    return (Object.keys(form) as (keyof SettingsForm)[]).some((key) => form[key] !== original[key]);
}

/** Whether OK can be clicked: something changed, every value is valid, and no save is in flight. */
export function canSaveSettings(state: SettingsEditorState): boolean {
    return state.phase === "editing" && settingsDirty(state) && settingsProblem(state.form) === null;
}

/** The settings to send on save. Call only for a form with no settingsProblem. */
export function settingsFromForm(form: SettingsForm): SettingsConfig {
    return {
        server: {
            port: wholeNumber(form.port),
            noAuth: form.noAuth,
            noBrowser: form.noBrowser,
            showMenubar: form.showMenubar,
        },
        terminal: {
            fontFamily: form.fontFamily.trim(),
            fontSize: wholeNumber(form.fontSize),
            autoResize: form.autoResize,
            rows: wholeNumber(form.rows),
            columns: wholeNumber(form.columns),
            ligatures: form.ligatures,
        },
    };
}

/**
 * Returns the next state. An action that doesn't apply in the current state returns
 * that same object, so a caller can tell whether "submitted" started a save.
 */
export function reduceSettingsEditor(state: SettingsEditorState, action: SettingsEditorAction): SettingsEditorState {
    switch (action.type) {
        case "opened": {
            const form = formFromSettings(action.settings);
            return { tab: "server", original: form, form, phase: "editing", error: null };
        }
        case "tabSelected":
            return action.tab === state.tab ? state : { ...state, tab: action.tab };
        case "fieldChanged":
            if (state.phase !== "editing") return state;
            return { ...state, form: { ...state.form, [action.field]: action.value }, error: null };
        case "submitted":
            if (!canSaveSettings(state)) return state;
            return { ...state, phase: "saving", error: null };
        case "failed":
            if (state.phase !== "saving") return state;
            return { ...state, phase: "editing", error: action.message };
    }
}
