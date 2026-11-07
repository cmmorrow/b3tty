/**
 * State for <b3tty-profile-editor> (profile-editor.ts), kept free of DOM access so it
 * can be tested under bun test. The component dispatches an action for each user event
 * and server response, and renders whatever state comes back.
 */
import type { ProfileConfig } from "../types.ts";

/** The form's text, exactly as typed; `commands` is the textarea's value, one per line. */
export interface ProfileForm {
    name: string;
    shell: string;
    title: string;
    workingDirectory: string;
    root: string;
    commands: string;
}

/**
 * - editing: the form can be edited and saved.
 * - loading: an existing profile was selected and its settings are being fetched.
 * - loadFailed: those settings couldn't be fetched, so the form holds only the name and
 *   saving is blocked (it would overwrite the profile with blanks).
 * - saving / deleting: a request is in flight.
 */
export type ProfileEditorPhase = "editing" | "loading" | "loadFailed" | "saving" | "deleting";

export interface ProfileEditorState {
    /** The non-default profiles, in the order given to open(). */
    profileNames: string[];
    /** The selected profile, or null for "Create new profile". */
    selected: string | null;
    phase: ProfileEditorPhase;
    form: ProfileForm;
    /** A failed load, save, or delete, shown in the actions bar. */
    error: string | null;
}

export type ProfileEditorAction =
    | { type: "opened"; profileNames: string[] }
    | { type: "createSelected" }
    | { type: "profileSelected"; name: string }
    | { type: "profileLoaded"; name: string; profile: ProfileConfig }
    | { type: "profileLoadFailed"; name: string; message: string }
    | { type: "fieldChanged"; field: keyof ProfileForm; value: string }
    | { type: "saveSubmitted" }
    | { type: "deleteSubmitted" }
    | { type: "requestFailed"; message: string };

const EMPTY_FORM: ProfileForm = { name: "", shell: "", title: "", workingDirectory: "", root: "", commands: "" };

export const initialProfileEditorState: ProfileEditorState = {
    profileNames: [],
    selected: null,
    phase: "editing",
    form: EMPTY_FORM,
    error: null,
};

/** True when the name is the reserved default profile's, which can't be created. */
export function isReservedProfileName(name: string): boolean {
    return name.trim() === "default";
}

/** Whether OK can be clicked: the form is loaded and editable, and the name is usable. */
export function canSaveProfile(state: ProfileEditorState): boolean {
    const name = state.form.name.trim();
    return state.phase === "editing" && name !== "" && !isReservedProfileName(name);
}

/** Whether Delete can be clicked: an existing profile is selected and loaded. */
export function canDeleteProfile(state: ProfileEditorState): boolean {
    return state.phase === "editing" && state.selected !== null;
}

/** Whether a request is in flight, during which the editor can't be changed or closed. */
export function isProfileEditorBusy(state: ProfileEditorState): boolean {
    return state.phase === "saving" || state.phase === "deleting";
}

/** The profile to send on save: fields trimmed, blank command lines dropped. */
export function profileFromForm(form: ProfileForm): ProfileConfig {
    return {
        shell: form.shell.trim(),
        title: form.title.trim(),
        workingDirectory: form.workingDirectory.trim(),
        root: form.root.trim(),
        commands: form.commands.split("\n").filter((line) => line !== ""),
    };
}

/**
 * Returns the next state. An action that doesn't apply in the current state returns
 * that same object, so a caller can tell whether, e.g., "saveSubmitted" started a save.
 */
export function reduceProfileEditor(state: ProfileEditorState, action: ProfileEditorAction): ProfileEditorState {
    switch (action.type) {
        case "opened":
            return {
                ...initialProfileEditorState,
                profileNames: action.profileNames.filter((name) => name !== "default"),
            };
        case "createSelected":
            if (isProfileEditorBusy(state)) return state;
            return { ...state, selected: null, phase: "editing", form: EMPTY_FORM, error: null };
        case "profileSelected":
            if (isProfileEditorBusy(state) || !state.profileNames.includes(action.name)) return state;
            // The form is cleared to just the name, so nothing from the previous
            // selection can be saved under this one.
            return {
                ...state,
                selected: action.name,
                phase: "loading",
                form: { ...EMPTY_FORM, name: action.name },
                error: null,
            };
        case "profileLoaded": {
            // Only the selection still waiting on this load counts.
            if (state.phase !== "loading" || state.selected !== action.name) return state;
            const p = action.profile;
            return {
                ...state,
                phase: "editing",
                form: {
                    name: action.name,
                    shell: p.shell ?? "",
                    title: p.title ?? "",
                    workingDirectory: p.workingDirectory ?? "",
                    root: p.root ?? "",
                    commands: (p.commands ?? []).join("\n"),
                },
            };
        }
        case "profileLoadFailed":
            if (state.phase !== "loading" || state.selected !== action.name) return state;
            return { ...state, phase: "loadFailed", error: `Couldn't load ${action.name}: ${action.message}` };
        case "fieldChanged":
            if (state.phase !== "editing") return state;
            // An existing profile's name is fixed.
            if (action.field === "name" && state.selected !== null) return state;
            return { ...state, form: { ...state.form, [action.field]: action.value }, error: null };
        case "saveSubmitted":
            if (!canSaveProfile(state)) return state;
            return { ...state, phase: "saving", error: null };
        case "deleteSubmitted":
            if (!canDeleteProfile(state)) return state;
            return { ...state, phase: "deleting", error: null };
        case "requestFailed":
            if (!isProfileEditorBusy(state)) return state;
            return { ...state, phase: "editing", error: action.message };
    }
}
