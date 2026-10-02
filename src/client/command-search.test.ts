import { describe, it, expect } from "bun:test";
import { matchLabel, searchCommands, splitLabel } from "./command-search.ts";
import type { PaletteCommand } from "./types.ts";

const COMMANDS: PaletteCommand[] = [
    { id: "select-theme", label: "Select Theme…", group: "Themes" },
    { id: "edit-theme", label: "Edit Theme…", group: "Themes" },
    { id: "theme:tokyo-night", label: "Switch Theme: tokyo-night", group: "Themes" },
    { id: "edit-profile", label: "Edit Profile…", group: "Profiles" },
    { id: "profile:api-server", label: "Open Profile: api-server", group: "Profiles" },
    { id: "settings", label: "Settings…", group: "b3tty" },
    { id: "about", label: "About b3tty…", group: "b3tty" },
];

const ids = (query: string, recent: string[] = []) => searchCommands(COMMANDS, query, recent).map((m) => m.command.id);

describe("matchLabel", () => {
    it("matches everything with an empty or whitespace-only query", () => {
        expect(matchLabel("Settings…", "")).toEqual({ score: 0, hits: [] });
        expect(matchLabel("Settings…", "   ")).toEqual({ score: 0, hits: [] });
    });

    it("is case-insensitive and returns substring hit indexes", () => {
        expect(matchLabel("Settings…", "SET")?.hits).toEqual([0, 1, 2]);
    });

    it("prefers a substring occurrence that starts a word", () => {
        expect(matchLabel("Select Theme…", "the")?.hits).toEqual([7, 8, 9]);
        expect(matchLabel("Open Profile: api-server", "ser")?.hits).toEqual([18, 19, 20]);
    });

    it("falls back to an in-order fuzzy match", () => {
        expect(matchLabel("Edit Profile…", "eprf")?.hits).toEqual([0, 5, 6, 8]);
    });

    it("ignores spaces in the query", () => {
        expect(matchLabel("Open Profile: api-server", "open pro")).not.toBeNull();
    });

    it("returns null when characters are missing or out of order", () => {
        expect(matchLabel("Settings…", "xyz")).toBeNull();
        expect(matchLabel("About b3tty…", "ytt")).toBeNull();
    });

    it("scores a substring match above any fuzzy match", () => {
        const sub = matchLabel("Settings…", "set")!;
        const fuzzy = matchLabel("Select Theme…", "set")!;
        expect(sub.score).toBeGreaterThan(fuzzy.score);
    });
});

describe("searchCommands", () => {
    it("returns every command in original order for an empty query", () => {
        expect(ids("")).toEqual(COMMANDS.map((c) => c.id));
    });

    it("puts recent commands first, most recent first, for an empty query", () => {
        const result = searchCommands(COMMANDS, "", ["settings", "theme:tokyo-night"]);
        expect(result.map((m) => m.command.id).slice(0, 3)).toEqual(["settings", "theme:tokyo-night", "select-theme"]);
        expect(result.map((m) => m.recent).slice(0, 3)).toEqual([true, true, false]);
    });

    it("ignores recent ids that aren't in the command list", () => {
        expect(ids("", ["gone", "about"])[0]).toBe("about");
    });

    it("drops non-matching commands and ranks better matches first", () => {
        expect(ids("prof")).toEqual(["edit-profile", "profile:api-server"]);
        expect(ids("zzz")).toEqual([]);
    });

    it("breaks a score tie in favor of the recent command but doesn't label it recent", () => {
        // "Select Theme…" and "Switch Theme: …" both match "theme" at index 7.
        const result = searchCommands(COMMANDS, "theme", ["theme:tokyo-night"]);
        expect(result.map((m) => m.command.id)).toEqual(["edit-theme", "theme:tokyo-night", "select-theme"]);
        expect(result.every((m) => !m.recent)).toBe(true);
    });
});

describe("splitLabel", () => {
    it("groups consecutive hits and misses into runs", () => {
        expect(splitLabel("Settings", [0, 1, 4])).toEqual([
            { text: "Se", hit: true },
            { text: "tt", hit: false },
            { text: "i", hit: true },
            { text: "ngs", hit: false },
        ]);
    });

    it("returns one unmatched run when there are no hits", () => {
        expect(splitLabel("About", [])).toEqual([{ text: "About", hit: false }]);
    });
});
