/**
 * State for the editors' prompts (buildPromptLayer in prompt-layer.ts), kept free of
 * DOM access so it can be tested under bun test.
 * Validating the field is up to the caller, which passes the result in as `problem`,
 * since what counts as a usable value depends on the prompt.
 */
export type PromptState =
    | { phase: "closed" }
    | {
          phase: "asking" | "submitting";
          /** Whether the prompt has a text field, or is a plain confirmation. */
          hasInput: boolean;
          value: string;
          /** Why the field's value is unusable, or null. */
          problem: string | null;
          /** A failed submit, shown in the actions row. */
          error: string | null;
      };

export type PromptAction =
    | { type: "opened"; hasInput: boolean; value: string; problem: string | null }
    | { type: "valueChanged"; value: string; problem: string | null }
    | { type: "submitted" }
    | { type: "failed"; message: string }
    | { type: "succeeded" }
    | { type: "cancelled" };

export const initialPromptState: PromptState = { phase: "closed" };

/** Whether the action button can be clicked: asking, with a usable value. */
export function canSubmitPrompt(state: PromptState): boolean {
    return state.phase === "asking" && state.problem === null;
}

/** Whether the field's problem is shown: an empty field is unusable but not yet worth flagging. */
export function showPromptProblem(state: PromptState): boolean {
    return state.phase !== "closed" && state.problem !== null && state.value !== "";
}

/**
 * Returns the next state. An action that doesn't apply in the current state returns
 * that same object, so a caller can tell whether "submitted" started a submit.
 */
export function reducePrompt(state: PromptState, action: PromptAction): PromptState {
    switch (action.type) {
        case "opened":
            return {
                phase: "asking",
                hasInput: action.hasInput,
                value: action.value,
                problem: action.problem,
                error: null,
            };
        case "valueChanged":
            if (state.phase !== "asking") return state;
            return { ...state, value: action.value, problem: action.problem, error: null };
        case "submitted":
            if (!canSubmitPrompt(state) || state.phase !== "asking") return state;
            return { ...state, phase: "submitting", error: null };
        case "failed":
            if (state.phase !== "submitting") return state;
            return { ...state, phase: "asking", error: action.message };
        case "succeeded":
            return state.phase === "submitting" ? initialPromptState : state;
        case "cancelled":
            // A submit in flight can't be cancelled; it closes the prompt when it succeeds.
            return state.phase === "asking" ? initialPromptState : state;
    }
}
