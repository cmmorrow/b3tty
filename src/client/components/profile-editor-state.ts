/**
 * State for <b3tty-profile-editor> (profile-editor.ts), kept free of DOM access so it
 * can be tested under bun test. The component dispatches an action for each user event
 * and server response, and renders whatever state comes back. The Copy… / Delete… and
 * Close prompts have their own state, in prompt-layer-state.ts.
 *
 * Like the theme editor, unsaved edits survive selecting another card: leaving a
 * profile (or "Create new profile") with unsaved changes keeps its form as a draft, and
 * selecting it again restores the draft instead of reloading. Drafts live only in this
 * state, so they last until the editor is closed or the page is reloaded.
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

/** A profile's unsaved form, kept while another card is selected. */
export interface ProfileDraft {
    form: ProfileForm;
    /** The profile as loaded, to compare against. */
    saved: ProfileForm;
}

/**
 * - editing: the form can be edited and saved.
 * - loading: an existing profile was selected and its settings are being fetched.
 * - loadFailed: those settings couldn't be fetched, so the form holds only the name and
 *   saving is blocked (it would overwrite the profile with blanks).
 * - saving: an edit-profile request is in flight.
 */
export type ProfileEditorPhase = "editing" | "loading" | "loadFailed" | "saving";

export interface ProfileEditorState {
    /** The non-default profiles, in the order given to open() or by the server. */
    profileNames: string[];
    /** The selected profile, or null for "Create new profile". */
    selected: string | null;
    phase: ProfileEditorPhase;
    form: ProfileForm;
    /** The selected profile's form as loaded (or last saved), to tell whether it has unsaved changes. */
    saved: ProfileForm | null;
    /** A failed load or save, shown in the actions bar. */
    error: string | null;
    /** A success message ("Saved"), shown in the actions bar until the next change. */
    notice: string | null;
    /** Unsaved forms of profiles other than the selected one, by name. */
    drafts: Record<string, ProfileDraft>;
    /** The unsaved "Create new profile" form, while another card is selected. */
    newDraft: ProfileForm | null;
}

export type ProfileEditorAction =
    | { type: "opened"; profileNames: string[] }
    | { type: "createSelected" }
    | { type: "profileSelected"; name: string }
    | { type: "profileLoaded"; name: string; profile: ProfileConfig }
    | { type: "profileLoadFailed"; name: string; message: string }
    | { type: "fieldChanged"; field: keyof ProfileForm; value: string }
    | { type: "resetRequested" }
    | { type: "saveSubmitted" }
    | { type: "saveSucceeded"; name: string; profileNames: string[] }
    | { type: "saveFailed"; message: string }
    /** Profiles were added or removed (a copy or delete); selects `select`, else "Create new profile". */
    | { type: "profilesChanged"; profileNames: string[]; select?: string };

function emptyForm(name = ""): ProfileForm {
    return { name, shell: "", title: "", workingDirectory: "", root: "", commands: "" };
}

export const initialProfileEditorState: ProfileEditorState = {
    profileNames: [],
    selected: null,
    phase: "editing",
    form: emptyForm(),
    saved: null,
    error: null,
    notice: null,
    drafts: {},
    newDraft: null,
};

/** The profile names the editor lists: every one but "default". */
function editableNames(names: string[]): string[] {
    return names.filter((name) => name !== "default");
}

/** The form for a profile as GET /profile-config returns it. */
export function formFromProfile(name: string, p: ProfileConfig): ProfileForm {
    return {
        name,
        shell: p.shell ?? "",
        title: p.title ?? "",
        workingDirectory: p.workingDirectory ?? "",
        root: p.root ?? "",
        commands: (p.commands ?? []).join("\n"),
    };
}

/** True when the name is the reserved default profile's, which can't be created. */
export function isReservedProfileName(name: string): boolean {
    return name.trim() === "default";
}

/** Whether the form can be changed: loaded and not being saved. */
export function isEditable(state: ProfileEditorState): boolean {
    return state.phase === "editing";
}

/** Whether Save can be clicked: the form is loaded and editable, and the name is usable. */
export function canSaveProfile(state: ProfileEditorState): boolean {
    const name = state.form.name.trim();
    return isEditable(state) && name !== "" && !isReservedProfileName(name);
}

/** Whether Copy… can be clicked: a profile is selected. It copies the saved profile. */
export function canCopyProfile(state: ProfileEditorState): boolean {
    return state.phase !== "saving" && state.selected !== null;
}

/** Whether Delete… can be clicked: a profile is selected. */
export function canDeleteProfile(state: ProfileEditorState): boolean {
    return state.phase !== "saving" && state.selected !== null;
}

/** Whether a request is in flight, during which the editor can't be changed or closed. */
export function isProfileEditorBusy(state: ProfileEditorState): boolean {
    return state.phase === "saving";
}

/** Whether a form has anything typed in it. */
function formIsEmpty(form: ProfileForm): boolean {
    return Object.values(form).every((value) => value.trim() === "");
}

function formsEqual(a: ProfileForm, b: ProfileForm): boolean {
    return (Object.keys(a) as Array<keyof ProfileForm>).every((key) => a[key] === b[key]);
}

/**
 * Whether the form holds edits that Save hasn't saved: for "Create new profile",
 * anything typed; for a profile, any field that differs from the saved one. A profile
 * that is still loading or failed to load never has any.
 */
export function formIsDirty(state: ProfileEditorState): boolean {
    if (state.phase === "loading" || state.phase === "loadFailed") return false;
    if (state.selected === null) return !formIsEmpty(state.form);
    return !!state.saved && !formsEqual(state.form, state.saved);
}

/** Whether Reset can be clicked: the form has unsaved changes to throw away. */
export function canResetProfile(state: ProfileEditorState): boolean {
    return isEditable(state) && formIsDirty(state);
}

/** Whether a profile's card should be badged as having unsaved changes. */
export function profileHasUnsavedChanges(state: ProfileEditorState, name: string): boolean {
    return name === state.selected ? formIsDirty(state) : name in state.drafts;
}

/** Whether the "Create new profile" card should be badged as having unsaved changes. */
export function newProfileHasUnsavedChanges(state: ProfileEditorState): boolean {
    return state.selected === null ? formIsDirty(state) : state.newDraft !== null;
}

/**
 * Everything Close would discard: the profiles with unsaved changes, sorted, and
 * whether "Create new profile" has any.
 */
export function unsavedChanges(state: ProfileEditorState): { names: string[]; newProfile: boolean } {
    const names = Object.keys(state.drafts);
    if (state.selected !== null && formIsDirty(state)) names.push(state.selected);
    return { names: names.sort(), newProfile: newProfileHasUnsavedChanges(state) };
}

/** Whether Close should ask before discarding unsaved changes. */
export function hasAnyUnsavedChanges(state: ProfileEditorState): boolean {
    const { names, newProfile } = unsavedChanges(state);
    return newProfile || names.length > 0;
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

/** A name for a copy of name that no profile has yet: name-copy, name-copy-2, … */
export function copyProfileName(profileNames: string[], name: string): string {
    const taken = new Set(profileNames);
    let candidate = `${name}-copy`;
    for (let i = 2; taken.has(candidate); i++) candidate = `${name}-copy-${i}`;
    return candidate;
}

/** Why newName can't name a copy, or null. */
export function copyProfileNameProblem(profileNames: string[], newName: string): string | null {
    if (!newName.trim()) return "Enter a name.";
    if (newName.trim() !== newName) return "The name can't start or end with a space.";
    if (isReservedProfileName(newName)) return "Cannot use 'default' as a profile name.";
    if (profileNames.includes(newName)) return "A profile with this name already exists.";
    return null;
}

/** Keeps the selection's unsaved form as a draft, before another card is selected. */
function stashSelection(state: ProfileEditorState): ProfileEditorState {
    const dirty = formIsDirty(state);
    if (state.selected === null) return { ...state, newDraft: dirty ? state.form : null };
    if (!dirty || !state.saved) return state;
    return { ...state, drafts: { ...state.drafts, [state.selected]: { form: state.form, saved: state.saved } } };
}

/** Drops drafts of profiles that no longer exist, e.g. after a delete. */
function pruneDrafts(state: ProfileEditorState): ProfileEditorState {
    const kept = Object.entries(state.drafts).filter(([name]) => state.profileNames.includes(name));
    if (kept.length === Object.keys(state.drafts).length) return state;
    return { ...state, drafts: Object.fromEntries(kept) };
}

/** Selects "Create new profile": its draft when it has one, else an empty form. */
function selectCreate(state: ProfileEditorState): ProfileEditorState {
    const form = state.newDraft ?? emptyForm();
    return { ...state, selected: null, phase: "editing", form, saved: null, error: null, notice: null, newDraft: null };
}

/**
 * Selects a profile: its draft when it has one, else starts loading it, with the form
 * holding only its name until then, so nothing from the previous selection can be
 * saved under it.
 */
function selectProfile(state: ProfileEditorState, name: string): ProfileEditorState {
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
 * that same object, so a caller can tell whether, e.g., "saveSubmitted" started a save.
 */
export function reduceProfileEditor(state: ProfileEditorState, action: ProfileEditorAction): ProfileEditorState {
    switch (action.type) {
        case "opened":
            return { ...initialProfileEditorState, profileNames: editableNames(action.profileNames) };
        case "createSelected":
            // Clicking it again keeps the form, like clicking the selected card.
            if (isProfileEditorBusy(state) || state.selected === null) return state;
            return selectCreate(stashSelection(state));
        case "profileSelected":
            if (isProfileEditorBusy(state) || !state.profileNames.includes(action.name)) return state;
            // Clicking the selected card again keeps the edits in progress; it only
            // retries a load that failed.
            if (action.name === state.selected && state.phase !== "loadFailed") return state;
            return selectProfile(stashSelection(state), action.name);
        case "profileLoaded": {
            // Only the selection still waiting on this load counts.
            if (state.phase !== "loading" || state.selected !== action.name) return state;
            const form = formFromProfile(action.name, action.profile);
            return { ...state, phase: "editing", form, saved: form };
        }
        case "profileLoadFailed":
            if (state.phase !== "loading" || state.selected !== action.name) return state;
            return { ...state, phase: "loadFailed", error: `Couldn't load ${action.name}: ${action.message}` };
        case "fieldChanged":
            if (!isEditable(state)) return state;
            // An existing profile's name is fixed.
            if (action.field === "name" && state.selected !== null) return state;
            return { ...state, form: { ...state.form, [action.field]: action.value }, error: null, notice: null };
        case "resetRequested": {
            if (!canResetProfile(state)) return state;
            const form = state.selected === null || !state.saved ? emptyForm() : state.saved;
            return { ...state, form, error: null, notice: null };
        }
        case "saveSubmitted":
            if (!canSaveProfile(state)) return state;
            return { ...state, phase: "saving", error: null, notice: null };
        case "saveSucceeded": {
            if (state.phase !== "saving") return state;
            // The form becomes what was saved (trimmed, blank command lines dropped), and
            // the profile is selected under that name; a draft it had was just overwritten.
            const form = formFromProfile(action.name, profileFromForm(state.form));
            const drafts = { ...state.drafts };
            delete drafts[action.name];
            return {
                ...state,
                profileNames: editableNames(action.profileNames),
                phase: "editing",
                selected: action.name,
                form,
                saved: form,
                drafts,
                notice: "Saved",
            };
        }
        case "saveFailed":
            if (state.phase !== "saving") return state;
            return { ...state, phase: "editing", error: action.message };
        case "profilesChanged": {
            if (isProfileEditorBusy(state)) return state;
            const next = pruneDrafts(stashSelection({ ...state, profileNames: editableNames(action.profileNames) }));
            // A selection that still exists is reselected from its draft (just stashed).
            return action.select !== undefined && next.profileNames.includes(action.select)
                ? selectProfile(next, action.select)
                : selectCreate(next);
        }
    }
}
