import { describe, expect, test } from "bun:test";
import {
    autoResizeChanged,
    canSaveSettings,
    initialSettingsEditorState,
    reduceSettingsEditor,
    serverSettingsChanged,
    settingsDirty,
    settingsFromForm,
    settingsProblem,
    type SettingsEditorAction,
    type SettingsEditorState,
} from "./settings-editor-state.ts";
import type { SettingsConfig } from "../types.ts";

const SETTINGS: SettingsConfig = {
    server: { port: 8080, noAuth: false, noBrowser: false, showMenubar: "hover" },
    terminal: { fontFamily: "Fira Code", fontSize: 14, autoResize: true, rows: 24, columns: 80 },
};
const OPENED: SettingsEditorAction = { type: "opened", settings: SETTINGS };

function run(...actions: SettingsEditorAction[]): SettingsEditorState {
    return actions.reduce(reduceSettingsEditor, initialSettingsEditorState);
}

describe("reduceSettingsEditor", () => {
    test("opens on the Server tab with nothing changed", () => {
        const s = run(OPENED);
        expect(s.tab).toBe("server");
        expect(s.form.port).toBe("8080");
        expect(settingsDirty(s)).toBe(false);
        expect(canSaveSettings(s)).toBe(false);
    });

    test("changing a field enables OK, and changing it back disables it again", () => {
        const changed = run(OPENED, { type: "fieldChanged", field: "fontSize", value: "16" });
        expect(canSaveSettings(changed)).toBe(true);
        const back = reduceSettingsEditor(changed, { type: "fieldChanged", field: "fontSize", value: "14" });
        expect(canSaveSettings(back)).toBe(false);
    });

    test("a server change is flagged as needing a restart", () => {
        const s = run(OPENED, { type: "fieldChanged", field: "noBrowser", value: true });
        expect(serverSettingsChanged(s)).toBe(true);
        expect(autoResizeChanged(s)).toBe(false);
    });

    test("toggling Auto-Resize is flagged as needing a restart; a font change isn't", () => {
        expect(autoResizeChanged(run(OPENED, { type: "fieldChanged", field: "autoResize", value: false }))).toBe(true);
        const font = run(OPENED, { type: "fieldChanged", field: "fontFamily", value: "Menlo" });
        expect(autoResizeChanged(font)).toBe(false);
        expect(serverSettingsChanged(font)).toBe(false);
    });

    test("an invalid value disables OK and says why", () => {
        const s = run(OPENED, { type: "fieldChanged", field: "port", value: "70000" });
        expect(settingsProblem(s.form)).toBe("Port must be a whole number from 1 to 65535.");
        expect(canSaveSettings(s)).toBe(false);
        expect(reduceSettingsEditor(s, { type: "submitted" })).toBe(s);
    });

    test("switching tabs keeps the form", () => {
        const s = run(
            OPENED,
            { type: "fieldChanged", field: "port", value: "9090" },
            { type: "tabSelected", tab: "terminal" }
        );
        expect(s.tab).toBe("terminal");
        expect(s.form.port).toBe("9090");
    });

    test("submitting starts a save that blocks edits and a second submit", () => {
        const saving = run(OPENED, { type: "fieldChanged", field: "fontSize", value: "16" }, { type: "submitted" });
        expect(saving.phase).toBe("saving");
        expect(canSaveSettings(saving)).toBe(false);
        expect(reduceSettingsEditor(saving, { type: "submitted" })).toBe(saving);
        expect(reduceSettingsEditor(saving, { type: "fieldChanged", field: "fontSize", value: "18" })).toBe(saving);
    });

    test("a failed save returns to editing with the reason, and editing clears it", () => {
        const failed = run(
            OPENED,
            { type: "fieldChanged", field: "fontSize", value: "16" },
            { type: "submitted" },
            { type: "failed", message: "disk full" }
        );
        expect(failed.phase).toBe("editing");
        expect(failed.error).toBe("disk full");
        expect(canSaveSettings(failed)).toBe(true);
        expect(reduceSettingsEditor(failed, { type: "fieldChanged", field: "fontSize", value: "17" }).error).toBeNull();
    });

    test("a failure with no save in flight is ignored", () => {
        const s = run(OPENED);
        expect(reduceSettingsEditor(s, { type: "failed", message: "late" })).toBe(s);
    });

    test("opening again starts over from the new settings", () => {
        const s = run(
            OPENED,
            { type: "tabSelected", tab: "terminal" },
            { type: "fieldChanged", field: "fontSize", value: "16" },
            { type: "opened", settings: { ...SETTINGS, terminal: { ...SETTINGS.terminal, fontSize: 16 } } }
        );
        expect(s.tab).toBe("server");
        expect(s.form.fontSize).toBe("16");
        expect(settingsDirty(s)).toBe(false);
    });
});

describe("settingsProblem", () => {
    const form = run(OPENED).form;

    test("accepts the opened settings", () => {
        expect(settingsProblem(form)).toBeNull();
    });

    test("rejects blanks, decimals, and out-of-range values", () => {
        expect(settingsProblem({ ...form, port: "" })).not.toBeNull();
        expect(settingsProblem({ ...form, port: "0" })).not.toBeNull();
        expect(settingsProblem({ ...form, fontSize: "12.5" })).toBe("Font size must be a whole number of at least 1.");
        expect(settingsProblem({ ...form, rows: "9" })).toBe("Rows must be a whole number of at least 10.");
        expect(settingsProblem({ ...form, columns: "abc" })).toBe("Columns must be a whole number of at least 10.");
    });
});

describe("settingsFromForm", () => {
    test("converts the form back to settings, trimming the font family", () => {
        const form = { ...run(OPENED).form, fontFamily: "  Menlo ", rows: "30" };
        expect(settingsFromForm(form)).toEqual({
            server: SETTINGS.server,
            terminal: { ...SETTINGS.terminal, fontFamily: "Menlo", rows: 30 },
        });
    });
});
