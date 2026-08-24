/**
 * Design system for b3tty web components.
 *
 * Each export is a CSS string fragment injected into a component's shadow DOM
 * <style> block. DESIGN_TOKENS defines CSS custom properties on :host so every
 * component shares the same palette, spacing, and typography scale.
 *
 * Usage:
 *   import { DESIGN_TOKENS, OVERLAY_STYLES, BUTTON_STYLES } from "./design.ts";
 *   style.textContent = `${DESIGN_TOKENS}${OVERLAY_STYLES}${BUTTON_STYLES}/* component-specific CSS *\/`;
 */

/** CSS custom properties shared across all b3tty web components. */
export const DESIGN_TOKENS = `
    :host {
        --color-surface-1: #e0e0e0;
        --color-surface-2: #c8c8c8;
        --color-surface-3: #f5f5f5;
        --color-surface-ro: #e8e8e8;
        --color-border: #aaa;
        --color-border-panel: #c0c0c0;
        --color-border-inner: #c8c8c8;
        --color-accent: #444;
        --color-accent-hover: #222;
        --color-text: #222;
        --color-text-subtle: #555;
        --color-muted: #666;
        --color-destructive: #c00;
        --color-destructive-bg: #f5d5d5;
        --color-destructive-border: #c44;
        --color-destructive-hover: #f0b8b8;
        --color-overlay: rgba(0,0,0,0.72);
        --color-shadow: rgba(0,0,0,0.55);
        --radius-sm: 3px;
        --radius-md: 5px;
        --radius-lg: 10px;
        --font-sm: 11px;
        --font-md: 13px;
        --font-lg: 14px;
        --transition: 0.15s;
    }
`;

/** OK, Cancel, and Delete button styles. */
export const BUTTON_STYLES = `
    .cancel-btn {
        padding: 8px 20px; border-radius: var(--radius-md);
        border: 1px solid var(--color-border); background: var(--color-surface-2);
        font-size: var(--font-lg); font-family: sans-serif; cursor: pointer;
    }
    .cancel-btn:hover { background: #b8b8b8; }
    .ok-btn {
        padding: 8px 28px; border-radius: var(--radius-md); border: none;
        background: var(--color-accent); color: #fff;
        font-size: var(--font-lg); font-family: sans-serif; cursor: pointer;
    }
    .ok-btn:disabled { background: var(--color-border); cursor: not-allowed; }
    .ok-btn:not(:disabled):hover { background: var(--color-accent-hover); }
    .delete-btn {
        margin-right: auto;
        padding: 8px 16px; border-radius: var(--radius-md);
        border: 1px solid var(--color-destructive-border); background: var(--color-destructive-bg);
        font-size: var(--font-lg); font-family: sans-serif; cursor: pointer;
        color: var(--color-destructive);
    }
    .delete-btn:hover { background: var(--color-destructive-hover); }
`;

/**
 * CSS custom property overrides applied to b3tty-palette-card elements
 * when embedded inside a left-panel list (theme editor, profile editor).
 */
export const PALETTE_CARD_VARS = `
    b3tty-palette-card {
        --palette-card-padding: 0;
        --palette-card-gap: 0;
        --palette-card-overflow: hidden;
        --palette-card-header-bg: #c8c8c8;
        --palette-card-header-padding: 8px 10px;
        --palette-card-header-font-size: 12px;
        --palette-card-terminal-gap: 6px;
        --palette-card-terminal-shadow: none;
        --palette-card-terminal-min-width: 0;
    }
`;

/**
 * Common overlay + modal base for full-screen dialog components.
 * Includes :host visibility toggling, .overlay backdrop, and .modal surface.
 * Each component adds its own flex-direction, padding, and dimensions to .modal.
 */
export const OVERLAY_STYLES = `
    :host { display: none; }
    :host([open]) { display: block; }
    .overlay {
        position: fixed; inset: 0;
        background: var(--color-overlay);
        z-index: 10000;
        display: flex; align-items: center; justify-content: center;
        padding: 20px; box-sizing: border-box;
    }
    .modal {
        background: var(--color-surface-1);
        border-radius: var(--radius-lg);
        box-sizing: border-box;
        box-shadow: 0 8px 40px var(--color-shadow);
    }
`;

/**
 * Selectable card styles for two-panel editor left panels (theme editor, profile editor).
 * Covers .create-card and .profile-card including hover, selected, and radio states.
 */
export const EDITOR_CARD_STYLES = `
    .create-card, .profile-card {
        display: flex; align-items: center; gap: 7px;
        padding: 8px 10px;
        background: var(--color-surface-2);
        border: 2px solid transparent;
        border-radius: 4px;
        cursor: pointer;
        font-family: sans-serif; font-size: var(--font-md); color: var(--color-text);
        user-select: none; flex-shrink: 0;
    }
    .create-card { font-weight: 600; }
    .create-card:hover, .profile-card:hover { background: #bbb; }
    .create-card[selected], .profile-card[selected] {
        border-color: var(--color-accent); background: #b8b8b8;
    }
    .create-card input[type=radio], .profile-card input[type=radio] {
        cursor: pointer; accent-color: var(--color-accent);
    }
`;

/**
 * Text, number, and color input field styles shared across editor components.
 * Classes: .name-input (theme editor), .field-input (profile editor),
 * .text-input / .number-input (settings editor), .color-input (theme editor).
 */
export const FORM_INPUT_STYLES = `
    .name-input, .field-input, .text-input {
        font-family: sans-serif; font-size: var(--font-md);
        padding: 4px 8px; border: 1px solid var(--color-border); border-radius: var(--radius-sm);
        background: var(--color-surface-3); box-sizing: border-box;
    }
    .name-input { flex: 1; }
    .text-input { flex: 1; max-width: 280px; }
    .name-input:read-only, .field-input:read-only {
        background: var(--color-surface-ro); color: var(--color-text-subtle);
    }
    .color-input {
        font-family: monospace; font-size: 12px;
        padding: 3px 6px; border: 1px solid var(--color-border); border-radius: var(--radius-sm);
        background: var(--color-surface-3); width: 100%; box-sizing: border-box; min-width: 0;
    }
    .number-input {
        width: 90px;
        font-family: sans-serif; font-size: var(--font-md);
        padding: 5px 8px; border: 1px solid var(--color-border); border-radius: var(--radius-sm);
        background: var(--color-surface-3); box-sizing: border-box;
    }
    .number-input:disabled { opacity: 0.4; cursor: not-allowed; }
`;

/**
 * Two-panel layout for editor components (theme editor, profile editor).
 * Provides .left-panel (without width — set per component), .right-panel, and .actions bar.
 */
export const SPLIT_PANEL_STYLES = `
    .left-panel {
        flex-shrink: 0;
        display: flex; flex-direction: column; gap: 6px;
        overflow-y: auto; min-height: 0;
        padding-right: 10px;
        border-right: 1px solid var(--color-border-panel);
    }
    .right-panel {
        flex: 1; display: flex; flex-direction: column;
        padding-left: 16px; min-width: 0;
    }
    .actions {
        display: flex; justify-content: flex-end; gap: 10px;
        padding-top: 10px; flex-shrink: 0;
        border-top: 1px solid var(--color-border-inner); margin-top: 8px;
    }
`;
