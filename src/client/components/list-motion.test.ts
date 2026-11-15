import { describe, expect, test } from "bun:test";
import { FADE_MS, listChanges, SLIDE_MS, slideKeyframes } from "./list-motion.ts";

describe("listChanges", () => {
    test("reports added and removed names, in order", () => {
        expect(listChanges(["a", "b", "c"], ["b", "d", "c", "e"])).toEqual({ added: ["d", "e"], removed: ["a"] });
    });

    test("a reorder is neither an addition nor a removal", () => {
        expect(listChanges(["a", "b"], ["b", "a"])).toEqual({ added: [], removed: [] });
    });

    test("handles empty lists", () => {
        expect(listChanges([], ["a"])).toEqual({ added: ["a"], removed: [] });
        expect(listChanges(["a"], [])).toEqual({ added: [], removed: ["a"] });
    });
});

describe("slideKeyframes", () => {
    const split = SLIDE_MS / (SLIDE_MS + FADE_MS);

    test("an entrance slides open with the item hidden, then fades it in", () => {
        const [start, opened, end] = slideKeyframes(80, 6, "in");
        expect(start).toMatchObject({ offset: 0, height: "0px", marginTop: "-6px", opacity: 0 });
        expect(opened).toMatchObject({ offset: split, height: "80px", marginTop: "0px", opacity: 0 });
        expect(end).toMatchObject({ offset: 1, height: "80px", marginTop: "0px", opacity: 1 });
    });

    test("an exit fades the item out, then collapses it", () => {
        const [start, faded, end] = slideKeyframes(80, 6, "out");
        expect(start).toMatchObject({ offset: 0, height: "80px", opacity: 1 });
        expect(faded).toMatchObject({ offset: FADE_MS / (SLIDE_MS + FADE_MS), height: "80px", opacity: 0 });
        expect(end).toMatchObject({ offset: 1, height: "0px", marginTop: "-6px", opacity: 0 });
    });

    test("with no gap the margin stays at zero", () => {
        expect(slideKeyframes(80, 0, "in")[0]).toMatchObject({ marginTop: "0px" });
    });
});
