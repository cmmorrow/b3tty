import { getProfileConfig, postDeleteProfile, postEditProfile } from "../api.ts";
import type { ProfileConfig } from "../types.ts";
import {
    BASE_STYLES,
    BUTTON_STYLES,
    DESIGN_TOKENS,
    EDITOR_CARD_STYLES,
    FORM_INPUT_STYLES,
    OVERLAY_STYLES,
    SAVE_ERROR_STYLES,
    SPLIT_PANEL_STYLES,
    TEXT_STYLES,
} from "../design.ts";
import { closeOverlay } from "./overlay.ts";

/**
 * Interface for the b3tty-profile-editor web component.
 */
export interface B3ttyProfileEditor {
    open(profileNames: string[]): void;
    close(): void;
}

/**
 * Returns true when el is a b3tty-profile-editor element.
 */
export function isB3ttyProfileEditor(el: Element): el is HTMLElement & B3ttyProfileEditor {
    return el.tagName.toLowerCase() === "b3tty-profile-editor";
}

const PROFILE_EDITOR_STYLES = `
    ${DESIGN_TOKENS}
    ${BASE_STYLES}
    ${OVERLAY_STYLES}
    ${TEXT_STYLES}
    .modal {
        padding: var(--space-2xl);
        display: flex; flex-direction: row;
        width: min(680px, 100%); height: min(500px, 90vh);
        overflow: hidden;
    }
    ${SPLIT_PANEL_STYLES}
    .left-panel { width: 200px; }
    ${EDITOR_CARD_STYLES}
    .fields-section {
        display: flex; flex-direction: column; gap: var(--space-xs);
        flex-shrink: 0; padding-bottom: var(--space-md);
        border-bottom: 1px solid var(--color-border-inner);
        margin-bottom: var(--space-sm);
    }
    .field-row {
        display: grid;
        grid-template-columns: 120px 1fr;
        gap: var(--space-xs); align-items: center;
    }
    .field-row label { white-space: nowrap; }
    .field-row label .required { color: var(--color-destructive); }
    .name-error { grid-column: 2; }
    .commands-section {
        flex: 1; display: flex; flex-direction: column; gap: var(--space-2xs);
        min-height: 0;
    }
    .commands-label { flex-shrink: 0; }
    .commands-wrapper {
        display: flex;
        border: 1px solid var(--color-border);
        border-radius: var(--radius-sm);
        overflow: hidden;
        flex: 1; min-height: 0;
        background: var(--color-surface-3);
        font-family: var(--font-family-mono);
        font-size: var(--font-size-sm);
        line-height: var(--line-height-body);
    }
    .line-numbers {
        width: 32px;
        padding: var(--space-2xs);
        background: var(--color-surface-1);
        color: var(--color-text-faint);
        text-align: right;
        user-select: none;
        overflow: hidden;
        white-space: pre;
        line-height: inherit;
        flex-shrink: 0;
        box-sizing: border-box;
    }
    .commands-area {
        flex: 1;
        padding: var(--space-2xs) var(--space-xs);
        border: none;
        outline: none;
        resize: none;
        background: transparent;
        color: var(--color-text);
        font-family: inherit;
        font-size: inherit;
        line-height: inherit;
        overflow: auto;
        white-space: pre;
        box-sizing: border-box;
    }
    ${FORM_INPUT_STYLES}
    ${BUTTON_STYLES}
    ${SAVE_ERROR_STYLES}
    /* .delete-btn already claims margin-right: auto in this
       actions bar; save-error sits immediately to its right
       instead of also trying to auto-push. */
    .save-error { margin-right: 0; }
`;

/** The profile editor's text fields, keyed by data-field. */
const PROFILE_FIELDS = [
    { key: "shell", label: "Shell", placeholder: "e.g. /bin/zsh" },
    { key: "title", label: "Title", placeholder: "Terminal window title" },
    { key: "working-directory", label: "Working Directory", placeholder: "e.g. ~/projects" },
    { key: "root", label: "Root", placeholder: "e.g. /" },
];

// Builds markup from the hard-coded values above; profile data reaches the form
// later, through input values.
const profileField = (key: string, label: string, placeholder: string): string => `
    <div class="field-row">
        <label class="field-label">${label}</label>
        <input type="text" class="field-input" placeholder="${placeholder}" data-field="${key}">
    </div>
`;

// The profile cards are added by open() (see #makeProfileCard), since their
// names are user data.
const PROFILE_EDITOR_LEFT_PANEL = `
    <div class="left-panel">
        <div class="create-card" selected>
            <input type="radio" name="profile" checked>
            <span>Create new profile</span>
        </div>
    </div>
`;

const PROFILE_EDITOR_RIGHT_PANEL = `
    <div class="right-panel">
        <div class="fields-section">
            <div class="field-row">
                <label class="field-label">Profile Name <span class="required">*</span></label>
                <input type="text" class="field-input" placeholder="Enter profile name" data-field="name">
                <span class="name-error field-error">Cannot use 'default' as a profile name</span>
            </div>
            ${PROFILE_FIELDS.map(({ key, label, placeholder }) => profileField(key, label, placeholder)).join("")}
        </div>
        <div class="commands-section">
            <div class="commands-label field-label">Commands (one per line, run on startup):</div>
            <div class="commands-wrapper">
                <div class="line-numbers">1</div>
                <textarea class="commands-area" placeholder="npm start" spellcheck="false" wrap="off"></textarea>
            </div>
        </div>
        <div class="actions">
            <button class="delete-btn" style="display: none;">Delete Profile</button>
            <span class="save-error"></span>
            <button class="cancel-btn">Cancel</button>
            <button class="ok-btn" disabled>OK</button>
        </div>
    </div>
`;

if (typeof HTMLElement !== "undefined") {
    class B3ttyProfileEditorImpl extends HTMLElement implements B3ttyProfileEditor {
        #shadow: ShadowRoot;
        #leftPanel: HTMLDivElement;
        #createCard: HTMLDivElement;
        #createRadio: HTMLInputElement;
        #nameInput: HTMLInputElement;
        #nameError: HTMLSpanElement;
        #shellInput: HTMLInputElement;
        #titleInput: HTMLInputElement;
        #wdInput: HTMLInputElement;
        #rootInput: HTMLInputElement;
        #commandsArea: HTMLTextAreaElement;
        #lineNumbers: HTMLDivElement;
        #okBtn: HTMLButtonElement;
        #cancelBtn: HTMLButtonElement;
        #deleteBtn: HTMLButtonElement;
        #saveError: HTMLSpanElement;
        #selectedName: string | null = null;
        #selectedCard: HTMLDivElement | null = null;
        #isLoading = false;
        /** True while an edit-profile or delete-profile request is in flight. */
        #saving = false;

        constructor() {
            super();
            this.#shadow = this.attachShadow({ mode: "open" });

            const shadow = this.#shadow;
            shadow.innerHTML = `
                <style>${PROFILE_EDITOR_STYLES}</style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true">
                        ${PROFILE_EDITOR_LEFT_PANEL}
                        ${PROFILE_EDITOR_RIGHT_PANEL}
                    </div>
                </div>
            `;
            const field = (key: string) => shadow.querySelector<HTMLInputElement>(`[data-field="${key}"]`)!;
            this.#leftPanel = shadow.querySelector<HTMLDivElement>(".left-panel")!;
            this.#createCard = shadow.querySelector<HTMLDivElement>(".create-card")!;
            this.#createRadio = this.#createCard.querySelector<HTMLInputElement>("input")!;
            this.#nameInput = field("name");
            this.#nameError = shadow.querySelector<HTMLSpanElement>(".name-error")!;
            this.#shellInput = field("shell");
            this.#titleInput = field("title");
            this.#wdInput = field("working-directory");
            this.#rootInput = field("root");
            this.#lineNumbers = shadow.querySelector<HTMLDivElement>(".line-numbers")!;
            this.#commandsArea = shadow.querySelector<HTMLTextAreaElement>(".commands-area")!;
            this.#deleteBtn = shadow.querySelector<HTMLButtonElement>(".delete-btn")!;
            this.#saveError = shadow.querySelector<HTMLSpanElement>(".save-error")!;
            this.#okBtn = shadow.querySelector<HTMLButtonElement>(".ok-btn")!;
            this.#cancelBtn = shadow.querySelector<HTMLButtonElement>(".cancel-btn")!;

            // --- Event listeners ---
            this.#nameInput.addEventListener("input", () => this.#validateForm());

            this.#commandsArea.addEventListener("input", () => this.#syncLineNumbers());
            this.#commandsArea.addEventListener("scroll", () => {
                this.#lineNumbers.scrollTop = this.#commandsArea.scrollTop;
            });

            this.#createCard.addEventListener("click", () => {
                this.#selectCreateCard();
            });

            this.#leftPanel.addEventListener("click", (e) => {
                const card = (e.target as HTMLElement).closest(".profile-card") as HTMLDivElement | null;
                if (!card) return;
                const name = (card as HTMLDivElement).dataset["profileName"];
                if (!name) return;
                this.#selectProfileCard(card, name);
            });

            this.#cancelBtn.addEventListener("click", () => this.close());
            this.#deleteBtn.addEventListener("click", () => void this.#handleDelete());
            this.#okBtn.addEventListener("click", () => void this.#handleOk());
        }

        open(profileNames: string[]): void {
            this.#selectedName = null;
            this.#selectedCard = null;
            this.#isLoading = false;
            this.#clearForm();
            this.#okBtn.disabled = true;
            this.#deleteBtn.style.display = "none";
            this.#saveError.classList.remove("visible");
            this.#nameInput.readOnly = false;

            for (const card of Array.from(this.#leftPanel.querySelectorAll(".profile-card"))) {
                card.remove();
            }
            this.#selectCreateCard();

            const filtered = profileNames.filter((n) => n !== "default");
            for (const name of filtered) {
                this.#leftPanel.appendChild(this.#makeProfileCard(name));
            }
            this.setAttribute("open", "");
        }

        close(): void {
            this.#selectedName = null;
            this.#nameError.classList.remove("visible");
            this.#saveError.classList.remove("visible");
            closeOverlay(this);
        }

        #makeProfileCard(name: string): HTMLDivElement {
            const card = document.createElement("div");
            card.className = "profile-card";
            card.dataset["profileName"] = name;
            const radio = document.createElement("input");
            radio.type = "radio";
            radio.name = "profile";
            const lbl = document.createElement("span");
            lbl.textContent = name;
            card.appendChild(radio);
            card.appendChild(lbl);
            return card;
        }

        #selectCreateCard(): void {
            for (const c of Array.from(this.#leftPanel.querySelectorAll(".profile-card"))) {
                c.removeAttribute("selected");
                (c.querySelector("input[type=radio]") as HTMLInputElement | null)!.checked = false;
            }
            this.#createCard.setAttribute("selected", "");
            this.#createRadio.checked = true;
            this.#selectedName = null;
            this.#selectedCard = null;
            this.#nameInput.readOnly = false;
            this.#deleteBtn.style.display = "none";
            this.#isLoading = false;
            this.#clearForm();
            this.#validateForm();
        }

        #selectProfileCard(card: HTMLDivElement, name: string): void {
            this.#createCard.removeAttribute("selected");
            this.#createRadio.checked = false;
            for (const c of Array.from(this.#leftPanel.querySelectorAll(".profile-card"))) {
                c.removeAttribute("selected");
                (c.querySelector("input[type=radio]") as HTMLInputElement | null)!.checked = false;
            }
            card.setAttribute("selected", "");
            (card.querySelector("input[type=radio]") as HTMLInputElement | null)!.checked = true;
            this.#selectedName = name;
            this.#selectedCard = card;
            this.#nameInput.value = name;
            this.#nameInput.readOnly = true;
            this.#deleteBtn.style.display = "";
            this.#isLoading = true;
            this.#okBtn.disabled = true;
            void this.#loadProfileConfig(name);
        }

        async #loadProfileConfig(name: string): Promise<void> {
            try {
                const cfg = await getProfileConfig(name);
                // Another card was selected while this profile loaded.
                if (this.#selectedName !== name) return;
                this.#shellInput.value = cfg.shell ?? "";
                this.#titleInput.value = cfg.title ?? "";
                this.#wdInput.value = cfg.workingDirectory ?? "";
                this.#rootInput.value = cfg.root ?? "";
                this.#commandsArea.value = (cfg.commands ?? []).join("\n");
                this.#syncLineNumbers();
            } catch {
                // Silently keep existing form state on load failure.
            } finally {
                if (this.#selectedName === name) {
                    this.#isLoading = false;
                    this.#validateForm();
                }
            }
        }

        #clearForm(): void {
            this.#nameInput.value = "";
            this.#shellInput.value = "";
            this.#titleInput.value = "";
            this.#wdInput.value = "";
            this.#rootInput.value = "";
            this.#commandsArea.value = "";
            this.#syncLineNumbers();
            this.#nameError.classList.remove("visible");
        }

        #syncLineNumbers(): void {
            const lineCount = this.#commandsArea.value === "" ? 1 : this.#commandsArea.value.split("\n").length;
            this.#lineNumbers.textContent = Array.from({ length: lineCount }, (_, i) => i + 1).join("\n");
            this.#lineNumbers.scrollTop = this.#commandsArea.scrollTop;
        }

        #validateForm(): void {
            if (this.#isLoading || this.#saving) {
                this.#okBtn.disabled = true;
                return;
            }
            const name = this.#nameInput.value.trim();
            if (name === "default") {
                this.#nameError.classList.add("visible");
                this.#okBtn.disabled = true;
                return;
            }
            this.#nameError.classList.remove("visible");
            this.#okBtn.disabled = name === "";
        }

        async #handleOk(): Promise<void> {
            const name = this.#nameInput.value.trim();
            if (!name || this.#saving) return;
            const rawCommands = this.#commandsArea.value.split("\n");
            const commands = rawCommands.filter((line) => line !== "");
            const profile: ProfileConfig = {
                shell: this.#shellInput.value.trim(),
                title: this.#titleInput.value.trim(),
                workingDirectory: this.#wdInput.value.trim(),
                root: this.#rootInput.value.trim(),
                commands,
            };
            this.#saveError.classList.remove("visible");
            this.#setSaving(true);
            try {
                const response = await postEditProfile(name, profile);
                this.dispatchEvent(
                    new CustomEvent("b3tty-profile-edited", {
                        detail: { name, response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.close();
            } catch (err) {
                // Keep editor open so user can retry, but say why the save
                // didn't visibly do anything.
                this.#saveError.textContent = err instanceof Error ? err.message : "Failed to save profile.";
                this.#saveError.classList.add("visible");
            } finally {
                this.#setSaving(false);
            }
        }

        async #handleDelete(): Promise<void> {
            const name = this.#selectedName;
            if (!name || this.#saving) return;
            this.#saveError.classList.remove("visible");
            this.#setSaving(true);
            try {
                const response = await postDeleteProfile(name);
                this.dispatchEvent(
                    new CustomEvent("b3tty-profile-edited", {
                        detail: { name: null, response },
                        bubbles: true,
                        composed: true,
                    })
                );
                this.close();
            } catch (err) {
                // Keep editor open so user can retry, but say why the delete
                // didn't visibly do anything.
                this.#saveError.textContent = err instanceof Error ? err.message : "Failed to delete profile.";
                this.#saveError.classList.add("visible");
            } finally {
                this.#setSaving(false);
            }
        }

        /**
         * Enters or leaves the saving state. While a request is in flight, OK, Delete, and
         * Cancel stay disabled, so a double click can't send it twice and the editor can't
         * close under it. Leaving it re-checks the form, unless the save closed the editor.
         */
        #setSaving(saving: boolean): void {
            this.#saving = saving;
            this.#cancelBtn.disabled = saving;
            this.#deleteBtn.disabled = saving;
            if (saving) this.#okBtn.disabled = true;
            else if (this.hasAttribute("open")) this.#validateForm();
        }
    }

    customElements.define("b3tty-profile-editor", B3ttyProfileEditorImpl);
}
