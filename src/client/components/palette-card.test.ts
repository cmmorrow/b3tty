import { describe, expect, test } from "bun:test";
import { contrastingTextColor } from "./palette-card.ts";

describe("contrastingTextColor", () => {
    test("picks white text on dark backgrounds", () => {
        expect(contrastingTextColor("#000000")).toBe("#ffffff");
        expect(contrastingTextColor("#282a36")).toBe("#ffffff");
        expect(contrastingTextColor("#0f1117")).toBe("#ffffff");
    });

    test("picks black text on light backgrounds", () => {
        expect(contrastingTextColor("#ffffff")).toBe("#000000");
        expect(contrastingTextColor("#fdf6e3")).toBe("#000000");
    });

    test("accepts #rgb shorthand, any case, and surrounding whitespace", () => {
        expect(contrastingTextColor("#FFF")).toBe("#000000");
        expect(contrastingTextColor(" #000 ")).toBe("#ffffff");
    });

    test("returns null for anything that isn't a hex color", () => {
        expect(contrastingTextColor("")).toBeNull();
        expect(contrastingTextColor("navy")).toBeNull();
        expect(contrastingTextColor("#12345")).toBeNull();
    });
});
