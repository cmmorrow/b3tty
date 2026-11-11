import { describe, expect, test } from "bun:test";
import {
    canCopyTheme,
    canDeleteTheme,
    canExportTheme,
    canResetTheme,
    canSaveTheme,
    cardPalette,
    copyName,
    copyNameProblem,
    formIsDirty,
    hasAnyUnsavedChanges,
    hasUnsavedChanges,
    initialThemeEditorState,
    isReadOnly,
    nameIsTaken,
    previewPalette,
    reduceThemeEditor,
    themeDataFromForm,
    themeHasUnsavedChanges,
    newThemeHasUnsavedChanges,
    unsavedChanges,
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

    test("going back to Create clears the form and disables Save", () => {
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

describe("reduceThemeEditor: saving without closing", () => {
    const EDITED: ThemeEditorAction[] = [
        ...LOADED_MIDNIGHT,
        { type: "fieldChanged", field: "background", value: "#202040" },
    ];

    test("a save keeps the form, makes it the saved theme, and says so", () => {
        const s = run(...EDITED, { type: "saveSubmitted" }, { type: "saveSucceeded", name: "midnight" });
        expect(s).toMatchObject({ phase: "editing", selected: "midnight", notice: "Saved" });
        expect(s.form.colors.background).toBe("#202040");
        expect(hasUnsavedChanges(s)).toBe(false);
        expect(formIsDirty(s)).toBe(false);
        expect(canSaveTheme(s)).toBe(true);
        // The next edit clears the notice.
        const edited = reduceThemeEditor(s, { type: "fieldChanged", field: "red", value: "#ff0000" });
        expect(edited.notice).toBeNull();
        expect(formIsDirty(edited)).toBe(true);
    });

    test("a new theme is selected under its trimmed name once saved", () => {
        const s = run(
            ...LISTED,
            { type: "fieldChanged", field: "name", value: " fresh " },
            { type: "saveSubmitted" },
            { type: "saveSucceeded", name: "fresh" }
        );
        expect(s).toMatchObject({ selected: "fresh", phase: "editing" });
        expect(s.form.name).toBe("fresh");
        expect(formIsDirty(s)).toBe(false);
        // The refresh that follows keeps it selected without reloading it.
        const refreshed = reduceThemeEditor(
            reduceThemeEditor(s, {
                type: "refreshStarted",
                entries: [...ENTRIES, { name: "fresh", source: "user", inMenu: true }],
            }),
            {
                type: "refreshLoaded",
                listId: 2,
                palettes: { ...PALETTES, fresh: PALETTE },
                keepSelection: true,
            }
        );
        expect(refreshed).toMatchObject({ selected: "fresh", phase: "editing", notice: "Saved" });
        expect(refreshed.form).toBe(s.form);
    });

    test("only a save in flight can succeed", () => {
        const s = run(...EDITED);
        expect(reduceThemeEditor(s, { type: "saveSucceeded", name: "midnight" })).toBe(s);
    });
});

describe("reduceThemeEditor: unsaved drafts", () => {
    const EDITED: ThemeEditorAction[] = [
        ...LOADED_MIDNIGHT,
        { type: "fieldChanged", field: "background", value: "#202040" },
    ];

    test("leaving an edited theme keeps its draft, badges it, and previews it on its card", () => {
        const s = run(...EDITED, { type: "themeSelected", name: "dracula" });
        expect(s.selected).toBe("dracula");
        expect(s.drafts["midnight"]?.form.colors.background).toBe("#202040");
        expect(themeHasUnsavedChanges(s, "midnight")).toBe(true);
        expect(themeHasUnsavedChanges(s, "dracula")).toBe(false);
        expect(cardPalette(s, "midnight")?.bg).toBe("#202040");
        expect(cardPalette(s, "aurora")).toBe(PALETTE);
    });

    test("selecting a theme with a draft restores it without loading", () => {
        const s = run(
            ...EDITED,
            { type: "themeSelected", name: "dracula" },
            { type: "themeSelected", name: "midnight" }
        );
        expect(s).toMatchObject({ selected: "midnight", phase: "editing" });
        expect(s.form.colors.background).toBe("#202040");
        expect(s.saved?.colors.background).toBe("#101020");
        expect(s.drafts).toEqual({});
        expect(hasUnsavedChanges(s)).toBe(true);
    });

    test("leaving an unedited theme keeps no draft", () => {
        const s = run(...LOADED_MIDNIGHT, { type: "themeSelected", name: "dracula" });
        expect(s.drafts).toEqual({});
        expect(hasAnyUnsavedChanges(s)).toBe(false);
    });

    test("a renamed user theme counts as unsaved", () => {
        const s = run(...LOADED_MIDNIGHT, { type: "fieldChanged", field: "name", value: "midnight-2" });
        expect(hasUnsavedChanges(s)).toBe(false);
        expect(formIsDirty(s)).toBe(true);
    });

    test("a new theme in progress is kept while another card is selected", () => {
        const s = run(
            ...LISTED,
            { type: "fieldChanged", field: "name", value: "fresh" },
            { type: "themeSelected", name: "midnight" }
        );
        expect(s.newDraft?.name).toBe("fresh");
        expect(newThemeHasUnsavedChanges(s)).toBe(true);
        const back = reduceThemeEditor(s, { type: "createSelected" });
        expect(back.form.name).toBe("fresh");
        expect(back.newDraft).toBeNull();
        expect(newThemeHasUnsavedChanges(back)).toBe(true);
    });

    test("clicking Create new theme again keeps the form", () => {
        const s = run(...LISTED, { type: "fieldChanged", field: "name", value: "fresh" });
        expect(reduceThemeEditor(s, { type: "createSelected" })).toBe(s);
    });

    test("read-only themes never count as unsaved", () => {
        const s = run(
            ...LISTED,
            { type: "themeSelected", name: "dracula" },
            { type: "themeLoaded", name: "dracula", config: MIDNIGHT },
            { type: "themeSelected", name: "midnight" }
        );
        expect(s.drafts).toEqual({});
    });

    test("unsavedChanges lists every theme Close would discard", () => {
        const s = run(
            ...LISTED,
            { type: "fieldChanged", field: "name", value: "fresh" },
            { type: "themeSelected", name: "midnight" },
            { type: "themeLoaded", name: "midnight", config: MIDNIGHT },
            { type: "fieldChanged", field: "red", value: "#ff0000" }
        );
        expect(unsavedChanges(s)).toEqual({ names: ["midnight"], newTheme: true });
        expect(hasAnyUnsavedChanges(s)).toBe(true);
    });

    test("a refresh that removes a theme drops its draft", () => {
        const s = run(
            ...EDITED,
            { type: "themeSelected", name: "dracula" },
            { type: "refreshStarted", entries: ENTRIES.slice(1) },
            { type: "refreshLoaded", listId: 2, palettes: PALETTES }
        );
        expect(s.drafts).toEqual({});
    });

    test("a refresh that selects another theme keeps the selection's draft", () => {
        const s = run(
            ...EDITED,
            { type: "refreshStarted", entries: ENTRIES },
            { type: "refreshLoaded", listId: 2, palettes: PALETTES, select: "aurora" }
        );
        expect(s.selected).toBe("aurora");
        expect(s.drafts["midnight"]?.form.colors.background).toBe("#202040");
    });

    test("saving over a theme with a draft drops that draft", () => {
        const s = run(
            ...EDITED,
            { type: "createSelected" },
            { type: "fieldChanged", field: "name", value: "midnight" },
            { type: "saveSubmitted" },
            { type: "saveSucceeded", name: "midnight" }
        );
        expect(s.drafts).toEqual({});
    });

    test("opening again discards every draft", () => {
        const s = run(...EDITED, { type: "createSelected" }, { type: "opened" });
        expect(s.drafts).toEqual({});
        expect(s.newDraft).toBeNull();
    });
});

describe("reduceThemeEditor: reset", () => {
    test("restores a user theme to its saved values", () => {
        const s = run(
            ...LOADED_MIDNIGHT,
            { type: "fieldChanged", field: "background", value: "#202040" },
            { type: "fieldChanged", field: "name", value: "midnight-2" }
        );
        expect(canResetTheme(s)).toBe(true);
        const reset = reduceThemeEditor(s, { type: "resetRequested" });
        expect(reset.form).toBe(reset.saved!);
        expect(reset.form.name).toBe("midnight");
        expect(formIsDirty(reset)).toBe(false);
        expect(canResetTheme(reset)).toBe(false);
    });

    test("restores what was last saved, not what was first loaded", () => {
        const s = run(
            ...LOADED_MIDNIGHT,
            { type: "fieldChanged", field: "background", value: "#202040" },
            { type: "saveSubmitted" },
            { type: "saveSucceeded", name: "midnight" },
            { type: "fieldChanged", field: "red", value: "#00ff00" },
            { type: "resetRequested" }
        );
        expect(s.form.colors.background).toBe("#202040");
        expect(s.form.colors.red).toBe("#ff5555");
        expect(s.notice).toBeNull();
    });

    test("empties a new theme in progress", () => {
        const s = run(...LISTED, { type: "fieldChanged", field: "name", value: "fresh" }, { type: "resetRequested" });
        expect(s.form.name).toBe("");
        expect(newThemeHasUnsavedChanges(s)).toBe(false);
    });

    test("does nothing without unsaved changes, for a read-only theme, or while saving", () => {
        const clean = run(...LOADED_MIDNIGHT);
        expect(reduceThemeEditor(clean, { type: "resetRequested" })).toBe(clean);
        const builtin = run(
            ...LISTED,
            { type: "themeSelected", name: "dracula" },
            { type: "themeLoaded", name: "dracula", config: MIDNIGHT }
        );
        expect(canResetTheme(builtin)).toBe(false);
        const saving = run(
            ...LOADED_MIDNIGHT,
            { type: "fieldChanged", field: "red", value: "#00ff00" },
            { type: "saveSubmitted" }
        );
        expect(reduceThemeEditor(saving, { type: "resetRequested" })).toBe(saving);
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
