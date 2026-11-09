import { describe, expect, test } from "bun:test";
import {
    canCopyTheme,
    canDeleteTheme,
    canExportTheme,
    canSaveTheme,
    copyName,
    copyNameProblem,
    hasUnsavedChanges,
    initialThemeEditorState,
    isReadOnly,
    nameIsTaken,
    previewPalette,
    reduceThemeEditor,
    themeDataFromForm,
    type ThemeEditorAction,
    type ThemeEditorState,
} from "./theme-editor-state.ts";
import type { Palette, ThemeConfig, ThemeEntry } from "../types.ts";

const ENTRIES: ThemeEntry[] = [
    { name: "midnight", source: "user", inMenu: true },
    { name: "aurora", source: "installed", inMenu: false },
    { name: "dracula", source: "builtin", inMenu: true },
    { name: "solarized-dark", source: "builtin", inMenu: false },
];
const PALETTE: Palette = {
    bg: "#000000",
    fg: "#ffffff",
    selBg: "#333333",
    cursor: "#ffffff",
    normal: ["#000", "#f00", "#ff0", "#0f0", "#0ff", "#00f", "#f0f", "#fff"],
    bright: ["#111", "#f11", "#ff1", "#1f1", "#1ff", "#11f", "#f1f", "#eee"],
};
const PALETTES = Object.fromEntries(ENTRIES.map((e) => [e.name, PALETTE]));
const MIDNIGHT: ThemeConfig = {
    background: "#101020",
    foreground: "#e0e0f0",
    red: "#ff5555",
    backgroundImage: "~/bg.png",
    backgroundImageTransparency: 40,
};

function run(...actions: ThemeEditorAction[]): ThemeEditorState {
    return actions.reduce(reduceThemeEditor, initialThemeEditorState);
}

/** Opened and refreshed, with every palette loaded. */
const LISTED: ThemeEditorAction[] = [
    { type: "opened" },
    { type: "refreshStarted", entries: ENTRIES },
    { type: "refreshLoaded", listId: 1, palettes: PALETTES },
];
const LOADED_MIDNIGHT: ThemeEditorAction[] = [
    ...LISTED,
    { type: "themeSelected", name: "midnight" },
    { type: "themeLoaded", name: "midnight", config: MIDNIGHT },
];

describe("reduceThemeEditor: the list", () => {
    test("a refresh lands on Create new theme unless it names a theme to select", () => {
        const s = run(...LISTED);
        expect(s.selected).toBeNull();
        expect(s.palettes).toBe(PALETTES);
        const selecting = run(
            { type: "opened" },
            { type: "refreshStarted", entries: ENTRIES },
            { type: "refreshLoaded", listId: 1, palettes: PALETTES, select: "aurora" }
        );
        expect(selecting).toMatchObject({ selected: "aurora", phase: "loading" });
    });

    test("palettes from an earlier refresh are ignored", () => {
        const s = run(
            { type: "opened" },
            { type: "refreshStarted", entries: ENTRIES },
            { type: "refreshStarted", entries: ENTRIES.slice(0, 1) }
        );
        expect(reduceThemeEditor(s, { type: "refreshLoaded", listId: 1, palettes: PALETTES })).toBe(s);
    });

    test("selecting a theme with no card does nothing", () => {
        const s = run(...LISTED);
        expect(reduceThemeEditor(s, { type: "themeSelected", name: "nope" })).toBe(s);
    });
});

describe("reduceThemeEditor: selecting and loading", () => {
    test("a new theme can be saved once it has a name", () => {
        const s = run(...LISTED, { type: "fieldChanged", field: "name", value: "sunrise" });
        expect(canSaveTheme(s)).toBe(true);
        expect(canCopyTheme(s)).toBe(false);
        expect(canDeleteTheme(s)).toBe(false);
        expect(canExportTheme(s)).toBe(false);
    });

    test("a built-in or installed name can't be used", () => {
        const s = run(...LISTED, { type: "fieldChanged", field: "name", value: "dracula" });
        expect(nameIsTaken(s)).toBe(true);
        expect(canSaveTheme(s)).toBe(false);
    });

    test("selecting a theme loads it, with OK disabled until it arrives", () => {
        const loading = run(...LISTED, { type: "themeSelected", name: "midnight" });
        expect(loading).toMatchObject({ selected: "midnight", phase: "loading" });
        expect(loading.form.name).toBe("midnight");
        expect(canSaveTheme(loading)).toBe(false);

        const loaded = run(...LOADED_MIDNIGHT);
        expect(loaded.phase).toBe("editing");
        expect(loaded.form.colors.background).toBe("#101020");
        expect(loaded.form.colors.cursor).toBe("");
        expect(loaded.form.backgroundImage).toBe("~/bg.png");
        expect(loaded.form.transparency).toBe("40");
        expect(canSaveTheme(loaded)).toBe(true);
        expect(canExportTheme(loaded)).toBe(true);
    });

    test("clicking the selected theme again keeps the edits and starts no new load", () => {
        const edited = run(...LOADED_MIDNIGHT, { type: "fieldChanged", field: "red", value: "#ff0000" });
        expect(reduceThemeEditor(edited, { type: "themeSelected", name: "midnight" })).toBe(edited);
        const loading = run(...LISTED, { type: "themeSelected", name: "midnight" });
        expect(reduceThemeEditor(loading, { type: "themeSelected", name: "midnight" })).toBe(loading);
    });

    test("clicking the selected theme after a failed load retries it", () => {
        const failed = run(
            ...LISTED,
            { type: "themeSelected", name: "midnight" },
            { type: "themeLoadFailed", name: "midnight", message: "timeout" }
        );
        expect(reduceThemeEditor(failed, { type: "themeSelected", name: "midnight" })).toMatchObject({
            selected: "midnight",
            phase: "loading",
            error: null,
        });
    });

    test("going back to Create clears the form and disables OK", () => {
        const s = run(...LOADED_MIDNIGHT, { type: "createSelected" });
        expect(s.selected).toBeNull();
        expect(s.form.colors.background).toBe("");
        expect(canSaveTheme(s)).toBe(false);
    });

    test("a load that finishes after another selection is ignored", () => {
        const s = run(
            ...LISTED,
            { type: "themeSelected", name: "midnight" },
            { type: "themeSelected", name: "dracula" },
            { type: "themeLoaded", name: "midnight", config: MIDNIGHT }
        );
        expect(s).toMatchObject({ selected: "dracula", phase: "loading" });
        expect(s.form.colors.background).toBe("");
    });

    test("a failed load shows why and can't be saved over", () => {
        const s = run(
            ...LISTED,
            { type: "themeSelected", name: "midnight" },
            {
                type: "themeLoadFailed",
                name: "midnight",
                message: "404 Not Found",
            }
        );
        expect(s.phase).toBe("loadFailed");
        expect(s.error).toBe("Couldn't load midnight: 404 Not Found");
        expect(canSaveTheme(s)).toBe(false);
        // Copy and Delete work from the server's copy, so they stay available.
        expect(canCopyTheme(s)).toBe(true);
        expect(canDeleteTheme(s)).toBe(true);
    });
});

describe("reduceThemeEditor: read-only themes", () => {
    test("a built-in or installed theme can't be edited or exported, only copied", () => {
        for (const name of ["dracula", "aurora"]) {
            const s = run(...LISTED, { type: "themeSelected", name }, { type: "themeLoaded", name, config: {} });
            expect(isReadOnly(s)).toBe(true);
            expect(canSaveTheme(s)).toBe(false);
            expect(canExportTheme(s)).toBe(false);
            expect(canCopyTheme(s)).toBe(true);
            expect(reduceThemeEditor(s, { type: "fieldChanged", field: "red", value: "#f00" })).toBe(s);
        }
    });

    test("a built-in can be deleted (removed from the menu) only when it's in the menu", () => {
        const inMenu = run(...LISTED, { type: "themeSelected", name: "dracula" });
        const notInMenu = run(...LISTED, { type: "themeSelected", name: "solarized-dark" });
        expect(canDeleteTheme(inMenu)).toBe(true);
        expect(canDeleteTheme(notInMenu)).toBe(false);
    });
});

describe("reduceThemeEditor: editing and saving", () => {
    test("an invalid color or transparency disables OK", () => {
        expect(canSaveTheme(run(...LOADED_MIDNIGHT, { type: "fieldChanged", field: "red", value: "#zz" }))).toBe(false);
        expect(
            canSaveTheme(run(...LOADED_MIDNIGHT, { type: "fieldChanged", field: "transparency", value: "101" }))
        ).toBe(false);
    });

    test("unsaved changes are tracked against the theme as loaded", () => {
        const loaded = run(...LOADED_MIDNIGHT);
        expect(hasUnsavedChanges(loaded)).toBe(false);
        const changed = reduceThemeEditor(loaded, { type: "fieldChanged", field: "red", value: "#ff0000" });
        expect(hasUnsavedChanges(changed)).toBe(true);
        const back = reduceThemeEditor(changed, { type: "fieldChanged", field: "red", value: "#ff5555" });
        expect(hasUnsavedChanges(back)).toBe(false);
        expect(hasUnsavedChanges(run(...LISTED, { type: "fieldChanged", field: "red", value: "#f00" }))).toBe(false);
    });

    test("saving blocks a second save, edits, and selecting", () => {
        const saving = run(...LOADED_MIDNIGHT, { type: "saveSubmitted" });
        expect(saving.phase).toBe("saving");
        for (const action of [
            { type: "saveSubmitted" },
            { type: "createSelected" },
            { type: "themeSelected", name: "dracula" },
            { type: "fieldChanged", field: "red", value: "#f00" },
        ] as ThemeEditorAction[]) {
            expect(reduceThemeEditor(saving, action)).toBe(saving);
        }
        expect(canCopyTheme(saving)).toBe(false);
        expect(canDeleteTheme(saving)).toBe(false);
        expect(canExportTheme(saving)).toBe(false);
    });

    test("a refresh during a save updates the list but keeps the selection", () => {
        const saving = run(...LOADED_MIDNIGHT, { type: "saveSubmitted" }, { type: "refreshStarted", entries: ENTRIES });
        const s = reduceThemeEditor(saving, { type: "refreshLoaded", listId: 2, palettes: {} });
        expect(s).toMatchObject({ selected: "midnight", phase: "saving", palettes: {} });
    });

    test("a failed save returns to editing with the reason, and editing clears it", () => {
        const failed = run(...LOADED_MIDNIGHT, { type: "saveSubmitted" }, { type: "saveFailed", message: "bad path" });
        expect(failed).toMatchObject({ phase: "editing", error: "bad path" });
        expect(canSaveTheme(failed)).toBe(true);
        expect(reduceThemeEditor(failed, { type: "fieldChanged", field: "backgroundImage", value: "" }).error).toBe(
            null
        );
    });

    test("opening again starts over but keeps counting refreshes", () => {
        const s = run(...LOADED_MIDNIGHT, { type: "opened" });
        expect(s).toEqual({ ...initialThemeEditorState, listId: 1 });
    });
});

describe("previewPalette", () => {
    test("lays valid typed colors over the loaded palette", () => {
        const s = run(
            ...LOADED_MIDNIGHT,
            { type: "fieldChanged", field: "brightWhite", value: "#abcdef" },
            { type: "fieldChanged", field: "cursor", value: "#nope" }
        );
        const preview = previewPalette(s)!;
        expect(preview.bg).toBe("#101020");
        expect(preview.cursor).toBe(PALETTE.cursor);
        expect(preview.normal[1]).toBe("#ff5555");
        expect(preview.bright[7]).toBe("#abcdef");
    });

    test("is null for Create new theme", () => {
        expect(previewPalette(run(...LISTED))).toBeNull();
    });
});

describe("themeDataFromForm", () => {
    test("sends set colors, the trimmed image path, and the transparency when set", () => {
        const form = run(...LOADED_MIDNIGHT, {
            type: "fieldChanged",
            field: "backgroundImage",
            value: " ~/bg.png ",
        }).form;
        expect(themeDataFromForm(form)).toEqual({
            background: "#101020",
            foreground: "#e0e0f0",
            red: "#ff5555",
            backgroundImage: "~/bg.png",
            backgroundImageTransparency: 40,
        });
        expect(themeDataFromForm({ ...form, transparency: "", backgroundImage: "" })).toMatchObject({
            backgroundImage: "",
        });
        expect("backgroundImageTransparency" in themeDataFromForm({ ...form, transparency: "" })).toBe(false);
    });
});

describe("copyName and copyNameProblem", () => {
    test("suggests the first free name", () => {
        expect(copyName(ENTRIES, "dracula")).toBe("dracula-copy");
        const withCopy = [...ENTRIES, { name: "dracula-copy", source: "user", inMenu: true } as ThemeEntry];
        expect(copyName(withCopy, "dracula")).toBe("dracula-copy-2");
    });

    test("rejects names the server would", () => {
        expect(copyNameProblem(ENTRIES, "")).toBe("Enter a name.");
        expect(copyNameProblem(ENTRIES, " x")).toBe("The name can't start or end with a space.");
        expect(copyNameProblem(ENTRIES, "a/b")).toBe("The name can't contain a /.");
        expect(copyNameProblem(ENTRIES, "x".repeat(65))).toBe("The name can't be longer than 64 characters.");
        expect(copyNameProblem(ENTRIES, "dracula")).toBe("A theme with this name already exists.");
        expect(copyNameProblem(ENTRIES, "fresh")).toBeNull();
    });
});
