import { describe, expect, test } from "bun:test";
import {
    canSubmitPrompt,
    initialPromptState,
    reducePrompt,
    showPromptProblem,
    type PromptAction,
    type PromptState,
} from "./prompt-layer-state.ts";

function run(...actions: PromptAction[]): PromptState {
    return actions.reduce(reducePrompt, initialPromptState);
}

const ASK_NAME: PromptAction = { type: "opened", hasInput: true, value: "midnight-copy", problem: null };
const CONFIRM: PromptAction = { type: "opened", hasInput: false, value: "", problem: null };

describe("reducePrompt", () => {
    test("starts closed", () => {
        expect(initialPromptState).toEqual({ phase: "closed" });
        expect(canSubmitPrompt(initialPromptState)).toBe(false);
    });

    test("a prompt with a usable default value can be submitted right away", () => {
        expect(canSubmitPrompt(run(ASK_NAME))).toBe(true);
        expect(canSubmitPrompt(run(CONFIRM))).toBe(true);
    });

    test("a problem blocks submitting, and shows only once something is typed", () => {
        const empty = run(ASK_NAME, { type: "valueChanged", value: "", problem: "Enter a name." });
        expect(canSubmitPrompt(empty)).toBe(false);
        expect(showPromptProblem(empty)).toBe(false);
        const taken = run(ASK_NAME, { type: "valueChanged", value: "dracula", problem: "Already exists." });
        expect(showPromptProblem(taken)).toBe(true);
        expect(reducePrompt(taken, { type: "submitted" })).toBe(taken);
    });

    test("submitting blocks edits, a second submit, and cancelling", () => {
        const submitting = run(ASK_NAME, { type: "submitted" });
        expect(submitting.phase).toBe("submitting");
        expect(canSubmitPrompt(submitting)).toBe(false);
        expect(reducePrompt(submitting, { type: "submitted" })).toBe(submitting);
        expect(reducePrompt(submitting, { type: "valueChanged", value: "x", problem: null })).toBe(submitting);
        expect(reducePrompt(submitting, { type: "cancelled" })).toBe(submitting);
    });

    test("a failed submit shows why and can be retried; typing clears the error", () => {
        const failed = run(ASK_NAME, { type: "submitted" }, { type: "failed", message: "already exists" });
        expect(failed).toMatchObject({ phase: "asking", error: "already exists" });
        expect(canSubmitPrompt(failed)).toBe(true);
        expect(reducePrompt(failed, { type: "valueChanged", value: "other", problem: null })).toMatchObject({
            error: null,
        });
    });

    test("a successful submit closes it", () => {
        expect(run(CONFIRM, { type: "submitted" }, { type: "succeeded" })).toEqual(initialPromptState);
    });

    test("cancelling closes it", () => {
        expect(run(ASK_NAME, { type: "cancelled" })).toEqual(initialPromptState);
    });

    test("late results when nothing is in flight are ignored", () => {
        const asking = run(ASK_NAME);
        expect(reducePrompt(asking, { type: "failed", message: "late" })).toBe(asking);
        expect(reducePrompt(asking, { type: "succeeded" })).toBe(asking);
        expect(reducePrompt(initialPromptState, { type: "cancelled" })).toBe(initialPromptState);
    });
});
