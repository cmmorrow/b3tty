import type { PaletteCommand } from "../types.ts";
import { type CommandMatch, searchCommands, splitLabel } from "../command-search.ts";
import { BASE_STYLES, DESIGN_TOKENS, OVERLAY_STYLES } from "../design.ts";

/**
 * Interface for the b3tty-command-palette web component.
 */
export interface B3ttyCommandPalette {
    open(commands: PaletteCommand[], recentIds?: string[]): void;
    close(): void;
}

/**
 * Returns true when el is a b3tty-command-palette element.
 */
export function isB3ttyCommandPalette(el: Element): el is HTMLElement & B3ttyCommandPalette {
    return el.tagName.toLowerCase() === "b3tty-command-palette";
}

// Rows have a fixed height so the list is always exactly VISIBLE_ROWS tall,
// scrolling when more commands match.
const PALETTE_ROW_HEIGHT = 32;
const PALETTE_ROW_GAP = 2;
const PALETTE_VISIBLE_ROWS = 8;

if (typeof HTMLElement !== "undefined") {
    class B3ttyCommandPaletteImpl extends HTMLElement implements B3ttyCommandPalette {
        #shadow: ShadowRoot;
        #input: HTMLInputElement;
        #list: HTMLElement;
        #count: HTMLElement;
        #commands: PaletteCommand[] = [];
        #recentIds: string[] = [];
        #matches: CommandMatch[] = [];
        #selected = 0;

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });
            const listHeight = PALETTE_VISIBLE_ROWS * PALETTE_ROW_HEIGHT + (PALETTE_VISIBLE_ROWS - 1) * PALETTE_ROW_GAP;

            const style = document.createElement("style");
            style.textContent = `
                ${DESIGN_TOKENS}
                ${BASE_STYLES}
                ${OVERLAY_STYLES}
                .overlay { align-items: flex-start; padding-top: 64px; }
                .modal {
                    width: 480px; max-width: 100%;
                    padding: var(--space-md);
                    display: flex; flex-direction: column; gap: var(--space-sm);
                }
                .search {
                    display: block; width: 100%; box-sizing: border-box; margin: 0;
                    font-size: var(--font-size-xl);
                    padding: var(--space-md) var(--space-lg);
                    background: var(--color-surface-3); color: var(--color-text);
                    border: 3px solid var(--color-accent); border-radius: var(--radius-md);
                    outline: none; appearance: none;
                    transition: border-color var(--transition);
                }
                .search:focus { border-color: var(--color-accent-hover); }
                .list {
                    display: flex; flex-direction: column; gap: ${PALETTE_ROW_GAP}px;
                    height: ${listHeight}px; overflow-y: auto;
                }
                .list.scrolls { padding-right: var(--space-xs); }
                .list::-webkit-scrollbar { width: 8px; }
                .list::-webkit-scrollbar-track { background: var(--color-surface-2); border-radius: var(--radius-full); }
                .list::-webkit-scrollbar-thumb { background: var(--color-handle); border-radius: var(--radius-full); }
                .list::-webkit-scrollbar-thumb:hover { background: var(--color-muted); }
                .row {
                    display: flex; align-items: center; gap: var(--space-lg);
                    height: ${PALETTE_ROW_HEIGHT}px; flex-shrink: 0; box-sizing: border-box;
                    padding: 0 var(--space-md); border-radius: var(--radius-sm);
                    font-size: var(--font-size-md); color: var(--color-text);
                    cursor: pointer; user-select: none;
                    transition: background var(--transition);
                }
                .row[aria-selected="true"] { background: var(--color-accent); color: var(--color-on-accent); }
                .label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
                .hit { font-weight: var(--font-weight-bold); text-decoration: underline; text-underline-offset: 2px; }
                .group { flex-shrink: 0; font-size: var(--font-size-xs); color: var(--color-text-subtle); }
                .row[aria-selected="true"] .group { color: var(--color-surface-1); }
                .empty { padding: var(--space-lg) var(--space-md); font-size: var(--font-size-md); color: var(--color-text-subtle); }
                .footer {
                    display: flex; align-items: center; gap: var(--space-lg);
                    padding: var(--space-sm) var(--space-2xs) 0;
                    border-top: 1px solid var(--color-border-inner);
                    font-size: var(--font-size-xs); color: var(--color-text-subtle);
                }
                .hint { display: flex; align-items: center; gap: var(--space-2xs); }
                kbd {
                    font-family: var(--font-family-mono); font-size: var(--font-size-xs);
                    padding: 1px var(--space-2xs);
                    border: 1px solid var(--color-border); border-radius: var(--radius-sm);
                    background: var(--color-surface-3); color: var(--color-text);
                }
                .count { margin-left: auto; }
            `;

            const overlay = document.createElement("div");
            overlay.className = "overlay";
            const modal = document.createElement("div");
            modal.className = "modal";
            modal.setAttribute("role", "dialog");
            modal.setAttribute("aria-modal", "true");
            modal.setAttribute("aria-label", "Command palette");

            this.#input = document.createElement("input");
            this.#input.className = "search";
            this.#input.type = "text";
            this.#input.autocomplete = "off";
            this.#input.spellcheck = false;
            this.#input.setAttribute("role", "combobox");
            this.#input.setAttribute("aria-label", "Search commands");
            this.#input.setAttribute("aria-expanded", "true");
            this.#input.setAttribute("aria-controls", "command-list");
            this.#input.setAttribute("aria-autocomplete", "list");
            this.#input.addEventListener("input", () => {
                this.#selected = 0;
                this.#render();
            });

            this.#list = document.createElement("div");
            this.#list.className = "list";
            this.#list.id = "command-list";
            this.#list.setAttribute("role", "listbox");

            // Keep focus in the search box when anything else in the palette (a row,
            // the footer, the backdrop) is pressed; clicking the backdrop closes it.
            overlay.addEventListener("mousedown", (e) => {
                if (e.target !== this.#input) e.preventDefault();
            });
            overlay.addEventListener("click", (e) => {
                if (e.target === overlay) this.#cancel();
            });

            const footer = document.createElement("div");
            footer.className = "footer";
            const hint = (keys: string[], text: string): HTMLElement => {
                const span = document.createElement("span");
                span.className = "hint";
                for (const key of keys) {
                    const kbd = document.createElement("kbd");
                    kbd.textContent = key;
                    span.appendChild(kbd);
                }
                span.append(` ${text}`);
                return span;
            };
            this.#count = document.createElement("span");
            this.#count.className = "count";
            this.#count.setAttribute("aria-live", "polite");
            footer.append(hint(["↑", "↓"], "navigate"), hint(["↵"], "run"), hint(["esc"], "close"), this.#count);

            modal.append(this.#input, this.#list, footer);
            overlay.appendChild(modal);
            this.#shadow.append(style, overlay);
        }

        /**
         * Opens the palette with an empty query and focuses the search box.
         * recentIds lists command ids, most recent first; with an empty query
         * those commands are listed first and labeled "recent".
         */
        open(commands: PaletteCommand[], recentIds: string[] = []): void {
            this.#commands = commands;
            this.#recentIds = recentIds;
            this.#input.value = "";
            this.#selected = 0;
            this.setAttribute("open", "");
            this.#render();
            this.#input.focus();
            document.addEventListener("keydown", this.#onDocumentKeyDown, { capture: true });
        }

        close(): void {
            this.removeAttribute("open");
            document.removeEventListener("keydown", this.#onDocumentKeyDown, { capture: true });
        }

        /** Closes the palette and dispatches b3tty-command-cancel (Escape or a backdrop click). */
        #cancel(): void {
            this.close();
            // Lets the caller return focus to wherever the palette was opened from.
            this.dispatchEvent(new CustomEvent("b3tty-command-cancel", { bubbles: true, composed: true }));
        }

        /**
         * Handles keys at the document level, in the capture phase, while the palette is
         * open, so navigation keeps working when focus has left the search box (e.g.
         * after switching tabs) and no key reaches the terminal underneath. Any other
         * key typed while focus is elsewhere moves focus back to the search box, where
         * its character lands.
         */
        #onDocumentKeyDown = (e: KeyboardEvent): void => {
            if (e.isComposing) return;
            const n = this.#matches.length;
            let handled = true;
            if (e.key === "ArrowDown") {
                if (n) this.#select((this.#selected + 1) % n);
            } else if (e.key === "ArrowUp") {
                if (n) this.#select((this.#selected - 1 + n) % n);
            } else if (e.key === "Enter") {
                if (n) this.#run(this.#selected);
            } else if (e.key === "Escape") {
                this.#cancel();
            } else if (e.key === "Tab") {
                this.#input.focus();
            } else {
                handled = false;
            }
            if (handled) {
                e.preventDefault();
                e.stopPropagation();
            } else if (this.#shadow.activeElement !== this.#input) {
                e.stopPropagation();
                this.#input.focus();
            }
        };

        #run(index: number): void {
            const match = this.#matches[index];
            if (!match) return;
            this.close();
            this.dispatchEvent(
                new CustomEvent("b3tty-command-run", {
                    bubbles: true,
                    composed: true,
                    detail: { id: match.command.id },
                })
            );
        }

        #select(index: number): void {
            const rows = this.#list.querySelectorAll<HTMLElement>(".row");
            rows[this.#selected]?.setAttribute("aria-selected", "false");
            this.#selected = index;
            const row = rows[index];
            if (!row) return;
            row.setAttribute("aria-selected", "true");
            this.#input.setAttribute("aria-activedescendant", row.id);
            row.scrollIntoView({ block: "nearest" });
        }

        #render(): void {
            this.#matches = searchCommands(this.#commands, this.#input.value, this.#recentIds);
            const n = this.#matches.length;
            this.#list.replaceChildren();
            this.#list.classList.toggle("scrolls", n > PALETTE_VISIBLE_ROWS);
            this.#input.removeAttribute("aria-activedescendant");

            this.#matches.forEach((match, i) => {
                const row = document.createElement("div");
                row.className = "row";
                row.id = `command-${i}`;
                row.setAttribute("role", "option");
                row.setAttribute("aria-selected", "false");

                const label = document.createElement("span");
                label.className = "label";
                for (const seg of splitLabel(match.command.label, match.hits)) {
                    if (seg.hit) {
                        const b = document.createElement("span");
                        b.className = "hit";
                        b.textContent = seg.text;
                        label.appendChild(b);
                    } else {
                        label.append(seg.text);
                    }
                }
                row.append(label);
                // The label's prefix (theme:, edit:, …) already names the kind of
                // command, so the right-hand column only flags recent ones.
                if (match.recent) {
                    const recent = document.createElement("span");
                    recent.className = "group";
                    recent.textContent = "recent";
                    row.append(recent);
                }
                // pointermove rather than mouseenter, so a row scrolled under a
                // resting pointer by arrow-key navigation doesn't steal the selection.
                row.addEventListener("pointermove", () => {
                    if (this.#selected !== i) this.#select(i);
                });
                row.addEventListener("click", () => this.#run(i));
                this.#list.appendChild(row);
            });

            if (n === 0) {
                const empty = document.createElement("div");
                empty.className = "empty";
                empty.textContent = "No matching commands";
                this.#list.appendChild(empty);
            } else {
                this.#select(Math.min(this.#selected, n - 1));
            }
            this.#count.textContent = `${n} of ${this.#commands.length}`;
        }
    }

    customElements.define("b3tty-command-palette", B3ttyCommandPaletteImpl);
}
