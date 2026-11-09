/**
 * Removes a menu-driven overlay's open attribute and, if it was open, dispatches
 * b3tty-overlay-close (bubbles + composed) so terminal.ts can return focus to the
 * terminal. Shared by every overlay opened from the menu bar or command palette;
 * <b3tty-dialog> (the connection-closed notice) deliberately doesn't use it.
 */
export function closeOverlay(el: HTMLElement): void {
    if (!el.hasAttribute("open")) return;
    el.removeAttribute("open");
    el.dispatchEvent(new CustomEvent("b3tty-overlay-close", { bubbles: true, composed: true }));
}

/**
 * Closes a dialog that can open on top of another overlay. With a connected opener,
 * focus goes back to it and no b3tty-overlay-close is dispatched (that would send
 * focus to the terminal, under the still-open overlay); otherwise it is closeOverlay.
 */
export function closeStackedOverlay(el: HTMLElement, opener: HTMLElement | null): void {
    if (opener?.isConnected) {
        if (!el.hasAttribute("open")) return;
        el.removeAttribute("open");
        opener.focus();
    } else {
        closeOverlay(el);
    }
}
