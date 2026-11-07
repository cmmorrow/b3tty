import { describe, expect, test } from "bun:test";
import {
    canExportTheme,
    initialThemeExportState,
    reduceThemeExport,
    sameAsLastVersion,
    showRepoError,
    themeExportMetadata,
    type ThemeExportAction,
    type ThemeExportState,
} from "./theme-export-dialog-state.ts";

function run(...actions: ThemeExportAction[]): ThemeExportState {
    return actions.reduce(reduceThemeExport, initialThemeExportState);
}

const OPENED: ThemeExportAction = { type: "opened", themeName: "midnight", source: "user", unsavedChanges: false };
const FILLED: ThemeExportAction[] = [
    OPENED,
    { type: "fieldChanged", field: "repo", value: "https://github.com/me/midnight" },
    { type: "fieldChanged", field: "author", value: "Me" },
    { type: "fieldChanged", field: "themeVersion", value: "1.1.0" },
    { type: "fieldChanged", field: "description", value: "Dark blue" },
];

describe("reduceThemeExport", () => {
    test("a user theme opens with its default path and empty details", () => {
        const s = run(OPENED);
        expect(s.phase).toBe("editing");
        if (s.phase === "unavailable") throw new Error("unreachable");
        expect(s.path).toBe("~/midnight.b3theme");
        expect(s.fields).toEqual({ repo: "", author: "", description: "", themeVersion: "" });
        expect(canExportTheme(s)).toBe(false);
    });

    test("no theme, a built-in, or an installed theme can't be exported", () => {
        expect(run({ type: "opened", themeName: "", source: "user", unsavedChanges: false })).toMatchObject({
            phase: "unavailable",
            reason: "noTheme",
        });
        expect(run({ type: "opened", themeName: "dracula", source: "builtin", unsavedChanges: false })).toMatchObject({
            phase: "unavailable",
            reason: "builtin",
        });
        expect(run({ type: "opened", themeName: "aurora", source: "installed", unsavedChanges: false })).toMatchObject({
            phase: "unavailable",
            reason: "installed",
        });
    });

    test("nothing can be typed or submitted when unavailable", () => {
        const s = run({ type: "opened", themeName: "", source: "user", unsavedChanges: false });
        expect(reduceThemeExport(s, { type: "pathChanged", path: "~/x" })).toBe(s);
        expect(reduceThemeExport(s, { type: "fieldChanged", field: "author", value: "x" })).toBe(s);
        expect(reduceThemeExport(s, { type: "submitted" })).toBe(s);
    });

    test("Export enables once every detail is filled in", () => {
        expect(canExportTheme(run(...FILLED))).toBe(true);
        expect(canExportTheme(run(...FILLED, { type: "fieldChanged", field: "description", value: "  " }))).toBe(false);
        expect(canExportTheme(run(...FILLED, { type: "pathChanged", path: "" }))).toBe(false);
    });

    test("a repository that isn't https shows an error and blocks Export", () => {
        const s = run(...FILLED, { type: "fieldChanged", field: "repo", value: "http://github.com/me/midnight" });
        expect(showRepoError(s)).toBe(true);
        expect(canExportTheme(s)).toBe(false);
        expect(showRepoError(run(OPENED))).toBe(false);
    });

    test("saved details fill only the empty fields and record the last version", () => {
        const { openId } = run(OPENED);
        const s = run(
            OPENED,
            { type: "fieldChanged", field: "author", value: "Typed" },
            {
                type: "prefillLoaded",
                openId,
                saved: { repo: "https://github.com/me/midnight", author: "Saved", themeVersion: "1.0.0" },
            }
        );
        if (s.phase === "unavailable") throw new Error("unreachable");
        expect(s.fields).toEqual({
            repo: "https://github.com/me/midnight",
            author: "Typed",
            description: "",
            themeVersion: "1.0.0",
        });
        expect(s.lastVersion).toBe("1.0.0");
        expect(sameAsLastVersion(s)).toBe(true);
        expect(
            sameAsLastVersion(reduceThemeExport(s, { type: "fieldChanged", field: "themeVersion", value: "1.1" }))
        ).toBe(false);
    });

    test("saved details fetched for an earlier open are ignored", () => {
        const first = run(OPENED);
        const second = reduceThemeExport(first, OPENED);
        const late = reduceThemeExport(second, {
            type: "prefillLoaded",
            openId: first.openId,
            saved: { author: "Old" },
        });
        expect(late).toBe(second);
    });

    test("exporting blocks edits and a second submit", () => {
        const exporting = run(...FILLED, { type: "submitted" });
        expect(exporting.phase).toBe("exporting");
        expect(canExportTheme(exporting)).toBe(false);
        expect(reduceThemeExport(exporting, { type: "submitted" })).toBe(exporting);
        expect(reduceThemeExport(exporting, { type: "pathChanged", path: "~/x" })).toBe(exporting);
        expect(reduceThemeExport(exporting, { type: "fieldChanged", field: "author", value: "x" })).toBe(exporting);
    });

    test("an incomplete form can't be submitted", () => {
        const s = run(OPENED);
        expect(reduceThemeExport(s, { type: "submitted" })).toBe(s);
    });

    test("a failed export returns to editing with the reason, and typing clears it", () => {
        const failed = run(...FILLED, { type: "submitted" }, { type: "failed", message: "file exists" });
        expect(failed.phase).toBe("editing");
        expect(failed).toMatchObject({ error: "file exists" });
        expect(canExportTheme(failed)).toBe(true);
        expect(reduceThemeExport(failed, { type: "pathChanged", path: "~/other" })).toMatchObject({ error: null });
    });

    test("a failure with no export in flight is ignored", () => {
        const s = run(...FILLED);
        expect(reduceThemeExport(s, { type: "failed", message: "late" })).toBe(s);
    });

    test("opening again starts over", () => {
        const s = run(...FILLED, { type: "opened", themeName: "other", source: "user", unsavedChanges: true });
        expect(s).toMatchObject({ themeName: "other", unsavedChanges: true, path: "~/other.b3theme", error: null });
        if (s.phase === "unavailable") throw new Error("unreachable");
        expect(s.fields.author).toBe("");
    });
});

describe("themeExportMetadata", () => {
    test("trims every field", () => {
        expect(
            themeExportMetadata({ repo: " https://x.dev ", author: "Me ", description: " D", themeVersion: "1 " })
        ).toEqual({ repo: "https://x.dev", author: "Me", description: "D", themeVersion: "1" });
    });
});
