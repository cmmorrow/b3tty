import { describe, expect, it } from "bun:test";
import { SHADE_OPACITY, splitShadeRuns } from "./shade-swap";

describe("splitShadeRuns", () => {
    it("returns null for text without shade characters", () => {
        expect(splitShadeRuns("")).toBeNull();
        expect(splitShadeRuns("plain text █▀▄")).toBeNull();
    });

    it("gives each shade its opacity", () => {
        expect(SHADE_OPACITY["░"]).toBe("0.25");
        expect(SHADE_OPACITY["▒"]).toBe("0.5");
        expect(SHADE_OPACITY["▓"]).toBe("0.75");
        expect(splitShadeRuns("▒")).toEqual([{ text: "▒", opacity: "0.5" }]);
    });

    it("keeps a run of one shade together and splits where the shade changes", () => {
        expect(splitShadeRuns("░░░▓▓")).toEqual([
            { text: "░░░", opacity: "0.25" },
            { text: "▓▓", opacity: "0.75" },
        ]);
    });

    it("keeps the plain text around and between shade runs", () => {
        expect(splitShadeRuns("ab░░cd▒e")).toEqual([
            { text: "ab" },
            { text: "░░", opacity: "0.25" },
            { text: "cd" },
            { text: "▒", opacity: "0.5" },
            { text: "e" },
        ]);
    });

    it("leaves the full block alone", () => {
        expect(splitShadeRuns("█░█")).toEqual([{ text: "█" }, { text: "░", opacity: "0.25" }, { text: "█" }]);
    });

    it("loses no characters", () => {
        const text = "x░░y▒▒▒z▓ ░";
        expect(
            splitShadeRuns(text)!
                .map((p) => p.text)
                .join("")
        ).toBe(text);
    });
});
