import { describe, expect, test } from "bun:test";
import {
    canDeleteProfile,
    canSaveProfile,
    initialProfileEditorState,
    isProfileEditorBusy,
    profileFromForm,
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

    test("selecting a profile loads it, with OK disabled until it arrives", () => {
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

    test("going back to Create clears the form and disables OK", () => {
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
        expect(canDeleteProfile(s)).toBe(false);
    });

    test("an existing profile's name can't be changed", () => {
        const s = run(...LOADED_API);
        expect(reduceProfileEditor(s, { type: "fieldChanged", field: "name", value: "other" })).toBe(s);
    });

    test("fields can't change while loading", () => {
        const loading = run(OPENED, { type: "profileSelected", name: "api" });
        expect(reduceProfileEditor(loading, { type: "fieldChanged", field: "shell", value: "x" })).toBe(loading);
    });

    test("saving blocks a second save, deleting, selecting, and editing", () => {
        const saving = run(...LOADED_API, { type: "saveSubmitted" });
        expect(saving.phase).toBe("saving");
        expect(isProfileEditorBusy(saving)).toBe(true);
        for (const action of [
            { type: "saveSubmitted" },
            { type: "deleteSubmitted" },
            { type: "createSelected" },
            { type: "profileSelected", name: "logs" },
            { type: "fieldChanged", field: "shell", value: "x" },
        ] as ProfileEditorAction[]) {
            expect(reduceProfileEditor(saving, action)).toBe(saving);
        }
    });

    test("deleting is only possible for a loaded existing profile", () => {
        expect(reduceProfileEditor(run(OPENED), { type: "deleteSubmitted" }).phase).toBe("editing");
        expect(run(...LOADED_API, { type: "deleteSubmitted" }).phase).toBe("deleting");
    });

    test("a failed request returns to editing with the reason", () => {
        const s = run(...LOADED_API, { type: "saveSubmitted" }, { type: "requestFailed", message: "disk full" });
        expect(s.phase).toBe("editing");
        expect(s.error).toBe("disk full");
        expect(canSaveProfile(s)).toBe(true);
    });

    test("editing a field clears the error", () => {
        const s = run(
            ...LOADED_API,
            { type: "saveSubmitted" },
            { type: "requestFailed", message: "disk full" },
            { type: "fieldChanged", field: "title", value: "API 2" }
        );
        expect(s.error).toBeNull();
        expect(s.form.title).toBe("API 2");
    });

    test("a failure with no request in flight is ignored", () => {
        const s = run(...LOADED_API);
        expect(reduceProfileEditor(s, { type: "requestFailed", message: "late" })).toBe(s);
    });

    test("opening again starts over", () => {
        const s = run(...LOADED_API, { type: "saveSubmitted" }, { type: "opened", profileNames: ["logs"] });
        expect(s).toEqual({ ...initialProfileEditorState, profileNames: ["logs"] });
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
