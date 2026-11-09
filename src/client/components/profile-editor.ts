import { getProfileConfig, postDeleteProfile, postEditProfile } from "../api.ts";
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
import {
    canDeleteProfile,
    canSaveProfile,
    initialProfileEditorState,
    isProfileEditorBusy,
    isReservedProfileName,
    profileFromForm,
    reduceProfileEditor,
    type ProfileEditorAction,
    type ProfileEditorState,
    type ProfileForm,
} from "./profile-editor-state.ts";

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

// The profile cards are added by #renderCards (see #makeProfileCard), since their
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
        #leftPanel: HTMLDivElement;
        #createCard: HTMLDivElement;
        #createRadio: HTMLInputElement;
        /** The form's inputs, keyed by the ProfileForm field each one edits. */
        #inputs: Map<keyof ProfileForm, HTMLInputElement | HTMLTextAreaElement> = new Map();
        #nameError: HTMLSpanElement;
        #commandsArea: HTMLTextAreaElement;
        #lineNumbers: HTMLDivElement;
        #okBtn: HTMLButtonElement;
        #cancelBtn: HTMLButtonElement;
        #deleteBtn: HTMLButtonElement;
        #saveError: HTMLSpanElement;
        #state: ProfileEditorState = initialProfileEditorState;

        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });
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
            this.#nameError = shadow.querySelector<HTMLSpanElement>(".name-error")!;
            this.#lineNumbers = shadow.querySelector<HTMLDivElement>(".line-numbers")!;
            this.#commandsArea = shadow.querySelector<HTMLTextAreaElement>(".commands-area")!;
            this.#deleteBtn = shadow.querySelector<HTMLButtonElement>(".delete-btn")!;
            this.#saveError = shadow.querySelector<HTMLSpanElement>(".save-error")!;
            this.#okBtn = shadow.querySelector<HTMLButtonElement>(".ok-btn")!;
            this.#cancelBtn = shadow.querySelector<HTMLButtonElement>(".cancel-btn")!;
            this.#inputs.set("name", field("name"));
            this.#inputs.set("shell", field("shell"));
            this.#inputs.set("title", field("title"));
            this.#inputs.set("workingDirectory", field("working-directory"));
            this.#inputs.set("root", field("root"));
            this.#inputs.set("commands", this.#commandsArea);

            // --- Event listeners ---
            for (const [key, input] of this.#inputs) {
                input.addEventListener("input", () =>
                    this.#dispatch({ type: "fieldChanged", field: key, value: input.value })
                );
            }
            this.#commandsArea.addEventListener("scroll", () => {
                this.#lineNumbers.scrollTop = this.#commandsArea.scrollTop;
            });
            this.#createCard.addEventListener("click", () => this.#dispatch({ type: "createSelected" }));
            this.#leftPanel.addEventListener("click", (e) => {
                const card = (e.target as HTMLElement).closest<HTMLDivElement>(".profile-card");
                const name = card?.dataset["profileName"];
                if (name) void this.#selectProfile(name);
            });
            this.#cancelBtn.addEventListener("click", () => this.close());
            this.#deleteBtn.addEventListener("click", () => void this.#handleDelete());
            this.#okBtn.addEventListener("click", () => void this.#handleOk());
            this.#render();
        }

        open(profileNames: string[]): void {
            this.#dispatch({ type: "opened", profileNames });
            this.setAttribute("open", "");
        }

        close(): void {
            closeOverlay(this);
        }

        #dispatch(action: ProfileEditorAction): void {
            this.#state = reduceProfileEditor(this.#state, action);
            this.#render();
        }

        /** Brings the DOM in line with #state. Safe to call at any time. */
        #render(): void {
            const state = this.#state;
            const busy = isProfileEditorBusy(state);

            this.#renderCards();
            this.#createRadio.checked = state.selected === null;
            this.#createCard.toggleAttribute("selected", state.selected === null);
            for (const card of Array.from(this.#leftPanel.querySelectorAll<HTMLDivElement>(".profile-card"))) {
                const selected = card.dataset["profileName"] === state.selected;
                card.toggleAttribute("selected", selected);
                card.querySelector<HTMLInputElement>("input")!.checked = selected;
            }

            for (const [key, input] of this.#inputs) {
                if (input.value !== state.form[key]) input.value = state.form[key];
                // An existing profile's name is fixed; nothing changes while a request is in flight.
                input.readOnly = busy || (key === "name" && state.selected !== null);
            }
            this.#syncLineNumbers();
            this.#nameError.classList.toggle("visible", isReservedProfileName(state.form.name));

            this.#saveError.textContent = state.error ?? "";
            this.#saveError.classList.toggle("visible", state.error !== null);
            this.#deleteBtn.style.display = state.selected === null ? "none" : "";
            this.#deleteBtn.disabled = !canDeleteProfile(state);
            // Cancel too, so the editor can't close and reopen under a request in flight.
            this.#cancelBtn.disabled = busy;
            this.#okBtn.disabled = !canSaveProfile(state);
        }

        /** Rebuilds the profile cards when the list of names has changed. */
        #renderCards(): void {
            const cards = Array.from(this.#leftPanel.querySelectorAll<HTMLDivElement>(".profile-card"));
            const rendered = cards.map((card) => card.dataset["profileName"]);
            const names = this.#state.profileNames;
            if (rendered.length === names.length && rendered.every((name, i) => name === names[i])) return;
            for (const card of cards) card.remove();
            for (const name of names) this.#leftPanel.appendChild(this.#makeProfileCard(name));
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

        #syncLineNumbers(): void {
            const lineCount = this.#commandsArea.value === "" ? 1 : this.#commandsArea.value.split("\n").length;
            this.#lineNumbers.textContent = Array.from({ length: lineCount }, (_, i) => i + 1).join("\n");
            this.#lineNumbers.scrollTop = this.#commandsArea.scrollTop;
        }

        async #selectProfile(name: string): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "profileSelected", name });
            if (this.#state === before) return;
            try {
                const profile = await getProfileConfig(name);
                this.#dispatch({ type: "profileLoaded", name, profile });
            } catch (err) {
                const message = err instanceof Error ? err.message : String(err);
                this.#dispatch({ type: "profileLoadFailed", name, message });
            }
        }

        async #handleOk(): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "saveSubmitted" });
            if (this.#state === before) return;
            const name = this.#state.form.name.trim();
            try {
                const response = await postEditProfile(name, profileFromForm(this.#state.form));
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
                const message = err instanceof Error ? err.message : "Failed to save profile.";
                this.#dispatch({ type: "requestFailed", message });
            }
        }

        async #handleDelete(): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "deleteSubmitted" });
            if (this.#state === before || this.#state.selected === null) return;
            try {
                const response = await postDeleteProfile(this.#state.selected);
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
                const message = err instanceof Error ? err.message : "Failed to delete profile.";
                this.#dispatch({ type: "requestFailed", message });
            }
        }
    }

    customElements.define("b3tty-profile-editor", B3ttyProfileEditorImpl);
}
