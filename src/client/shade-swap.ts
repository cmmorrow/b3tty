/**
 * Translucent shade characters (terminal.translucent-shades).
 *
 * Fonts draw the shade characters ░ ▒ ▓ (U+2591–U+2593) as dithered patterns.
 * With the setting on, each run of one shade is wrapped in a `.b3tty-shade`
 * span (styled in terminal.css) that fills its cells with the text color at
 * 25%, 50%, or 75% opacity. xterm's DOM renderer offers no per-cell styling
 * hook and rebuilds a row's spans every time it redraws it, so a
 * MutationObserver on `.xterm-rows` re-applies the swap to each redrawn row,
 * before the browser paints it. Only the DOM changes: the buffer, and so
 * copied text, keeps the original characters.
 */

/** The opacity each shade character is drawn at. */
export const SHADE_OPACITY: Readonly<Record<string, string>> = {
    "░": "0.25",
    "▒": "0.5",
    "▓": "0.75",
};

export const SHADE_CLASS = "b3tty-shade";

const HAS_SHADE = /[░-▓]/;
const SHADE_RUN = /░+|▒+|▓+/g;

/** A piece of a split text: plain text, or a run of one shade character. */
export type ShadePiece = { text: string; opacity?: string };

/**
 * Splits text into plain pieces and runs of a single shade character, the
 * latter carrying their opacity. Returns null when the text has no shade
 * characters, so callers can skip it without allocating.
 */
export function splitShadeRuns(text: string): ShadePiece[] | null {
    if (!HAS_SHADE.test(text)) return null;
    const pieces: ShadePiece[] = [];
    let last = 0;
    for (const m of text.matchAll(SHADE_RUN)) {
        const at = m.index ?? 0;
        if (at > last) pieces.push({ text: text.slice(last, at) });
        pieces.push({ text: m[0], opacity: SHADE_OPACITY[m[0].charAt(0)] });
        last = at + m[0].length;
    }
    if (last < text.length) pieces.push({ text: text.slice(last) });
    return pieces;
}

/** The parts of an xterm.js Terminal the swapper uses. */
export interface ShadeTerminal {
    readonly element: HTMLElement | undefined;
    readonly rows: number;
    refresh(start: number, end: number): void;
}

/**
 * Applies translucent shades to a terminal's rendered rows while enabled.
 * Create it after term.open(), since it needs the terminal's `.xterm-rows`.
 */
export class ShadeSwapper {
    #term: ShadeTerminal;
    #observer: MutationObserver | null = null;

    constructor(term: ShadeTerminal) {
        this.#term = term;
    }

    /** Turns the swap on or off; does nothing if it is already in that state. */
    setEnabled(enabled: boolean): void {
        if (enabled === (this.#observer !== null)) return;
        const rowsEl = this.#term.element?.querySelector(".xterm-rows");
        if (!rowsEl) return;
        if (enabled) {
            this.#observer = new MutationObserver((records) => this.#onMutations(rowsEl, records));
            this.#observer.observe(rowsEl, { childList: true, subtree: true, characterData: true });
            for (const row of Array.from(rowsEl.children)) swapRow(row);
            this.#observer.takeRecords();
        } else {
            this.#observer?.disconnect();
            this.#observer = null;
            // Redrawing every row replaces the swapped spans with xterm's own.
            this.#term.refresh(0, this.#term.rows - 1);
        }
    }

    #onMutations(rowsEl: Element, records: MutationRecord[]): void {
        const rows = new Set<Element>();
        for (const record of records) {
            let node: Node | null = record.target;
            while (node && node.parentNode !== rowsEl) node = node.parentNode;
            if (node instanceof Element) rows.add(node);
        }
        for (const row of rows) swapRow(row);
        // Drop the records the swap itself just produced.
        this.#observer?.takeRecords();
    }
}

/** Wraps every run of shade characters in one rendered row in a shade span. */
function swapRow(row: Element): void {
    const walker = document.createTreeWalker(row, NodeFilter.SHOW_TEXT);
    const found: [Text, ShadePiece[]][] = [];
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        const text = node as Text;
        if (text.parentElement?.classList.contains(SHADE_CLASS)) continue;
        const pieces = splitShadeRuns(text.data);
        if (pieces) found.push([text, pieces]);
    }
    for (const [text, pieces] of found) {
        const fragment = document.createDocumentFragment();
        for (const piece of pieces) {
            if (piece.opacity === undefined) {
                fragment.append(piece.text);
                continue;
            }
            const span = document.createElement("span");
            span.className = SHADE_CLASS;
            span.style.opacity = piece.opacity;
            span.textContent = piece.text;
            fragment.append(span);
        }
        text.replaceWith(fragment);
    }
}
