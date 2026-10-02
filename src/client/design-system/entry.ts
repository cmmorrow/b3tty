/**
 * Entry point of the design system bundle (see build.ts). Importing
 * ../components.ts defines all ten b3tty-* custom elements; this module then
 * assigns window.B3tty with the helpers and fixture data the previews use.
 */
import "../components.ts";
import * as styles from "../design.ts";
import THEMES from "b3tty:builtin-themes";
import { PROFILE_NAMES, SAMPLE_SETTINGS, USER_THEMES, paletteOf } from "./mock-api.ts";

// Every custom property DESIGN_TOKENS declares, in source order.
const TOKEN_NAMES = [...styles.DESIGN_TOKENS.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1] ?? "");

/**
 * Copies each design token's value from the document root (the design
 * system's tokens.css) onto el as an inline custom property. Components
 * declare their tokens on :host, and an inline style on the host outranks
 * :host, so token edits in the design system show in the real components.
 */
function applyTokens(el: HTMLElement): HTMLElement {
    const root = getComputedStyle(document.documentElement);
    for (const name of TOKEN_NAMES) {
        const v = root.getPropertyValue(name).trim();
        if (v) el.style.setProperty(name, v);
    }
    return el;
}

/**
 * Renders html inside a shadow root styled by DESIGN_TOKENS, BASE_STYLES and
 * the named design.ts fragments, in the same order a component uses them,
 * followed by any extra css.
 */
function mountStyled(host: HTMLElement, fragments: string[], html: string, css = ""): ShadowRoot {
    const shadow = host.attachShadow({ mode: "open" });
    const lookup = styles as Record<string, string>;
    const base = [styles.DESIGN_TOKENS, styles.BASE_STYLES, ...fragments.map((f) => lookup[f] ?? "")].join("\n");
    shadow.innerHTML = `<style>${base}\n${css}</style>${html}`;
    applyTokens(host);
    return shadow;
}

(globalThis as Record<string, unknown>)["B3tty"] = {
    version: __B3TTY_VERSION__,
    styles,
    tokenNames: TOKEN_NAMES,
    applyTokens,
    mountStyled,
    themes: THEMES,
    userThemeNames: Object.keys(USER_THEMES),
    profileNames: PROFILE_NAMES,
    sampleSettings: SAMPLE_SETTINGS,
    paletteOf,
    // Same as terminal.ts menuBarColors: bar = theme foreground, text = theme background.
    menuBarColors(name: string) {
        const t = THEMES[name] ?? THEMES["b3tty-dark"] ?? {};
        return { bg: t["foreground"] || "white", fg: t["background"] || "black" };
    },
};
