/**
 * State for <b3tty-theme-import-dialog> (theme-import-dialog.ts), kept free of DOM
 * access so it can be tested under bun test. The component dispatches an action for
 * each user event and server response, and renders whatever state comes back.
 *
 * The dialog is either editing the path or waiting on an import request. `upgrade`
 * holds the server's description of an upgrade once it said the package replaces an
 * installed theme; while it is set, the action button is Replace and the next import
 * sends replace: true.
 */
export type ThemeImportState =
    | { phase: "editing"; path: string; upgrade: string | null; error: string | null }
    | { phase: "importing"; path: string; upgrade: string | null };

export type ThemeImportAction =
    | { type: "opened" }
    | { type: "pathChanged"; path: string }
    | { type: "submitted" }
    | { type: "upgradeRequired"; message: string }
    | { type: "failed"; message: string };

export const initialThemeImportState: ThemeImportState = {
    phase: "editing",
    path: "",
    upgrade: null,
    error: null,
};

/** Whether Import (or Replace) can be clicked: not already importing, and a path is entered. */
export function canSubmitThemeImport(state: ThemeImportState): boolean {
    return state.phase === "editing" && state.path.trim() !== "";
}

/**
 * Returns the next state. An action that doesn't apply in the current state returns
 * that same object, so a caller can tell whether, e.g., "submitted" started an import.
 */
export function reduceThemeImport(state: ThemeImportState, action: ThemeImportAction): ThemeImportState {
    switch (action.type) {
        case "opened":
            return initialThemeImportState;
        case "pathChanged":
            if (state.phase !== "editing") return state;
            // A different path is a different package, so any pending Replace is undone.
            return { phase: "editing", path: action.path, upgrade: null, error: null };
        case "submitted":
            if (!canSubmitThemeImport(state)) return state;
            return { phase: "importing", path: state.path, upgrade: state.upgrade };
        case "upgradeRequired":
            // Responses only count while their import is still the one in flight.
            if (state.phase !== "importing") return state;
            return { phase: "editing", path: state.path, upgrade: action.message, error: null };
        case "failed":
            if (state.phase !== "importing") return state;
            return { phase: "editing", path: state.path, upgrade: state.upgrade, error: action.message };
    }
}
