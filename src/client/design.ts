/**
 * Design system for b3tty web components.
 *
 * Each export is a CSS string fragment injected into a component's shadow DOM
 * <style> block. DESIGN_TOKENS defines CSS custom properties on :host so every
 * component shares the same colors, typography, spacing, radius, shadow,
 * z-index, and motion scales. BASE_STYLES applies the shared font and form
 * control resets; the remaining fragments are reusable pieces of UI.
 *
 * Components should reference tokens (`var(--space-sm)`) rather than raw
 * values. The only literals left in component CSS should be layout dimensions
 * (widths, heights, grid columns) and palette-preview data colors.
 *
 * Theming: because tokens are declared on :host, any style targeting the host
 * element from outside the shadow root (a page stylesheet or an inline
 * `style.setProperty`) overrides them. Override the semantic tokens
 * (`--color-*`, `--shadow-*`) to theme a component without touching its CSS.
 *
 * Usage:
 *   import { DESIGN_TOKENS, BASE_STYLES, OVERLAY_STYLES, BUTTON_STYLES } from "./design.ts";
 *   style.textContent = `${DESIGN_TOKENS}${BASE_STYLES}${OVERLAY_STYLES}${BUTTON_STYLES}/* component-specific CSS *\/`;
 */

/** CSS custom properties shared across all b3tty web components. */
export const DESIGN_TOKENS = `
    :host {
        /* Color: surfaces (warm ivory) */
        --color-surface-1: #fbf9f3;
        --color-surface-2: #f1ede3;
        --color-surface-3: #fffefa;
        --color-surface-ro: #f4f0e7;
        --color-surface-card: #f1ede3;
        --color-surface-hover: #e8e2d5;
        --color-surface-selected: #e2dbcc;
        /* Color: borders. --color-border outlines controls (inputs, Cancel), so it
           keeps 3:1 against every surface it sits on; the panel/inner borders are
           decorative dividers. */
        --color-border: #8f8572;
        --color-border-strong: #6b6355;
        --color-border-panel: #e6e0d2;
        --color-border-inner: #e6e0d2;
        --color-border-subtle: rgba(60,45,20,0.15);
        /* Color: accent (navy) */
        --color-accent: #0b1a5c;
        --color-accent-hover: #1c2d78;
        --color-on-accent: #fffdf7;
        /* Color: text */
        --color-text: #2a2723;
        --color-text-subtle: #575045;
        --color-muted: #6b6355;
        --color-text-label: #48423a;
        --color-text-faint: #787061;
        /* Color: destructive. --color-destructive is error text; the solid pair
           fills every Delete button. */
        --color-destructive: #8a2216;
        --color-destructive-solid: #a3271c;
        --color-destructive-solid-hover: #8a2216;
        --color-on-destructive: #fff;
        /* Color: warning */
        --color-warning: #7a4f00;
        --color-warning-bg: #fbecc4;
        --color-warning-border: #d9a83a;
        /* Color: misc */
        --color-overlay: rgba(0,0,0,0.72);
        --color-handle: #8f8572;
        --color-focus-ring: #0b1a5c;

        /* Typography */
        --font-family-ui: system-ui, -apple-system, "Segoe UI", sans-serif;
        --font-family-mono: monospace;
        --font-size-xs: 11px;
        --font-size-sm: 12px;
        --font-size-md: 13px;
        --font-size-lg: 14px;
        --font-size-xl: 16px;
        --font-weight-regular: 400;
        --font-weight-medium: 500;
        --font-weight-semibold: 600;
        --font-weight-bold: 700;
        --line-height-body: 1.5;
        --letter-spacing-caps: 0.05em;

        /* Spacing */
        --space-3xs: 2px;
        --space-2xs: 4px;
        --space-xs: 6px;
        --space-sm: 8px;
        --space-md: 10px;
        --space-lg: 12px;
        --space-xl: 16px;
        --space-2xl: 20px;
        --space-3xl: 28px;

        /* Radius */
        --radius-sm: 4px;
        --radius-md: 6px;
        --radius-lg: 8px;
        --radius-xl: 10px;
        --radius-full: 999px;

        /* Shadows */
        --shadow-modal: 0 8px 40px rgba(0,0,0,0.55);
        --shadow-dropdown: 0 4px 12px rgba(0,0,0,0.3);
        --shadow-card: 0 2px 10px rgba(0,0,0,0.35);
        --shadow-knob: 0 1px 3px rgba(0,0,0,0.3);

        /* Z-index */
        --z-menubar: 1000;
        --z-dropdown: 1001;
        --z-overlay: 10000;
        /* Dialogs that open on top of another overlay (theme import/export over the theme editor). */
        --z-overlay-stacked: 10001;

        /* Motion & state */
        --transition: 0.15s;
        --opacity-disabled: 0.4;

        /* Layout sizes */
        --menubar-height: 32px;
    }
`;

/**
 * Base styles every component includes right after DESIGN_TOKENS: the UI font
 * and text color on :host (inherited by everything inside the shadow root),
 * font inheritance for form controls (which don't inherit by default), the
 * shared radio-button look, and the keyboard focus ring.
 */
export const BASE_STYLES = `
    :host { font-family: var(--font-family-ui); color: var(--color-text); }
    button, input, select, textarea { font-family: inherit; }
    input[type=radio] { cursor: pointer; accent-color: var(--color-accent); }
    :focus-visible { outline: 2px solid var(--color-focus-ring); outline-offset: 2px; }
`;

/**
 * Shared text roles. Elements opt in by adding the class alongside any
 * component-specific class name.
 *   .modal-title   — dialog heading
 *   .field-label   — form field label, the same in every form
 *   .section-title — uppercase group heading within a form
 *   .field-desc    — secondary help text under a field
 *   .field-error   — inline validation/save error, hidden until `.visible`
 *   .warning-note  — boxed caution message (e.g. unsaved changes), hidden until `.visible`
 *   .badge         — small pill flagging a state (e.g. an installed theme's "Modified")
 */
export const TEXT_STYLES = `
    .modal-title {
        margin: 0; font-size: var(--font-size-xl); font-weight: var(--font-weight-semibold);
        color: var(--color-text);
    }
    .field-label { font-size: var(--font-size-sm); color: var(--color-text-label); }
    .section-title {
        font-size: var(--font-size-xs); font-weight: var(--font-weight-semibold);
        text-transform: uppercase; letter-spacing: var(--letter-spacing-caps);
        color: var(--color-muted);
    }
    .field-desc {
        font-size: var(--font-size-xs); color: var(--color-muted);
        line-height: var(--line-height-body);
    }
    .field-error {
        display: none;
        font-size: var(--font-size-xs); color: var(--color-destructive);
    }
    .field-error.visible { display: block; }
    .warning-note {
        display: none;
        padding: var(--space-xs) var(--space-sm);
        border: 1px solid var(--color-warning-border); border-radius: var(--radius-sm);
        background: var(--color-warning-bg); color: var(--color-warning);
        font-size: var(--font-size-sm); line-height: var(--line-height-body);
    }
    .warning-note.visible { display: block; }
    /* A small pill flagging a state, e.g. an installed theme's "Modified". */
    .badge {
        display: inline-block; padding: 0 var(--space-2xs);
        border: 1px solid var(--color-warning-border); border-radius: var(--radius-full);
        background: var(--color-warning-bg); color: var(--color-warning);
        font-size: var(--font-size-xs); font-weight: var(--font-weight-semibold);
        line-height: var(--line-height-body); white-space: nowrap;
    }
`;

/** OK, Cancel, and Delete button styles. */
export const BUTTON_STYLES = `
    .ok-btn, .cancel-btn, .delete-btn {
        padding: var(--space-sm) var(--space-2xl); border-radius: var(--radius-md);
        font-size: var(--font-size-lg); cursor: pointer;
        transition: background var(--transition);
    }
    .cancel-btn {
        border: 1px solid var(--color-border); background: var(--color-surface-3);
        color: var(--color-text);
    }
    .cancel-btn:hover { background: var(--color-surface-2); border-color: var(--color-border-strong); }
    .ok-btn {
        padding-left: var(--space-3xl); padding-right: var(--space-3xl);
        border: none; background: var(--color-accent); color: var(--color-on-accent);
    }
    .ok-btn:disabled { opacity: var(--opacity-disabled); cursor: not-allowed; }
    .ok-btn:not(:disabled):hover { background: var(--color-accent-hover); }
    .delete-btn {
        margin-right: auto;
        padding-left: var(--space-xl); padding-right: var(--space-xl);
        border: 1px solid var(--color-destructive-solid); background: var(--color-destructive-solid);
        color: var(--color-on-destructive);
    }
    .delete-btn:hover {
        border-color: var(--color-destructive-solid-hover); background: var(--color-destructive-solid-hover);
    }
    /* Compact secondary button for use inline with form fields. */
    .inline-btn {
        padding: var(--space-xs) var(--space-md); border-radius: var(--radius-md);
        font-size: var(--font-size-md); white-space: nowrap; cursor: pointer;
        border: 1px solid var(--color-border); background: var(--color-surface-3);
        color: var(--color-text);
        transition: background var(--transition);
    }
    .inline-btn:not(:disabled):hover { background: var(--color-surface-2); border-color: var(--color-border-strong); }
    .inline-btn:disabled { opacity: var(--opacity-disabled); cursor: not-allowed; }
`;

/**
 * CSS custom property overrides applied to b3tty-palette-card elements
 * when embedded inside a left-panel list (theme editor, theme picker).
 */
export const PALETTE_CARD_VARS = `
    b3tty-palette-card {
        --palette-card-padding: 0;
        --palette-card-gap: 0;
        --palette-card-overflow: hidden;
        --palette-card-header-bg: var(--color-surface-2);
        --palette-card-header-padding: var(--space-sm) var(--space-md);
        --palette-card-header-font-size: var(--font-size-sm);
        --palette-card-terminal-gap: var(--space-xs);
        --palette-card-terminal-shadow: none;
        --palette-card-terminal-min-width: 0;
    }
`;

/**
 * Common overlay + modal base for full-screen dialog components.
 * Includes :host visibility toggling, .overlay backdrop, .modal surface, and a
 * base right-aligned .actions button row.
 * Each component adds its own flex-direction, padding, and dimensions to .modal.
 */
export const OVERLAY_STYLES = `
    :host { display: none; }
    :host([open]) { display: block; }
    .overlay {
        position: fixed; inset: 0;
        background: var(--color-overlay);
        z-index: var(--z-overlay);
        display: flex; align-items: center; justify-content: center;
        padding: var(--space-2xl); box-sizing: border-box;
    }
    .modal {
        background: var(--color-surface-1);
        border-radius: var(--radius-xl);
        box-sizing: border-box;
        box-shadow: var(--shadow-modal);
    }
    /* align-items: center keeps the buttons at their natural height when a
       wrapped .save-error message makes the row taller; the default stretch
       would grow them to match it. */
    .actions { display: flex; justify-content: flex-end; align-items: center; gap: var(--space-md); }
`;

/**
 * Selectable card styles for two-panel editor left panels (theme editor, profile editor).
 * Covers .create-card and .profile-card including hover and selected states, and
 * .action-card: a dashed-border card that runs an action (e.g. "Import theme…")
 * rather than selecting an item, so it has no selected state. Use it on a
 * <button>; an optional leading .action-card-icon holds a glyph.
 */
export const EDITOR_CARD_STYLES = `
    .create-card, .profile-card {
        display: flex; align-items: center; gap: var(--space-xs);
        padding: var(--space-sm) var(--space-md);
        background: var(--color-surface-2);
        border: 2px solid transparent;
        border-radius: var(--radius-sm);
        cursor: pointer;
        font-size: var(--font-size-md); color: var(--color-text);
        user-select: none; flex-shrink: 0;
    }
    .create-card { font-weight: var(--font-weight-semibold); }
    .create-card:hover, .profile-card:hover { background: var(--color-surface-hover); }
    .create-card[selected], .profile-card[selected] {
        border-color: var(--color-accent); background: var(--color-surface-selected);
    }
    .action-card {
        display: flex; align-items: center; gap: var(--space-xs);
        padding: var(--space-sm) var(--space-md);
        background: transparent;
        border: 2px dashed var(--color-border);
        border-radius: var(--radius-sm);
        cursor: pointer; text-align: left;
        font-size: var(--font-size-md); font-weight: var(--font-weight-semibold);
        color: var(--color-text);
        user-select: none; flex-shrink: 0;
        transition: background var(--transition);
    }
    .action-card:hover { background: var(--color-surface-hover); }
    .action-card-icon { width: 16px; text-align: center; }
`;

/**
 * Text, number, select, and color input field styles shared across editor components.
 * Classes: .name-input (theme editor), .field-input (profile editor),
 * .text-input / .number-input / .select-input (settings editor), .color-input (theme editor).
 */
export const FORM_INPUT_STYLES = `
    .name-input, .field-input, .text-input, .number-input, .select-input, .color-input {
        font-size: var(--font-size-md);
        padding: var(--space-xs) var(--space-sm);
        border: 1px solid var(--color-border); border-radius: var(--radius-sm);
        background: var(--color-surface-3); color: var(--color-text);
        box-sizing: border-box;
    }
    .name-input { flex: 1; }
    .text-input { flex: 1; max-width: 280px; }
    .number-input { width: 90px; }
    .name-input:read-only, .field-input:read-only {
        background: var(--color-surface-ro); color: var(--color-text-subtle);
    }
    .color-input {
        font-family: var(--font-family-mono); font-size: var(--font-size-sm);
        padding: var(--space-2xs) var(--space-xs);
        width: 100%; min-width: 0;
    }
    .number-input:disabled { opacity: var(--opacity-disabled); cursor: not-allowed; }
`;

/**
 * Inline error message shown in an editor's actions bar when a save/delete
 * request fails, so failures aren't silently swallowed. Hidden by default;
 * toggle the `visible` class to show it. Left-aligned within `.actions`
 * (which uses `justify-content: flex-end`) via `margin-right: auto`,
 * matching how `.delete-btn` claims the same position.
 */
export const SAVE_ERROR_STYLES = `
    .save-error {
        margin-right: auto;
        align-self: center;
        display: none;
        font-size: var(--font-size-xs); color: var(--color-destructive);
        max-width: 60%;
        /* Messages can quote file paths, which have no spaces to wrap at. */
        overflow-wrap: anywhere;
    }
    .save-error.visible { display: block; }
`;

/**
 * Two-panel layout for editor components (theme editor, profile editor).
 * Provides .left-panel (without width — set per component), .right-panel, and
 * extends OVERLAY_STYLES' .actions bar with a top divider.
 */
export const SPLIT_PANEL_STYLES = `
    .left-panel {
        flex-shrink: 0;
        display: flex; flex-direction: column; gap: var(--space-xs);
        overflow-y: auto; min-height: 0;
        padding-right: var(--space-md);
        border-right: 1px solid var(--color-border-panel);
    }
    .right-panel {
        flex: 1; display: flex; flex-direction: column;
        padding-left: var(--space-xl); min-width: 0;
    }
    .actions {
        padding-top: var(--space-md); flex-shrink: 0;
        border-top: 1px solid var(--color-border-inner); margin-top: var(--space-sm);
    }
`;
