/**
 * A small modal stacked above an editor's own overlay that asks for a value or a
 * confirmation. The theme and profile editors use it for Copy…, Delete…, and Close's
 * "Discard Unsaved Changes?". Its state lives in prompt-layer-state.ts.
 */
import {
    canSubmitPrompt,
    initialPromptState,
    reducePrompt,
    showPromptProblem,
    type PromptAction,
    type PromptState,
} from "./prompt-layer-state.ts";

/** Options for PromptLayer.ask(). */
export interface PromptOptions {
    title: string;
    message: Array<string | Node>;
    /** A text field to ask for a value; validate returns why a value is unusable, or null. */
    input?: { label: string; value: string; validate(value: string): string | null };
    actionLabel: string;
    /** Styles the action button as destructive (.delete-btn). */
    destructive?: boolean;
    /** Runs the action with the field's value; a thrown error is shown and the prompt stays open. */
    submit(value: string): Promise<void>;
    /** Where focus returns when the prompt closes. */
    opener: HTMLElement | null;
}

/** A small modal that asks for a value or a confirmation; see buildPromptLayer. */
export interface PromptLayer {
    /** Resolves true once submit succeeded, or false when the prompt was cancelled. */
    ask(options: PromptOptions): Promise<boolean>;
}

/**
 * Styles for buildPromptLayer. The host also needs OVERLAY_STYLES (.modal, .actions),
 * TEXT_STYLES, FORM_INPUT_STYLES, BUTTON_STYLES, and SAVE_ERROR_STYLES.
 */
export const PROMPT_STYLES = `
    .prompt-overlay {
        position: fixed; inset: 0; z-index: var(--z-overlay-stacked);
        display: none; align-items: center; justify-content: center;
        background: var(--color-overlay);
        padding: var(--space-2xl); box-sizing: border-box;
    }
    .prompt-overlay.open { display: flex; }
    /* .modal.prompt outranks the host's own .modal rule (the editors' is a
       fixed-height column), so the prompt sizes to its content like the import dialog. */
    .modal.prompt {
        width: min(460px, 100%); height: auto; overflow: visible; padding: var(--space-2xl);
        display: flex; flex-direction: column; gap: var(--space-lg);
    }
    .prompt-message {
        margin: 0; font-size: var(--font-size-md); line-height: var(--line-height-body);
        overflow-wrap: anywhere;
    }
    .prompt-field { display: flex; flex-direction: column; gap: var(--space-2xs); }
    .prompt-field[hidden] { display: none; }
    .prompt .field-input { width: 100%; }
    .prompt .delete-btn { margin-right: 0; }
`;

/**
 * Builds a prompt modal into a component's shadow root, stacked above the component's
 * own overlay: a title, a message, an optional text field with an inline validation
 * error, and an actions row of a .save-error, Cancel, and the action button. Enter in
 * the field submits; Escape cancels. Used by the theme and profile editors.
 */
export function buildPromptLayer(root: ShadowRoot): PromptLayer {
    const overlay = document.createElement("div");
    overlay.className = "prompt-overlay";
    const box = document.createElement("div");
    box.className = "modal prompt";
    box.setAttribute("role", "alertdialog");
    box.setAttribute("aria-modal", "true");
    const heading = document.createElement("h2");
    heading.className = "modal-title";
    const message = document.createElement("p");
    message.className = "prompt-message";

    const field = document.createElement("label");
    field.className = "prompt-field";
    const fieldLabel = document.createElement("span");
    fieldLabel.className = "field-label";
    const input = document.createElement("input");
    input.type = "text";
    input.className = "field-input";
    input.spellcheck = false;
    input.autocomplete = "off";
    const fieldError = document.createElement("span");
    fieldError.className = "field-error";
    field.append(fieldLabel, input, fieldError);

    const actions = document.createElement("div");
    actions.className = "actions";
    const saveError = document.createElement("span");
    saveError.className = "save-error";
    const cancelBtn = document.createElement("button");
    cancelBtn.className = "cancel-btn";
    cancelBtn.textContent = "Cancel";
    const actionBtn = document.createElement("button");
    actions.append(saveError, cancelBtn, actionBtn);

    box.append(heading, message, field, actions);
    overlay.appendChild(box);
    root.appendChild(overlay);

    let state: PromptState = initialPromptState;
    let current: { options: PromptOptions; resolve(ok: boolean): void } | null = null;

    const problemOf = (value: string): string | null => current?.options.input?.validate(value) ?? null;
    /** Brings the prompt's DOM in line with its state. */
    const render = (): void => {
        overlay.classList.toggle("open", state.phase !== "closed");
        if (state.phase === "closed") return;
        const submitting = state.phase === "submitting";
        if (input.value !== state.value) input.value = state.value;
        input.disabled = submitting;
        fieldError.textContent = state.problem ?? "";
        fieldError.classList.toggle("visible", showPromptProblem(state));
        saveError.textContent = state.error ?? "";
        saveError.classList.toggle("visible", state.error !== null);
        actionBtn.disabled = !canSubmitPrompt(state);
        cancelBtn.disabled = submitting;
    };
    const dispatch = (action: PromptAction): void => {
        state = reducePrompt(state, action);
        render();
    };
    /** Hands the result to ask()'s caller once the prompt has closed. */
    const settle = (ok: boolean): void => {
        const c = current;
        current = null;
        c?.options.opener?.focus();
        c?.resolve(ok);
    };
    const cancel = (): void => {
        const before = state;
        dispatch({ type: "cancelled" });
        if (state !== before) settle(false);
    };
    const submit = async (): Promise<void> => {
        const before = state;
        dispatch({ type: "submitted" });
        const options = current?.options;
        if (state === before || state.phase !== "submitting" || !options) return;
        try {
            await options.submit(state.value);
            dispatch({ type: "succeeded" });
            settle(true);
        } catch (err) {
            dispatch({ type: "failed", message: err instanceof Error ? err.message : String(err) });
        }
    };

    input.addEventListener("input", () =>
        dispatch({ type: "valueChanged", value: input.value, problem: problemOf(input.value) })
    );
    cancelBtn.addEventListener("click", cancel);
    actionBtn.addEventListener("click", () => void submit());
    box.addEventListener("keydown", (e) => {
        if (e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            cancel();
        } else if (e.key === "Enter" && e.target === input) {
            e.preventDefault();
            void submit();
        }
    });

    return {
        ask(options: PromptOptions): Promise<boolean> {
            if (current) settle(false);
            heading.textContent = options.title;
            message.replaceChildren(...options.message);
            field.hidden = !options.input;
            fieldLabel.textContent = options.input?.label ?? "";
            actionBtn.textContent = options.actionLabel;
            actionBtn.className = options.destructive ? "delete-btn" : "ok-btn";
            return new Promise((resolve) => {
                current = { options, resolve };
                const value = options.input?.value ?? "";
                dispatch({ type: "opened", hasInput: !!options.input, value, problem: problemOf(value) });
                if (options.input) {
                    input.focus();
                    input.select();
                } else {
                    // A confirmation starts on Cancel, so Enter doesn't confirm by accident.
                    cancelBtn.focus();
                }
            });
        },
    };
}

/**
 * Asks whether to discard the unsaved changes Close would lose: `names` are the saved
 * items with changes, shown in bold, and `newItem` (e.g. "a new theme") the unsaved
 * new one, if any. Resolves true when the user chose to discard them.
 */
export function askToDiscardChanges(
    prompt: PromptLayer,
    names: string[],
    newItem: string | null,
    opener: HTMLElement
): Promise<boolean> {
    const items: Array<string | Node> = names.map((name) => {
        const bold = document.createElement("b");
        bold.textContent = name;
        return bold;
    });
    if (newItem) items.push(newItem);
    const count = items.length;
    const listed: Array<string | Node> = [];
    items.forEach((item, i) => {
        if (i > 0) listed.push(i === count - 1 ? (count > 2 ? ", and " : " and ") : ", ");
        listed.push(item);
    });
    return prompt.ask({
        title: "Discard Unsaved Changes?",
        message: [
            "You have unsaved changes to ",
            ...listed,
            `. Closing the editor will discard ${count === 1 ? "them" : "all of them"}.`,
        ],
        actionLabel: "Discard Changes",
        destructive: true,
        opener,
        submit: async () => {},
    });
}
