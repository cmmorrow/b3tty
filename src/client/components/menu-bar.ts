import { BASE_STYLES, DESIGN_TOKENS } from "../design.ts";

/**
 * Colors used to style the menu bar: bg is applied as the bar's background,
 * fg as its text/icon color.
 */
export interface MenuBarColors {
    bg: string;
    fg: string;
}

/**
 * Interface for the b3tty-menu-bar web component.
 */
export interface B3ttyMenuBar {
    setup(themeNames: string[], profileNames: string[], colors: MenuBarColors, mode?: "hover" | "visible"): void;
    updateColors(colors: MenuBarColors): void;
}

/**
 * Returns true when el exposes the B3ttyMenuBar contract (setup/updateColors methods).
 * Use this instead of `as unknown as B3ttyMenuBar` to preserve type safety.
 */
export function isB3ttyMenuBar(el: Element): el is HTMLElement & B3ttyMenuBar {
    const r = el as unknown as Record<string, unknown>;
    return typeof r["setup"] === "function" && typeof r["updateColors"] === "function";
}

if (typeof HTMLElement !== "undefined") {
    class B3ttyMenuBarImpl extends HTMLElement implements B3ttyMenuBar {
        #hideTimer: ReturnType<typeof setTimeout> | null = null;
        #activeSection: string | null = null;
        #mode: "hover" | "visible" = "hover";
        #shadow: ShadowRoot;
        #menubar: HTMLDivElement;
        #trigger: HTMLDivElement;

        #onDocumentPointerDown = (e: PointerEvent): void => {
            if (this.#activeSection === null) return;
            if (!e.composedPath().includes(this)) {
                this.#toggleSection(this.#activeSection);
            }
        };

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            const style = document.createElement("style");
            style.textContent = `
                ${DESIGN_TOKENS}
                ${BASE_STYLES}
                :host {
                    display: block;
                    height: 0;
                    overflow: visible;
                    flex-shrink: 0;
                }
                :host([open]) {
                    height: var(--menubar-height);
                }
                .trigger {
                    position: fixed;
                    top: 0;
                    left: 50%;
                    transform: translateX(-50%);
                    width: 100px;
                    height: 6px;
                    background: var(--color-handle);
                    border-radius: 0 0 var(--radius-sm) var(--radius-sm);
                    cursor: pointer;
                    z-index: var(--z-menubar);
                    opacity: 0.5;
                    transition: opacity var(--transition);
                }
                .trigger:hover {
                    opacity: 1;
                }
                :host([open]) .trigger {
                    display: none;
                }
                :host([mode="visible"]) .trigger {
                    display: none;
                }
                .menubar {
                    display: none;
                    width: 100%;
                    height: var(--menubar-height);
                    box-sizing: border-box;
                    flex-direction: row;
                    align-items: stretch;
                    background: var(--menu-bg, #fff);
                    color: var(--menu-fg, #000);
                    font-size: var(--font-size-md);
                    user-select: none;
                    position: relative;
                }
                :host([open]) .menubar {
                    display: flex;
                }
                .section {
                    position: relative;
                }
                .section-label {
                    display: flex;
                    align-items: center;
                    padding: 0 var(--space-lg);
                    height: var(--menubar-height);
                    cursor: pointer;
                    box-sizing: border-box;
                    color: var(--menu-fg, #000);
                }
                .section-label:hover,
                .section.active .section-label {
                    filter: brightness(0.85);
                    background: var(--menu-bg, #fff);
                }
                .dropdown {
                    display: none;
                    position: absolute;
                    top: var(--menubar-height);
                    left: 0;
                    min-width: 140px;
                    background: var(--menu-bg, #fff);
                    color: var(--menu-fg, #000);
                    box-shadow: var(--shadow-dropdown);
                    flex-direction: column;
                    z-index: var(--z-dropdown);
                }
                .section.active .dropdown {
                    display: flex;
                }
                .menu-item {
                    padding: var(--space-xs) var(--space-xl);
                    cursor: pointer;
                    white-space: nowrap;
                    color: var(--menu-fg, #000);
                }
                .menu-item:hover {
                    filter: brightness(0.85);
                    background: var(--menu-bg, #fff);
                }
                .menu-separator {
                    height: 1px;
                    background: var(--menu-fg, #000);
                    opacity: 0.2;
                    margin: var(--space-3xs) var(--space-sm);
                }
            `;

            this.#trigger = document.createElement("div");
            this.#trigger.className = "trigger";

            this.#menubar = document.createElement("div");
            this.#menubar.className = "menubar";

            this.#shadow.appendChild(style);
            this.#shadow.appendChild(this.#trigger);
            this.#shadow.appendChild(this.#menubar);

            this.#trigger.addEventListener("mouseenter", () => this.#open());
            this.#menubar.addEventListener("mouseenter", () => this.#resetTimer());
        }

        connectedCallback(): void {
            document.addEventListener("pointerdown", this.#onDocumentPointerDown);
        }

        disconnectedCallback(): void {
            document.removeEventListener("pointerdown", this.#onDocumentPointerDown);
        }

        setup(
            themeNames: string[],
            profileNames: string[],
            colors: MenuBarColors,
            mode: "hover" | "visible" = "hover"
        ): void {
            this.#mode = mode;
            this.setAttribute("mode", mode);
            this.updateColors(colors);
            this.#menubar.innerHTML = "";
            this.#activeSection = null;

            // Always add all three sections so all actions are always accessible.
            this.#menubar.appendChild(this.#buildSection("b3tty", "b3tty", []));
            this.#menubar.appendChild(this.#buildSection("themes", "Themes", themeNames));
            this.#menubar.appendChild(this.#buildSection("profiles", "Profiles", profileNames));

            if (this.#mode === "visible") {
                this.#open();
            }
        }

        updateColors(colors: MenuBarColors): void {
            (this.#shadow.host as HTMLElement).style.setProperty("--menu-bg", colors.bg);
            (this.#shadow.host as HTMLElement).style.setProperty("--menu-fg", colors.fg);
        }

        #buildSection(type: string, label: string, items: string[]): HTMLDivElement {
            const section = document.createElement("div");
            section.className = "section";
            section.dataset["section"] = type;

            const sectionLabel = document.createElement("span");
            sectionLabel.className = "section-label";
            sectionLabel.textContent = label;

            const dropdown = document.createElement("div");
            dropdown.className = "dropdown";

            if (type === "b3tty") {
                const aboutItem = document.createElement("div");
                aboutItem.className = "menu-item";
                aboutItem.textContent = "About…";
                aboutItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(new CustomEvent("b3tty-open-about-dialog", { bubbles: true, composed: true }));
                    this.#close();
                });
                dropdown.appendChild(aboutItem);

                const settingsItem = document.createElement("div");
                settingsItem.className = "menu-item";
                settingsItem.textContent = "Settings\u2026";
                settingsItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(
                        new CustomEvent("b3tty-open-settings-editor", { bubbles: true, composed: true })
                    );
                    this.#close();
                });
                dropdown.appendChild(settingsItem);
            }

            if (type === "themes") {
                const selectItem = document.createElement("div");
                selectItem.className = "menu-item";
                selectItem.textContent = "Select Theme\u2026";
                selectItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(new CustomEvent("b3tty-open-theme-selector", { bubbles: true, composed: true }));
                    this.#close();
                });
                dropdown.appendChild(selectItem);
                const editItem = document.createElement("div");
                editItem.className = "menu-item";
                editItem.textContent = "Edit Theme…";
                editItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(new CustomEvent("b3tty-open-theme-editor", { bubbles: true, composed: true }));
                    this.#close();
                });
                dropdown.appendChild(editItem);
                if (items.length > 0) {
                    const sep = document.createElement("div");
                    sep.className = "menu-separator";
                    dropdown.appendChild(sep);
                }
            }

            if (type === "profiles") {
                const editProfileItem = document.createElement("div");
                editProfileItem.className = "menu-item";
                editProfileItem.textContent = "Edit Profile…";
                editProfileItem.addEventListener("click", (e) => {
                    e.stopPropagation();
                    this.dispatchEvent(new CustomEvent("b3tty-open-profile-editor", { bubbles: true, composed: true }));
                    this.#close();
                });
                dropdown.appendChild(editProfileItem);
                const switchable = items.filter((n) => n !== "default");
                if (switchable.length > 0) {
                    const sep = document.createElement("div");
                    sep.className = "menu-separator";
                    dropdown.appendChild(sep);
                    for (const name of switchable) {
                        const item = document.createElement("div");
                        item.className = "menu-item";
                        item.textContent = name;
                        item.addEventListener("click", (e) => {
                            e.stopPropagation();
                            this.dispatchEvent(
                                new CustomEvent("b3tty-profile-change", {
                                    detail: { name },
                                    bubbles: true,
                                    composed: true,
                                })
                            );
                            this.#close();
                        });
                        dropdown.appendChild(item);
                    }
                }
            } else {
                for (const name of items) {
                    const item = document.createElement("div");
                    item.className = "menu-item";
                    item.textContent = name;
                    item.addEventListener("click", (e) => {
                        e.stopPropagation();
                        const eventName = type === "themes" ? "b3tty-theme-change" : "b3tty-profile-change";
                        this.dispatchEvent(
                            new CustomEvent(eventName, {
                                detail: { name },
                                bubbles: true,
                                composed: true,
                            })
                        );
                        if (type === "themes") {
                            this.#toggleSection(type);
                        } else {
                            this.#close();
                        }
                    });
                    dropdown.appendChild(item);
                }
            }

            sectionLabel.addEventListener("click", (e) => {
                e.stopPropagation();
                this.#toggleSection(type);
            });

            section.appendChild(sectionLabel);
            section.appendChild(dropdown);
            return section;
        }

        #open(): void {
            this.style.height = "var(--menubar-height)";
            this.setAttribute("open", "");
            this.dispatchEvent(new CustomEvent("b3tty-menubar-open", { bubbles: true, composed: true }));
            this.#resetTimer();
        }

        #close(): void {
            if (this.#hideTimer !== null) {
                clearTimeout(this.#hideTimer);
                this.#hideTimer = null;
            }
            this.#activeSection = null;
            for (const s of Array.from(this.#menubar.querySelectorAll(".section.active"))) {
                s.classList.remove("active");
            }
            // In "visible" mode the bar itself never hides — only the active
            // dropdown section (cleared above) closes.
            if (this.#mode === "visible") return;
            this.style.height = "0px";
            this.removeAttribute("open");
            this.dispatchEvent(new CustomEvent("b3tty-menubar-close", { bubbles: true, composed: true }));
        }

        #resetTimer(): void {
            // "visible" mode never auto-hides, so no idle timer is armed.
            if (this.#mode === "visible") return;
            if (this.#hideTimer !== null) clearTimeout(this.#hideTimer);
            this.#hideTimer = setTimeout(() => this.#close(), 5000);
        }

        #toggleSection(type: string): void {
            this.#resetTimer();
            const section = this.#menubar.querySelector<HTMLDivElement>(`.section[data-section="${type}"]`);
            if (!section) return;
            if (this.#activeSection === type) {
                section.classList.remove("active");
                this.#activeSection = null;
            } else {
                for (const s of Array.from(this.#menubar.querySelectorAll(".section.active"))) {
                    s.classList.remove("active");
                }
                section.classList.add("active");
                this.#activeSection = type;
            }
        }
    }

    customElements.define("b3tty-menu-bar", B3ttyMenuBarImpl);
}
