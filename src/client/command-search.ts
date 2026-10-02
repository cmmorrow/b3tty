/**
 * Matching and ranking for the command palette (<b3tty-command-palette> in
 * components.ts). Kept free of DOM access so it can be tested under bun test.
 */
import type { PaletteCommand } from "./types.ts";

/** How well a label matched a query, and which label indexes matched. */
export interface LabelMatch {
    score: number;
    hits: number[];
}

/** A command that matched the current query, in display order. */
export interface CommandMatch {
    command: PaletteCommand;
    hits: number[];
    /** True when the query is empty and the command is in the recent list. */
    recent: boolean;
}

/** A run of label text that is either all matched or all unmatched. */
export interface LabelSegment {
    text: string;
    hit: boolean;
}

function isWordStart(label: string, i: number): boolean {
    return i === 0 || !/[a-z0-9]/.test(label[i - 1] ?? "");
}

/**
 * Matches query against label, case-insensitively and ignoring whitespace in
 * the query. A contiguous substring match wins (preferring one that starts a
 * word) and always outranks a fuzzy match; otherwise every query character
 * must appear in order, scoring extra for consecutive characters and word
 * starts. Returns null when the label doesn't match. An empty query matches
 * every label with score 0 and no hits.
 */
export function matchLabel(label: string, query: string): LabelMatch | null {
    const q = query.toLowerCase().replace(/\s+/g, "");
    if (!q) return { score: 0, hits: [] };
    const l = label.toLowerCase();

    let at = -1;
    for (let p = l.indexOf(q); p >= 0; p = l.indexOf(q, p + 1)) {
        if (at < 0) at = p;
        if (isWordStart(l, p)) {
            at = p;
            break;
        }
    }
    if (at >= 0) {
        const hits = Array.from({ length: q.length }, (_, i) => at + i);
        return { score: 1000 - at + (isWordStart(l, at) ? 200 : 0), hits };
    }

    const hits: number[] = [];
    let from = 0;
    let prev = -2;
    let score = 0;
    for (const ch of q) {
        const j = l.indexOf(ch, from);
        if (j < 0) return null;
        score += (j === prev + 1 ? 4 : 1) + (isWordStart(l, j) ? 3 : 0);
        hits.push(j);
        prev = j;
        from = j + 1;
    }
    return { score, hits };
}

/**
 * Returns the commands matching query, best first. recentIds lists command ids
 * most recent first: with an empty query those commands come first in that
 * order, followed by the rest in their original order; with a query, a recent
 * command wins a tie against an equally good match.
 */
export function searchCommands(commands: PaletteCommand[], query: string, recentIds: string[] = []): CommandMatch[] {
    const empty = !query.trim();
    const found: Array<CommandMatch & { score: number; order: number; recentRank: number }> = [];
    commands.forEach((command, order) => {
        const m = matchLabel(command.label, query);
        if (!m) return;
        const r = recentIds.indexOf(command.id);
        const recentRank = r < 0 ? Infinity : r;
        found.push({ command, hits: m.hits, recent: empty && r >= 0, score: m.score, order, recentRank });
    });
    found.sort((a, b) => b.score - a.score || a.recentRank - b.recentRank || a.order - b.order);
    return found.map(({ command, hits, recent }) => ({ command, hits, recent }));
}

/** Splits label into alternating matched/unmatched runs for highlighting. */
export function splitLabel(label: string, hits: number[]): LabelSegment[] {
    const hitSet = new Set(hits);
    const out: LabelSegment[] = [];
    for (let i = 0; i < label.length; i++) {
        const hit = hitSet.has(i);
        const last = out[out.length - 1];
        if (last && last.hit === hit) last.text += label[i];
        else out.push({ text: label[i] ?? "", hit });
    }
    return out;
}
