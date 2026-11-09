/**
 * State for <b3tty-theme-export-dialog> (theme-export-dialog.ts), kept free of DOM
 * access so it can be tested under bun test. The component dispatches an action for
 * each user event and server response, and renders whatever state comes back.
 */
import type { ThemeMetadata, ThemeSource } from "../types.ts";
import { isValidRepoUrl } from "../validators.ts";

/** Why there is nothing to export: no active theme, or one only user themes can be. */
export type ThemeExportBlock = "noTheme" | "builtin" | "installed";

export type ThemeExportState = {
    /** Bumped by every open, so saved details fetched for an earlier open are ignored. */
    openId: number;
} & (
    | { phase: "unavailable"; reason: ThemeExportBlock }
    | {
          phase: "editing" | "exporting";
          themeName: string;
          /** The theme editor's inputs differ from the saved theme, which is what's exported. */
          unsavedChanges: boolean;
          path: string;
          /** The detail fields exactly as typed. */
          fields: ThemeMetadata;
          /** The theme version of the last export, or "" for a theme never exported. */
          lastVersion: string;
          /** A failed export, shown in the actions row. */
          error: string | null;
      }
);

export type ThemeExportAction =
    | { type: "opened"; themeName: string; source: ThemeSource; unsavedChanges: boolean }
    | { type: "pathChanged"; path: string }
    | { type: "fieldChanged"; field: keyof ThemeMetadata; value: string }
    | { type: "prefillLoaded"; openId: number; saved: Partial<ThemeMetadata> }
    | { type: "submitted" }
    | { type: "failed"; message: string };

const EMPTY_FIELDS: ThemeMetadata = { repo: "", author: "", description: "", themeVersion: "" };

export const initialThemeExportState: ThemeExportState = { openId: 0, phase: "unavailable", reason: "noTheme" };

/** The details to send: every field trimmed. */
export function themeExportMetadata(fields: ThemeMetadata): ThemeMetadata {
    return {
        repo: fields.repo.trim(),
        author: fields.author.trim(),
        description: fields.description.trim(),
        themeVersion: fields.themeVersion.trim(),
    };
}

/** Whether to show the repository error: something is typed, and it isn't an https URL. */
export function showRepoError(state: ThemeExportState): boolean {
    if (state.phase === "unavailable") return false;
    const repo = state.fields.repo.trim();
    return repo !== "" && !isValidRepoUrl(repo);
}

/** Whether the typed version is the last export's, so installs wouldn't see an upgrade. */
export function sameAsLastVersion(state: ThemeExportState): boolean {
    return (
        state.phase !== "unavailable" &&
        state.lastVersion !== "" &&
        state.fields.themeVersion.trim() === state.lastVersion
    );
}

/** Whether Export can be clicked: a path, every detail filled in, an https repository, and no export in flight. */
export function canExportTheme(state: ThemeExportState): boolean {
    if (state.phase !== "editing" || state.path.trim() === "") return false;
    const meta = themeExportMetadata(state.fields);
    return !!meta.author && !!meta.description && !!meta.themeVersion && isValidRepoUrl(meta.repo);
}

/**
 * Returns the next state. An action that doesn't apply in the current state returns
 * that same object, so a caller can tell whether "submitted" started an export.
 */
export function reduceThemeExport(state: ThemeExportState, action: ThemeExportAction): ThemeExportState {
    switch (action.type) {
        case "opened": {
            const openId = state.openId + 1;
            if (!action.themeName) return { openId, phase: "unavailable", reason: "noTheme" };
            if (action.source !== "user") return { openId, phase: "unavailable", reason: action.source };
            return {
                openId,
                phase: "editing",
                themeName: action.themeName,
                unsavedChanges: action.unsavedChanges,
                path: `~/${action.themeName}.b3theme`,
                fields: EMPTY_FIELDS,
                lastVersion: "",
                error: null,
            };
        }
        case "pathChanged":
            if (state.phase !== "editing") return state;
            return { ...state, path: action.path, error: null };
        case "fieldChanged":
            if (state.phase !== "editing") return state;
            return { ...state, fields: { ...state.fields, [action.field]: action.value }, error: null };
        case "prefillLoaded": {
            if (state.phase !== "editing" || action.openId !== state.openId) return state;
            // Only fields still empty are filled, so nothing already typed is replaced.
            const { saved } = action;
            const fill = (current: string, value: string | undefined): string => current || value || "";
            return {
                ...state,
                fields: {
                    repo: fill(state.fields.repo, saved.repo),
                    author: fill(state.fields.author, saved.author),
                    description: fill(state.fields.description, saved.description),
                    themeVersion: fill(state.fields.themeVersion, saved.themeVersion),
                },
                lastVersion: saved.themeVersion ?? "",
            };
        }
        case "submitted":
            if (!canExportTheme(state) || state.phase !== "editing") return state;
            return { ...state, phase: "exporting", error: null };
        case "failed":
            if (state.phase !== "exporting") return state;
            return { ...state, phase: "editing", error: action.message };
    }
}
