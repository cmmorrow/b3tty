import { postSettings } from "../api.ts";
import type { SettingsConfig, SettingsServerConfig } from "../types.ts";
import {
    BASE_STYLES,
    BUTTON_STYLES,
    DESIGN_TOKENS,
    FORM_INPUT_STYLES,
    OVERLAY_STYLES,
    SAVE_ERROR_STYLES,
    TEXT_STYLES,
} from "../design.ts";
import { closeOverlay } from "./overlay.ts";
import {
    autoResizeChanged,
    canSaveSettings,
    initialSettingsEditorState,
    reduceSettingsEditor,
    serverSettingsChanged,
    settingsFromForm,
    settingsProblem,
    type SettingsEditorAction,
    type SettingsEditorState,
    type SettingsTab,
} from "./settings-editor-state.ts";

/**
 * Interface for the b3tty-settings-editor web component.
 */
export interface B3ttySettingsEditor {
    open(settings: SettingsConfig): void;
    close(): void;
}

/**
 * Returns true when el is a b3tty-settings-editor element.
 */
export function isB3ttySettingsEditor(el: Element): el is HTMLElement & B3ttySettingsEditor {
    return el.tagName.toLowerCase() === "b3tty-settings-editor";
}

if (typeof HTMLElement !== "undefined") {
    class B3ttySettingsEditorImpl extends HTMLElement implements B3ttySettingsEditor {
        #shadow: ShadowRoot;
        #state: SettingsEditorState = initialSettingsEditorState;

        // Server tab inputs
        #portInput: HTMLInputElement;
        #noAuthInput: HTMLInputElement;
        #noBrowserInput: HTMLInputElement;
        #showMenubarInput: HTMLSelectElement;

        // Terminal tab inputs
        #fontFamilyInput: HTMLInputElement;
        #fontSizeInput: HTMLInputElement;
        #ligaturesInput: HTMLInputElement;
        #autoResizeInput: HTMLInputElement;
        #rowsInput: HTMLInputElement;
        #columnsInput: HTMLInputElement;

        // Footer buttons
        #cancelBtn: HTMLButtonElement;
        #okBtn: HTMLButtonElement;
        #saveError: HTMLSpanElement;

        // Tab buttons
        #tabBtns: Map<string, HTMLButtonElement> = new Map();

        // Tab panels
        #panels: Map<string, HTMLDivElement> = new Map();

        // Restart notes (shown only when relevant fields are modified)
        #serverRestartNote: HTMLDivElement;
        #termRestartNote: HTMLDivElement;

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            const style = document.createElement("style");
            style.textContent = `
                ${DESIGN_TOKENS}
                ${BASE_STYLES}
                ${OVERLAY_STYLES}
                ${TEXT_STYLES}
                .modal {
                    display: flex; flex-direction: column;
                    width: min(820px, 100%); height: min(580px, 90vh);
                    overflow: hidden;
                }
                .tab-bar {
                    display: flex; flex-direction: row;
                    background: var(--color-surface-2);
                    border-bottom: 1px solid var(--color-border-panel);
                    flex-shrink: 0;
                }
                .tab-btn {
                    padding: var(--space-md) var(--space-2xl);
                    border: none; background: transparent;
                    font-size: var(--font-size-md); font-weight: var(--font-weight-medium);
                    cursor: pointer; color: var(--color-text-subtle);
                    border-bottom: 2px solid transparent;
                    transition: background var(--transition);
                }
                .tab-btn:hover { color: var(--color-text); background: var(--color-surface-hover); }
                .tab-btn.active { color: var(--color-text); border-bottom-color: var(--color-accent); background: var(--color-surface-1); }
                .tab-content {
                    flex: 1; overflow: hidden; min-height: 0;
                    display: flex; flex-direction: column;
                }
                .panel {
                    display: none; flex: 1; overflow-y: auto;
                    padding: var(--space-2xl); flex-direction: column; gap: var(--space-lg);
                    min-height: 0;
                }
                .panel.active { display: flex; }
                .field-group {
                    display: flex; flex-direction: column; gap: var(--space-2xs);
                }
                .field-row {
                    display: flex; align-items: center; gap: var(--space-md);
                }
                ${FORM_INPUT_STYLES}
                .toggle {
                    width: 36px; height: 20px;
                    appearance: none; -webkit-appearance: none;
                    background: var(--color-surface-hover); border-radius: var(--radius-full);
                    cursor: pointer; position: relative;
                    transition: background var(--transition);
                    flex-shrink: 0;
                }
                .toggle:checked { background: var(--color-accent); }
                .toggle::after {
                    content: ""; position: absolute;
                    width: 16px; height: 16px;
                    background: var(--color-on-accent); border-radius: var(--radius-full);
                    top: 2px; left: 2px;
                    transition: left var(--transition);
                    box-shadow: var(--shadow-knob);
                }
                .toggle:checked::after { left: 18px; }
                .restart-note { padding: var(--space-2xs) 0; margin-top: var(--space-2xs); }
                .section-divider {
                    height: 1px; background: var(--color-border-panel); margin: var(--space-2xs) 0;
                }
                .footer {
                    /* align-items: center: see OVERLAY_STYLES' .actions. */
                    display: flex; justify-content: flex-end; align-items: center; gap: var(--space-md);
                    padding: var(--space-lg) var(--space-2xl); border-top: 1px solid var(--color-border-panel);
                    flex-shrink: 0; background: var(--color-surface-1);
                }
                ${BUTTON_STYLES}
                ${SAVE_ERROR_STYLES}
            `;

            const overlay = document.createElement("div");
            overlay.className = "overlay";
            const modal = document.createElement("div");
            modal.className = "modal";
            modal.setAttribute("role", "dialog");
            modal.setAttribute("aria-modal", "true");

            // --- Tab bar ---
            const tabBar = document.createElement("div");
            tabBar.className = "tab-bar";
            for (const [id, label] of [
                ["server", "Server"],
                ["terminal", "Terminal"],
            ] as [string, string][]) {
                const btn = document.createElement("button");
                btn.className = "tab-btn";
                btn.textContent = label;
                btn.addEventListener("click", () => this.#dispatch({ type: "tabSelected", tab: id as SettingsTab }));
                this.#tabBtns.set(id, btn);
                tabBar.appendChild(btn);
            }

            // --- Tab content wrapper ---
            const tabContent = document.createElement("div");
            tabContent.className = "tab-content";

            // --- Server panel ---
            const serverPanel = document.createElement("div");
            serverPanel.className = "panel";
            this.#panels.set("server", serverPanel);

            this.#portInput = document.createElement("input");
            this.#portInput.type = "number";
            this.#portInput.className = "number-input";
            this.#portInput.min = "1";
            this.#portInput.max = "65535";
            serverPanel.appendChild(this.#makeSettingGroup("Port", "The TCP port b3tty listens on.", this.#portInput));

            this.#noAuthInput = document.createElement("input");
            this.#noAuthInput.type = "checkbox";
            this.#noAuthInput.className = "toggle";
            serverPanel.appendChild(
                this.#makeSettingGroup(
                    "Disable Authentication",
                    "When enabled, the API token check is skipped. Reduces security posture.",
                    this.#noAuthInput
                )
            );

            this.#noBrowserInput = document.createElement("input");
            this.#noBrowserInput.type = "checkbox";
            this.#noBrowserInput.className = "toggle";
            serverPanel.appendChild(
                this.#makeSettingGroup(
                    "Disable Auto-Open Browser",
                    "When enabled, b3tty will not automatically open a browser tab on startup.",
                    this.#noBrowserInput
                )
            );

            this.#showMenubarInput = document.createElement("select");
            this.#showMenubarInput.className = "select-input";
            for (const [value, label] of [
                ["hover", "Hover to show"],
                ["visible", "Always visible"],
                ["disable", "Disabled"],
            ] as [string, string][]) {
                const option = document.createElement("option");
                option.value = value;
                option.textContent = label;
                this.#showMenubarInput.appendChild(option);
            }
            serverPanel.appendChild(
                this.#makeSettingGroup(
                    "Menu Bar Visibility",
                    "Controls whether the menu bar shows on hover, stays always visible, or is disabled entirely. Requires restart.",
                    this.#showMenubarInput
                )
            );

            this.#serverRestartNote = document.createElement("div");
            this.#serverRestartNote.className = "restart-note field-error";
            this.#serverRestartNote.textContent = "⚠ Server settings will not take effect until b3tty is restarted.";
            serverPanel.appendChild(this.#serverRestartNote);

            // --- Terminal panel ---
            const terminalPanel = document.createElement("div");
            terminalPanel.className = "panel";
            this.#panels.set("terminal", terminalPanel);

            this.#fontFamilyInput = document.createElement("input");
            this.#fontFamilyInput.type = "text";
            this.#fontFamilyInput.className = "text-input";
            this.#fontFamilyInput.placeholder = "e.g. Fira Code, JetBrains Mono";
            terminalPanel.appendChild(
                this.#makeSettingGroup(
                    "Font Family",
                    "Primary font for the terminal. Falls back to Menlo, monospace if unavailable.",
                    this.#fontFamilyInput
                )
            );

            this.#fontSizeInput = document.createElement("input");
            this.#fontSizeInput.type = "number";
            this.#fontSizeInput.className = "number-input";
            this.#fontSizeInput.min = "6";
            this.#fontSizeInput.max = "72";
            terminalPanel.appendChild(
                this.#makeSettingGroup(
                    "Font Size (px)",
                    "Terminal font size in pixels. Applied immediately.",
                    this.#fontSizeInput
                )
            );

            this.#ligaturesInput = document.createElement("input");
            this.#ligaturesInput.type = "checkbox";
            this.#ligaturesInput.className = "toggle";
            terminalPanel.appendChild(
                this.#makeSettingGroup(
                    "Ligatures",
                    "Show the font's ligatures, such as → for ->, when it has them. Applied immediately.",
                    this.#ligaturesInput
                )
            );

            const divider = document.createElement("div");
            divider.className = "section-divider";
            terminalPanel.appendChild(divider);

            this.#autoResizeInput = document.createElement("input");
            this.#autoResizeInput.type = "checkbox";
            this.#autoResizeInput.className = "toggle";
            terminalPanel.appendChild(
                this.#makeSettingGroup(
                    "Auto-Resize",
                    "When enabled, the terminal resizes to fill the browser window. When disabled, Rows and Columns below set the fixed dimensions. Requires restart.",
                    this.#autoResizeInput
                )
            );

            this.#rowsInput = document.createElement("input");
            this.#rowsInput.type = "number";
            this.#rowsInput.className = "number-input";
            this.#rowsInput.min = "10";
            terminalPanel.appendChild(
                this.#makeSettingGroup(
                    "Rows",
                    "Fixed terminal height in rows (minimum 10). Used only when Auto-Resize is disabled. Requires restart.",
                    this.#rowsInput
                )
            );

            this.#columnsInput = document.createElement("input");
            this.#columnsInput.type = "number";
            this.#columnsInput.className = "number-input";
            this.#columnsInput.min = "10";
            terminalPanel.appendChild(
                this.#makeSettingGroup(
                    "Columns",
                    "Fixed terminal width in columns (minimum 10). Used only when Auto-Resize is disabled. Requires restart.",
                    this.#columnsInput
                )
            );

            this.#termRestartNote = document.createElement("div");
            this.#termRestartNote.className = "restart-note field-error";
            this.#termRestartNote.textContent = "⚠ Changing Auto-Resize requires a restart to take effect.";
            terminalPanel.appendChild(this.#termRestartNote);

            tabContent.appendChild(serverPanel);
            tabContent.appendChild(terminalPanel);

            // --- Footer ---
            const footer = document.createElement("div");
            footer.className = "footer";
            this.#saveError = document.createElement("span");
            this.#saveError.className = "save-error";
            this.#cancelBtn = document.createElement("button");
            this.#cancelBtn.className = "cancel-btn";
            this.#cancelBtn.textContent = "Cancel";
            this.#okBtn = document.createElement("button");
            this.#okBtn.className = "ok-btn";
            this.#okBtn.textContent = "OK";
            footer.appendChild(this.#saveError);
            footer.appendChild(this.#cancelBtn);
            footer.appendChild(this.#okBtn);

            modal.appendChild(tabBar);
            modal.appendChild(tabContent);
            modal.appendChild(footer);
            overlay.appendChild(modal);
            this.#shadow.appendChild(style);
            this.#shadow.appendChild(overlay);

            this.#cancelBtn.addEventListener("click", () => this.close());
            this.#okBtn.addEventListener("click", () => void this.#handleSettingsOk());

            const onInput = (el: HTMLInputElement, change: (value: string) => SettingsEditorAction): void =>
                el.addEventListener("input", () => this.#dispatch(change(el.value)));
            const onToggle = (el: HTMLInputElement, change: (checked: boolean) => SettingsEditorAction): void =>
                el.addEventListener("change", () => this.#dispatch(change(el.checked)));
            onInput(this.#portInput, (value) => ({ type: "fieldChanged", field: "port", value }));
            onInput(this.#fontFamilyInput, (value) => ({ type: "fieldChanged", field: "fontFamily", value }));
            onInput(this.#fontSizeInput, (value) => ({ type: "fieldChanged", field: "fontSize", value }));
            onInput(this.#rowsInput, (value) => ({ type: "fieldChanged", field: "rows", value }));
            onInput(this.#columnsInput, (value) => ({ type: "fieldChanged", field: "columns", value }));
            onToggle(this.#noAuthInput, (value) => ({ type: "fieldChanged", field: "noAuth", value }));
            onToggle(this.#noBrowserInput, (value) => ({ type: "fieldChanged", field: "noBrowser", value }));
            onToggle(this.#ligaturesInput, (value) => ({ type: "fieldChanged", field: "ligatures", value }));
            onToggle(this.#autoResizeInput, (value) => ({ type: "fieldChanged", field: "autoResize", value }));
            this.#showMenubarInput.addEventListener("change", () =>
                this.#dispatch({
                    type: "fieldChanged",
                    field: "showMenubar",
                    value: this.#showMenubarInput.value as SettingsServerConfig["showMenubar"],
                })
            );
            this.#render();
        }

        #makeSettingGroup(label: string, desc: string, input: HTMLElement): HTMLDivElement {
            const group = document.createElement("div");
            group.className = "field-group";
            const lbl = document.createElement("div");
            lbl.className = "field-label";
            lbl.textContent = label;
            const row = document.createElement("div");
            row.className = "field-row";
            row.appendChild(input);
            const descEl = document.createElement("div");
            descEl.className = "field-desc";
            descEl.textContent = desc;
            group.appendChild(lbl);
            group.appendChild(row);
            group.appendChild(descEl);
            return group;
        }

        #dispatch(action: SettingsEditorAction): void {
            this.#state = reduceSettingsEditor(this.#state, action);
            this.#render();
        }

        /** Brings the DOM in line with #state. Safe to call at any time. */
        #render(): void {
            const state = this.#state;
            const { form } = state;
            const saving = state.phase === "saving";

            for (const [id, btn] of this.#tabBtns) btn.classList.toggle("active", id === state.tab);
            for (const [id, panel] of this.#panels) panel.classList.toggle("active", id === state.tab);

            // Values are set only when they differ, so typing isn't disturbed.
            const text: [HTMLInputElement | HTMLSelectElement, string][] = [
                [this.#portInput, form.port],
                [this.#showMenubarInput, form.showMenubar],
                [this.#fontFamilyInput, form.fontFamily],
                [this.#fontSizeInput, form.fontSize],
                [this.#rowsInput, form.rows],
                [this.#columnsInput, form.columns],
            ];
            for (const [el, value] of text) {
                if (el.value !== value) el.value = value;
                el.disabled = saving;
            }
            const toggles: [HTMLInputElement, boolean][] = [
                [this.#noAuthInput, form.noAuth],
                [this.#noBrowserInput, form.noBrowser],
                [this.#ligaturesInput, form.ligatures],
                [this.#autoResizeInput, form.autoResize],
            ];
            for (const [el, checked] of toggles) {
                el.checked = checked;
                el.disabled = saving;
            }
            // Rows and columns only apply while Auto-Resize is off.
            this.#rowsInput.disabled = saving || form.autoResize;
            this.#columnsInput.disabled = saving || form.autoResize;

            this.#serverRestartNote.classList.toggle("visible", serverSettingsChanged(state));
            this.#termRestartNote.classList.toggle("visible", autoResizeChanged(state));

            const message = state.error ?? settingsProblem(form);
            this.#saveError.textContent = message ?? "";
            this.#saveError.classList.toggle("visible", message !== null);
            // Cancel too, so the editor can't close and reopen under a request in flight.
            this.#cancelBtn.disabled = saving;
            this.#okBtn.disabled = !canSaveSettings(state);
        }

        async #handleSettingsOk(): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "submitted" });
            if (this.#state === before) return;
            try {
                const response = await postSettings(settingsFromForm(this.#state.form));
                this.dispatchEvent(
                    new CustomEvent("b3tty-settings-edited", {
                        detail: { response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.close();
            } catch (err) {
                // Keep modal open for retry, but say why the save didn't
                // visibly do anything.
                const message = err instanceof Error ? err.message : "Failed to save settings.";
                this.#dispatch({ type: "failed", message });
            }
        }

        open(settings: SettingsConfig): void {
            this.#dispatch({ type: "opened", settings });
            this.setAttribute("open", "");
        }

        close(): void {
            closeOverlay(this);
        }
    }

    customElements.define("b3tty-settings-editor", B3ttySettingsEditorImpl);
}
