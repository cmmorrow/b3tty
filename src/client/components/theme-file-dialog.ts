import {
    BASE_STYLES,
    BUTTON_STYLES,
    DESIGN_TOKENS,
    FORM_INPUT_STYLES,
    OVERLAY_STYLES,
    SAVE_ERROR_STYLES,
    TEXT_STYLES,
} from "../design.ts";

export interface ThemeFileDialogParts {
    intro: HTMLParagraphElement;
    warning: HTMLDivElement;
    input: HTMLInputElement;
    hint: HTMLSpanElement;
    actions: HTMLDivElement;
    saveError: HTMLSpanElement;
    cancelBtn: HTMLButtonElement;
    actionBtn: HTMLButtonElement;
}

/**
 * Builds the shared layout of the theme import and export dialogs into host's shadow
 * root: title, intro, a hidden .warning-note, a File Path field, and an actions row of
 * .save-error, Cancel, and the action button. Opens above other overlays
 * (--z-overlay-stacked) since both can be opened from the theme editor. Cancel calls
 * host.close(); Enter in the field clicks the action button when it is enabled.
 */
export function buildThemeFileDialog(
    host: HTMLElement & { close(): void },
    title: string,
    actionLabel: string
): ThemeFileDialogParts {
    const shadow = host.attachShadow({ mode: "open" });

    const style = document.createElement("style");
    style.textContent = `
        ${DESIGN_TOKENS}
        ${BASE_STYLES}
        ${OVERLAY_STYLES}
        ${TEXT_STYLES}
        ${FORM_INPUT_STYLES}
        ${BUTTON_STYLES}
        ${SAVE_ERROR_STYLES}
        .overlay { z-index: var(--z-overlay-stacked); }
        .modal {
            width: min(460px, 100%); padding: var(--space-2xl);
            display: flex; flex-direction: column; gap: var(--space-lg);
        }
        .intro { margin: calc(-1 * var(--space-sm)) 0 0; font-size: var(--font-size-sm); }
        .intro b { color: var(--color-text); }
        .field { display: flex; flex-direction: column; gap: var(--space-2xs); }
        .field-input { width: 100%; font-family: var(--font-family-mono); font-size: var(--font-size-sm); }
        .field-input:disabled { opacity: var(--opacity-disabled); }
        .actions { margin-top: var(--space-xs); }
    `;

    const overlay = document.createElement("div");
    overlay.className = "overlay";
    const modal = document.createElement("div");
    modal.className = "modal";
    modal.setAttribute("role", "dialog");
    modal.setAttribute("aria-modal", "true");

    const heading = document.createElement("h2");
    heading.className = "modal-title";
    heading.textContent = title;
    const intro = document.createElement("p");
    intro.className = "intro field-desc";
    const warning = document.createElement("div");
    warning.className = "warning-note";
    warning.setAttribute("role", "status");

    const field = document.createElement("label");
    field.className = "field";
    const label = document.createElement("span");
    label.className = "field-label";
    label.textContent = "File Path";
    const input = document.createElement("input");
    input.type = "text";
    input.className = "field-input";
    input.spellcheck = false;
    input.autocomplete = "off";
    const hint = document.createElement("span");
    hint.className = "field-desc";
    field.append(label, input, hint);

    const actions = document.createElement("div");
    actions.className = "actions";
    const saveError = document.createElement("span");
    saveError.className = "save-error";
    const cancelBtn = document.createElement("button");
    cancelBtn.className = "cancel-btn";
    cancelBtn.textContent = "Cancel";
    cancelBtn.addEventListener("click", () => host.close());
    const actionBtn = document.createElement("button");
    actionBtn.className = "ok-btn";
    actionBtn.textContent = actionLabel;
    actions.append(saveError, cancelBtn, actionBtn);

    input.addEventListener("input", () => {
        actionBtn.disabled = input.disabled || !input.value.trim();
        saveError.classList.remove("visible");
    });
    input.addEventListener("keydown", (e) => {
        if (e.key === "Enter" && !actionBtn.disabled) actionBtn.click();
    });

    modal.append(heading, intro, warning, field, actions);
    overlay.appendChild(modal);
    shadow.append(style, overlay);
    return { intro, warning, input, hint, actions, saveError, cancelBtn, actionBtn };
}

/** Focuses the path field with the caret at the end. Call after setting [open]. */
export function focusPathInput(input: HTMLInputElement): void {
    input.focus();
    input.setSelectionRange(input.value.length, input.value.length);
}
