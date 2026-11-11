import { describe, expect, test } from "bun:test";
import {
    canCopyProfile,
    canDeleteProfile,
    canResetProfile,
    canSaveProfile,
    copyProfileName,
    copyProfileNameProblem,
    hasAnyUnsavedChanges,
    initialProfileEditorState,
    isProfileEditorBusy,
    newProfileHasUnsavedChanges,
    profileFromForm,
    profileHasUnsavedChanges,
    unsavedChanges,
    reduceProfileEditor,
    type ProfileEditorAction,
    type ProfileEditorState,
} from "./profile-editor-state.ts";
import type { ProfileConfig } from "../types.ts";

function run(...actions: ProfileEditorAction[]): ProfileEditorState {
    return actions.reduce(reduceProfileEditor, initialProfileEditorState);
}

const OPENED: ProfileEditorAction = { type: "opened", profileNames: ["default", "api", "logs"] };
const API: ProfileConfig = {
    shell: "/bin/zsh",
    title: "API",
    workingDirectory: "~/api",
    root: "/",
    commands: ["npm install", "npm start"],
};
const LOADED_API: ProfileEditorAction[] = [
    OPENED,
    { type: "profileSelected", name: "api" },
    { type: "profileLoaded", name: "api", profile: API },
];

describe("reduceProfileEditor", () => {
    test("opens on Create new profile, without the default profile", () => {
        const s = run(OPENED);
        expect(s.profileNames).toEqual(["api", "logs"]);
        expect(s.selected).toBeNull();
        expect(s.phase).toBe("editing");
        expect(s.form.name).toBe("");
        expect(canSaveProfile(s)).toBe(false);
        expect(canDeleteProfile(s)).toBe(false);
    });

    test("a new profile can be saved once it has a name", () => {
        const s = run(OPENED, { type: "fieldChanged", field: "name", value: "work" });
        expect(canSaveProfile(s)).toBe(true);
    });

    test("'default' can't be used as a name", () => {
        expect(canSaveProfile(run(OPENED, { type: "fieldChanged", field: "name", value: " default " }))).toBe(false);
    });

    test("selecting a profile loads it, with Save disabled until it arrives", () => {
        const loading = run(OPENED, { type: "profileSelected", name: "api" });
        expect(loading.phase).toBe("loading");
        expect(loading.form).toEqual({
            name: "api",
            shell: "",
            title: "",
            workingDirectory: "",
            root: "",
            commands: "",
        });
        expect(canSaveProfile(loading)).toBe(false);

        const loaded = run(...LOADED_API);
        expect(loaded.phase).toBe("editing");
        expect(loaded.form).toEqual({
            name: "api",
            shell: "/bin/zsh",
            title: "API",
            workingDirectory: "~/api",
            root: "/",
            commands: "npm install\nnpm start",
        });
        expect(canSaveProfile(loaded)).toBe(true);
        expect(canDeleteProfile(loaded)).toBe(true);
    });

    test("selecting an unknown profile does nothing", () => {
        const s = run(OPENED);
        expect(reduceProfileEditor(s, { type: "profileSelected", name: "nope" })).toBe(s);
    });

    test("going back to Create clears the form and disables Save", () => {
        const s = run(...LOADED_API, { type: "createSelected" });
        expect(s.selected).toBeNull();
        expect(s.form.name).toBe("");
        expect(s.form.shell).toBe("");
        expect(canSaveProfile(s)).toBe(false);
    });

    test("a load that finishes after another selection is ignored", () => {
        const s = run(
            OPENED,
            { type: "profileSelected", name: "api" },
            { type: "profileSelected", name: "logs" },
            { type: "profileLoaded", name: "api", profile: API }
        );
        expect(s.selected).toBe("logs");
        expect(s.phase).toBe("loading");
        expect(s.form.shell).toBe("");
    });

    test("a load that finishes after going back to Create is ignored", () => {
        const s = run(
            OPENED,
            { type: "profileSelected", name: "api" },
            { type: "createSelected" },
            { type: "profileLoaded", name: "api", profile: API }
        );
        expect(s.selected).toBeNull();
        expect(s.form.shell).toBe("");
    });

    test("a failed load shows why and can't be saved", () => {
        const s = run(
            ...LOADED_API,
            { type: "profileSelected", name: "logs" },
            {
                type: "profileLoadFailed",
                name: "logs",
                message: "404 Not Found",
            }
        );
        expect(s.phase).toBe("loadFailed");
        expect(s.error).toBe("Couldn't load logs: 404 Not Found");
        // Nothing from the previously loaded profile is left to be saved under this one.
        expect(s.form.shell).toBe("");
        expect(canSaveProfile(s)).toBe(false);
        // Copy… and Delete… act on the server's copy, so they still work.
        expect(canDeleteProfile(s)).toBe(true);
        expect(canCopyProfile(s)).toBe(true);
    });

    test("an existing profile's name can't be changed", () => {
        const s = run(...LOADED_API);
        expect(reduceProfileEditor(s, { type: "fieldChanged", field: "name", value: "other" })).toBe(s);
    });

    test("fields can't change while loading", () => {
        const loading = run(OPENED, { type: "profileSelected", name: "api" });
        expect(reduceProfileEditor(loading, { type: "fieldChanged", field: "shell", value: "x" })).toBe(loading);
    });

    test("saving blocks a second save, copying, deleting, selecting, and editing", () => {
        const saving = run(...LOADED_API, { type: "saveSubmitted" });
        expect(saving.phase).toBe("saving");
        expect(isProfileEditorBusy(saving)).toBe(true);
        expect(canCopyProfile(saving)).toBe(false);
        expect(canDeleteProfile(saving)).toBe(false);
        for (const action of [
            { type: "saveSubmitted" },
            { type: "resetRequested" },
            { type: "profilesChanged", profileNames: ["api"] },
            { type: "createSelected" },
            { type: "profileSelected", name: "logs" },
            { type: "fieldChanged", field: "shell", value: "x" },
        ] as ProfileEditorAction[]) {
            expect(reduceProfileEditor(saving, action)).toBe(saving);
        }
    });

    test("a failed save returns to editing with the reason", () => {
        const s = run(...LOADED_API, { type: "saveSubmitted" }, { type: "saveFailed", message: "disk full" });
        expect(s.phase).toBe("editing");
        expect(s.error).toBe("disk full");
        expect(canSaveProfile(s)).toBe(true);
    });

    test("editing a field clears the error", () => {
        const s = run(
            ...LOADED_API,
            { type: "saveSubmitted" },
            { type: "saveFailed", message: "disk full" },
            { type: "fieldChanged", field: "title", value: "API 2" }
        );
        expect(s.error).toBeNull();
        expect(s.form.title).toBe("API 2");
    });

    test("a failure with no save in flight is ignored", () => {
        const s = run(...LOADED_API);
        expect(reduceProfileEditor(s, { type: "saveFailed", message: "late" })).toBe(s);
    });

    test("opening again starts over", () => {
        const s = run(...LOADED_API, { type: "saveSubmitted" }, { type: "opened", profileNames: ["logs"] });
        expect(s).toEqual({ ...initialProfileEditorState, profileNames: ["logs"] });
    });
});

describe("saving", () => {
    test("a save keeps the editor on the profile, normalized, with a notice", () => {
        const s = run(
            OPENED,
            { type: "fieldChanged", field: "name", value: " work " },
            { type: "fieldChanged", field: "commands", value: "ls\n\npwd\n" },
            { type: "saveSubmitted" },
            { type: "saveSucceeded", name: "work", profileNames: ["api", "logs", "work"] }
        );
        expect(s.phase).toBe("editing");
        expect(s.selected).toBe("work");
        expect(s.profileNames).toEqual(["api", "logs", "work"]);
        expect(s.form.name).toBe("work");
        expect(s.form.commands).toBe("ls\npwd");
        expect(s.saved).toEqual(s.form);
        expect(s.notice).toBe("Saved");
        expect(hasAnyUnsavedChanges(s)).toBe(false);
    });

    test("the next edit clears the notice", () => {
        const s = run(
            ...LOADED_API,
            { type: "saveSubmitted" },
            { type: "saveSucceeded", name: "api", profileNames: ["api", "logs"] },
            { type: "fieldChanged", field: "title", value: "x" }
        );
        expect(s.notice).toBeNull();
    });

    test("a save that overwrites a profile with a draft drops the draft", () => {
        const s = run(
            ...LOADED_API,
            { type: "fieldChanged", field: "title", value: "draft" },
            { type: "createSelected" },
            { type: "fieldChanged", field: "name", value: "api" },
            { type: "saveSubmitted" },
            { type: "saveSucceeded", name: "api", profileNames: ["api", "logs"] }
        );
        expect(s.drafts).toEqual({});
        expect(s.newDraft).toBeNull();
    });
});

describe("unsaved changes", () => {
    const EDITED_API = [
        ...LOADED_API,
        { type: "fieldChanged", field: "title", value: "API 2" },
    ] as ProfileEditorAction[];

    test("an edited profile is dirty, and Reset puts it back", () => {
        const s = run(...EDITED_API);
        expect(profileHasUnsavedChanges(s, "api")).toBe(true);
        expect(canResetProfile(s)).toBe(true);
        const reset = reduceProfileEditor(s, { type: "resetRequested" });
        expect(reset.form.title).toBe("API");
        expect(canResetProfile(reset)).toBe(false);
        expect(reduceProfileEditor(reset, { type: "resetRequested" })).toBe(reset);
    });

    test("editing back to the saved value isn't dirty", () => {
        const s = run(...EDITED_API, { type: "fieldChanged", field: "title", value: "API" });
        expect(hasAnyUnsavedChanges(s)).toBe(false);
    });

    test("leaving an edited profile keeps a draft that comes back without a reload", () => {
        const away = run(...EDITED_API, { type: "profileSelected", name: "logs" });
        expect(away.drafts["api"]?.form.title).toBe("API 2");
        expect(profileHasUnsavedChanges(away, "api")).toBe(true);
        expect(unsavedChanges(away)).toEqual({ names: ["api"], newProfile: false });

        const back = reduceProfileEditor(away, { type: "profileSelected", name: "api" });
        expect(back.phase).toBe("editing");
        expect(back.form.title).toBe("API 2");
        expect(back.saved?.title).toBe("API");
        expect(back.drafts).toEqual({});
    });

    test("clicking the selected profile again keeps its edits", () => {
        const s = run(...EDITED_API);
        expect(reduceProfileEditor(s, { type: "profileSelected", name: "api" })).toBe(s);
    });

    test("a new profile in progress is kept while another card is selected", () => {
        const s = run(
            OPENED,
            { type: "fieldChanged", field: "shell", value: "/bin/fish" },
            { type: "profileSelected", name: "api" }
        );
        expect(newProfileHasUnsavedChanges(s)).toBe(true);
        expect(unsavedChanges(s)).toEqual({ names: [], newProfile: true });
        const back = reduceProfileEditor(s, { type: "createSelected" });
        expect(back.form.shell).toBe("/bin/fish");
        expect(back.newDraft).toBeNull();
        // Reset clears it.
        expect(reduceProfileEditor(back, { type: "resetRequested" }).form.shell).toBe("");
    });

    test("an empty new profile isn't kept", () => {
        const s = run(OPENED, { type: "profileSelected", name: "api" });
        expect(s.newDraft).toBeNull();
        expect(hasAnyUnsavedChanges(s)).toBe(false);
    });

    test("a loading or failed profile has no unsaved changes", () => {
        expect(hasAnyUnsavedChanges(run(OPENED, { type: "profileSelected", name: "api" }))).toBe(false);
    });
});

describe("profilesChanged", () => {
    test("after a copy, selects the copy and keeps the source's draft", () => {
        const s = run(
            ...LOADED_API,
            { type: "fieldChanged", field: "title", value: "API 2" },
            { type: "profilesChanged", profileNames: ["api", "api-copy", "logs"], select: "api-copy" }
        );
        expect(s.profileNames).toEqual(["api", "api-copy", "logs"]);
        expect(s.selected).toBe("api-copy");
        expect(s.phase).toBe("loading");
        expect(s.drafts["api"]?.form.title).toBe("API 2");
    });

    test("after a delete, selects Create new profile and drops the deleted profile's draft", () => {
        const s = run(
            ...LOADED_API,
            { type: "fieldChanged", field: "title", value: "API 2" },
            { type: "profilesChanged", profileNames: ["logs"] }
        );
        expect(s.selected).toBeNull();
        expect(s.drafts).toEqual({});
        expect(hasAnyUnsavedChanges(s)).toBe(false);
    });
});

describe("copyProfileName", () => {
    test("picks the first free -copy name", () => {
        expect(copyProfileName(["api"], "api")).toBe("api-copy");
        expect(copyProfileName(["api", "api-copy", "api-copy-2"], "api")).toBe("api-copy-3");
    });

    test("copyProfileNameProblem rejects empty, padded, reserved, and taken names", () => {
        expect(copyProfileNameProblem(["api"], "")).toBe("Enter a name.");
        expect(copyProfileNameProblem(["api"], " x")).toBe("The name can't start or end with a space.");
        expect(copyProfileNameProblem(["api"], "default")).toBe("Cannot use 'default' as a profile name.");
        expect(copyProfileNameProblem(["api"], "api")).toBe("A profile with this name already exists.");
        expect(copyProfileNameProblem(["api"], "api-copy")).toBeNull();
    });
});

describe("profileFromForm", () => {
    test("trims fields and drops blank command lines", () => {
        expect(
            profileFromForm({
                name: "api",
                shell: " /bin/zsh ",
                title: "API ",
                workingDirectory: " ~/api",
                root: "",
                commands: "npm install\n\nnpm start\n",
            })
        ).toEqual({
            shell: "/bin/zsh",
            title: "API",
            workingDirectory: "~/api",
            root: "",
            commands: ["npm install", "npm start"],
        });
    });
});
