/**
 * Slide-and-fade motion for items entering and leaving a vertical flex list: an
 * entering item grows from zero height, pushing the items below it down, then fades
 * in; a leaving item fades out, then collapses. The item must have no padding or
 * border of its own (wrap anything that does), so that it can reach zero height.
 */

/** How long the slide (height) and fade (opacity) phases take. */
export const SLIDE_MS = 180;
export const FADE_MS = 150;

/** The names in `next` that aren't in `prev`, and the names in `prev` that aren't in `next`. */
export function listChanges(prev: Iterable<string>, next: Iterable<string>): { added: string[]; removed: string[] } {
    const before = new Set(prev);
    const after = new Set(next);
    return {
        added: [...after].filter((name) => !before.has(name)),
        removed: [...before].filter((name) => !after.has(name)),
    };
}

/**
 * The keyframes for an item of `height` px entering (or, reversed, leaving) a list
 * whose items are `gap` px apart. While it slides, a negative top margin cancels the
 * gap the list adds beside the item, so the items below move smoothly from their old
 * place rather than jumping by the gap first.
 */
export function slideKeyframes(height: number, gap: number, direction: "in" | "out"): Keyframe[] {
    const collapsed = { height: "0px", marginTop: `${-gap}px`, opacity: 0 };
    const open = { height: `${height}px`, marginTop: "0px", opacity: 0 };
    const shown = { height: `${height}px`, marginTop: "0px", opacity: 1 };
    const total = SLIDE_MS + FADE_MS;
    if (direction === "in") {
        return [
            { ...collapsed, offset: 0, easing: "ease-out" },
            { ...open, offset: SLIDE_MS / total, easing: "linear" },
            { ...shown, offset: 1 },
        ];
    }
    return [
        { ...shown, offset: 0, easing: "linear" },
        { ...open, offset: FADE_MS / total, easing: "ease-in" },
        { ...collapsed, offset: 1 },
    ];
}

/**
 * Puts `children` into `parent` in the given order, moving only the ones out of place.
 * Leaving items (data-leaving) are stepped over, so they stay where they were.
 */
export function placeInOrder(parent: Element, children: Element[]): void {
    const staying = (el: Element | null): Element | null => {
        while (el && el instanceof HTMLElement && "leaving" in el.dataset) el = el.nextElementSibling;
        return el;
    };
    let next = staying(parent.firstElementChild);
    for (const child of children) {
        if (child === next) next = staying(child.nextElementSibling);
        else parent.insertBefore(child, next);
    }
}

/** True when the user has asked the OS for less motion. */
export function prefersReducedMotion(): boolean {
    return typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** The parent's row gap when `el` has a sibling to be spaced from, else 0. */
function gapBeside(el: HTMLElement): number {
    const parent = el.parentElement;
    if (!parent || parent.childElementCount < 2) return 0;
    return parseFloat(getComputedStyle(parent).rowGap) || 0;
}

function run(el: HTMLElement, direction: "in" | "out"): Promise<void> {
    const keyframes = slideKeyframes(el.offsetHeight, gapBeside(el), direction);
    el.style.overflow = "hidden";
    const animation = el.animate(keyframes, { duration: SLIDE_MS + FADE_MS });
    return animation.finished.then(
        () => undefined,
        () => undefined // cancelled
    );
}

/** Plays `el`'s entrance; `el` must already be in place in the list. */
export function slideIn(el: HTMLElement): void {
    void run(el, "in").then(() => el.style.removeProperty("overflow"));
}

/**
 * Plays `el`'s exit, then removes it. It stays in the DOM meanwhile, so it is made
 * inert and marked with data-leaving for callers to skip.
 */
export function slideOut(el: HTMLElement): void {
    el.dataset.leaving = "";
    el.inert = true;
    void run(el, "out").then(() => el.remove());
}
