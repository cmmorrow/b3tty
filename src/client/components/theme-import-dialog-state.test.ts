import { describe, expect, test } from "bun:test";
import {
    canSubmitThemeImport,
    initialThemeImportState,
    reduceThemeImport,
    type ThemeImportAction,
    type ThemeImportState,
} from "./theme-import-dialog-state.ts";

function run(...actions: ThemeImportAction[]): ThemeImportState {
    return actions.reduce(reduceThemeImport, initialThemeImportState);
}

const UPGRADE = "aurora 1.0 is installed; this package is 2.0.";

describe("reduceThemeImport", () => {
    test("starts empty, with nothing to submit", () => {
        expect(run({ type: "opened" })).toEqual({ phase: "editing", path: "", upgrade: null, error: null });
        expect(canSubmitThemeImport(initialThemeImportState)).toBe(false);
    });

    test("a typed path can be submitted, a blank one can't", () => {
        expect(canSubmitThemeImport(run({ type: "pathChanged", path: "~/a.b3theme" }))).toBe(true);
        expect(canSubmitThemeImport(run({ type: "pathChanged", path: "   " }))).toBe(false);
    });

    test("submitting starts an import", () => {
        const s = run({ type: "pathChanged", path: "~/a.b3theme" }, { type: "submitted" });
        expect(s).toEqual({ phase: "importing", path: "~/a.b3theme", upgrade: null });
        expect(canSubmitThemeImport(s)).toBe(false);
    });

    test("submitting with no path does nothing", () => {
        expect(reduceThemeImport(initialThemeImportState, { type: "submitted" })).toBe(initialThemeImportState);
    });

    test("a second submit while importing is ignored", () => {
        const importing = run({ type: "pathChanged", path: "~/a.b3theme" }, { type: "submitted" });
        expect(reduceThemeImport(importing, { type: "submitted" })).toBe(importing);
    });

    test("the path can't change while importing", () => {
        const importing = run({ type: "pathChanged", path: "~/a.b3theme" }, { type: "submitted" });
        expect(reduceThemeImport(importing, { type: "pathChanged", path: "~/b.b3theme" })).toBe(importing);
    });

    test("an upgrade switches to replace mode, which the next submit sends", () => {
        const s = run(
            { type: "pathChanged", path: "~/a.b3theme" },
            { type: "submitted" },
            { type: "upgradeRequired", message: UPGRADE }
        );
        expect(s).toEqual({ phase: "editing", path: "~/a.b3theme", upgrade: UPGRADE, error: null });
        expect(reduceThemeImport(s, { type: "submitted" })).toEqual({
            phase: "importing",
            path: "~/a.b3theme",
            upgrade: UPGRADE,
        });
    });

    test("editing the path leaves replace mode and clears the error", () => {
        const s = run(
            { type: "pathChanged", path: "~/a.b3theme" },
            { type: "submitted" },
            { type: "upgradeRequired", message: UPGRADE },
            { type: "pathChanged", path: "~/b.b3theme" }
        );
        expect(s).toEqual({ phase: "editing", path: "~/b.b3theme", upgrade: null, error: null });
    });

    test("a failure shows its reason and keeps replace mode", () => {
        const s = run(
            { type: "pathChanged", path: "~/a.b3theme" },
            { type: "submitted" },
            { type: "upgradeRequired", message: UPGRADE },
            { type: "submitted" },
            { type: "failed", message: "permission denied" }
        );
        expect(s).toEqual({ phase: "editing", path: "~/a.b3theme", upgrade: UPGRADE, error: "permission denied" });
        expect(canSubmitThemeImport(s)).toBe(true);
    });

    test("typing after a failure clears the error", () => {
        const s = run(
            { type: "pathChanged", path: "~/a.b3theme" },
            { type: "submitted" },
            { type: "failed", message: "no such file" },
            { type: "pathChanged", path: "~/a.b3them" }
        );
        expect(s).toEqual({ phase: "editing", path: "~/a.b3them", upgrade: null, error: null });
    });

    test("responses that arrive when no import is in flight are ignored", () => {
        const editing = run({ type: "pathChanged", path: "~/a.b3theme" });
        expect(reduceThemeImport(editing, { type: "failed", message: "late" })).toBe(editing);
        expect(reduceThemeImport(editing, { type: "upgradeRequired", message: "late" })).toBe(editing);
    });

    test("opening again starts over", () => {
        const s = run(
            { type: "pathChanged", path: "~/a.b3theme" },
            { type: "submitted" },
            { type: "failed", message: "no such file" },
            { type: "opened" }
        );
        expect(s).toEqual(initialThemeImportState);
    });
});
