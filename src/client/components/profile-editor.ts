import { getProfileConfig, postDeleteProfile, postEditProfile } from "../api.ts";
import type { EditProfileResponse } from "../types.ts";
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
import { askToDiscardChanges, buildPromptLayer, PROMPT_STYLES, type PromptLayer } from "./prompt-layer.ts";
import {
    canCopyProfile,
    canDeleteProfile,
    canResetProfile,
    canSaveProfile,
    copyProfileName,
    copyProfileNameProblem,
    hasAnyUnsavedChanges,
    initialProfileEditorState,
    isProfileEditorBusy,
    isReservedProfileName,
    newProfileHasUnsavedChanges,
    profileFromForm,
    profileHasUnsavedChanges,
    reduceProfileEditor,
    unsavedChanges,
    type ProfileEditorAction,
    type ProfileEditorState,
    type ProfileForm,
} from "./profile-editor-state.ts";

/**
 * Interface for the b3tty-profile-editor web component. Save, Copy…, and Delete… each
 * dispatch b3tty-profile-edited (bubbles + composed) with `{ name, response }` — `name`
 * null after a delete — and keep the editor open; Close asks before discarding unsaved
 * changes. close() closes unconditionally.
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

// Sized and spaced like the theme editor, so the two editors match.
const PROFILE_EDITOR_STYLES = `
    ${DESIGN_TOKENS}
    ${BASE_STYLES}
    ${OVERLAY_STYLES}
    ${TEXT_STYLES}
    .modal {
        padding: var(--space-2xl);
        display: flex; flex-direction: column; gap: var(--space-lg);
        width: min(803px, 100%); height: min(661px, 90vh);
        overflow: hidden;
    }
    .panels { flex: 1; min-height: 0; display: flex; flex-direction: row; }
    ${SPLIT_PANEL_STYLES}
    .left-panel { width: 252px; overflow: hidden; }
    /* The list's scrollbar overlays its content, so the list keeps a gutter on the right
       for it, clear of the selected card's outline; everything else in the panel leaves
       the same gap, so all the cards line up. */
    .left-panel > :not(.profile-list) { margin-right: var(--space-md); }
    .profile-list {
        flex: 1; min-height: 0; overflow-y: auto;
        display: flex; flex-direction: column; gap: var(--space-xs);
        padding-right: var(--space-md);
    }
    .list-footer {
        flex-shrink: 0; display: flex; gap: var(--space-xs);
        padding-top: var(--space-md); border-top: 1px solid var(--color-border-inner);
    }
    /* Same size as the actions row's Close and Save. */
    .list-footer .cancel-btn {
        flex: 1; padding-left: var(--space-sm); padding-right: var(--space-sm);
    }
    .list-footer .cancel-btn:disabled { background: var(--color-surface-2); }
    ${EDITOR_CARD_STYLES}
    .profile-card span:not(.badge) { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .create-card .badge, .profile-card .badge {
        margin-left: auto; flex-shrink: 0; font-weight: var(--font-weight-semibold);
    }
    .create-card .badge[hidden], .profile-card .badge[hidden] { display: none; }
    .name-section {
        display: flex; flex-direction: column; gap: var(--space-2xs);
        flex-shrink: 0; padding-bottom: var(--space-md);
        /* Matches .profile-form's right gutter, as in the theme editor. */
        padding-right: var(--space-md);
        border-bottom: 1px solid var(--color-border-inner);
        margin-bottom: var(--space-sm);
    }
    .name-row { display: flex; align-items: center; gap: var(--space-sm); }
    .name-section label { white-space: nowrap; min-width: 80px; }
    /* Lets the name field shrink to make room for Reset. */
    .name-row .name-input { min-width: 0; }
    .profile-form {
        flex: 1; min-height: 0;
        display: flex; flex-direction: column; gap: var(--space-2xs);
        padding-right: var(--space-md);
    }
    .field-row {
        display: grid;
        grid-template-columns: 140px 1fr;
        gap: var(--space-2xs); align-items: center;
    }
    .field-row label { white-space: nowrap; }
    .commands-section {
        flex: 1; display: flex; flex-direction: column; gap: var(--space-2xs);
        min-height: 0; padding-top: var(--space-sm);
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
    /* A successful save reuses the error's slot, in the accent color. */
    .save-error.notice { color: var(--color-accent); font-weight: var(--font-weight-semibold); }
    ${PROMPT_STYLES}
`;

/** The profile editor's text fields below the name, keyed by data-field. */
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

// The card badge for a profile with unsaved changes.
const UNSAVED_BADGE = "Unsaved";

// The profile cards are added to .profile-list by #renderCards (see #makeProfileCard),
// since their names are user data. The footer's Copy… and Delete… act on the selected
// profile, so both are disabled while "Create new profile" is selected.
const PROFILE_EDITOR_LEFT_PANEL = `
    <div class="left-panel">
        <div class="create-card" selected>
            <input type="radio" name="profile" checked>
            <span>Create new profile</span>
            <span class="badge accent" hidden>${UNSAVED_BADGE}</span>
        </div>
        <div class="profile-list"></div>
        <div class="list-footer">
            <button class="cancel-btn copy-btn" disabled>Copy…</button>
            <button class="cancel-btn delete-profile-btn" disabled>Delete…</button>
        </div>
    </div>
`;

// An existing profile's name is fixed, so the name field is only editable for "Create
// new profile". Reset, beside it, throws away the form's unsaved changes.
const PROFILE_EDITOR_RIGHT_PANEL = `
    <div class="right-panel">
        <div class="name-section">
            <div class="name-row">
                <label class="field-label">Profile Name</label>
                <input type="text" class="name-input" placeholder="Enter profile name" data-field="name">
                <button class="inline-btn reset-btn" disabled>Reset</button>
            </div>
            <span class="name-error field-error">Cannot use 'default' as a profile name</span>
        </div>
        <div class="profile-form">
            ${PROFILE_FIELDS.map(({ key, label, placeholder }) => profileField(key, label, placeholder)).join("")}
            <div class="commands-section">
                <div class="commands-label field-label">Commands (one per line, run on startup):</div>
                <div class="commands-wrapper">
                    <div class="line-numbers">1</div>
                    <textarea class="commands-area" placeholder="npm start" spellcheck="false" wrap="off"></textarea>
                </div>
            </div>
        </div>
        <div class="actions">
            <span class="save-error" role="status" aria-live="polite"></span>
            <button class="cancel-btn close-btn">Close</button>
            <button class="ok-btn" disabled>Save</button>
        </div>
    </div>
`;

if (typeof HTMLElement !== "undefined") {
    class B3ttyProfileEditorImpl extends HTMLElement implements B3ttyProfileEditor {
        #profileList: HTMLDivElement;
        #createCard: HTMLDivElement;
        #createRadio: HTMLInputElement;
        #createBadge: HTMLSpanElement;
        /** The form's inputs, keyed by the ProfileForm field each one edits. */
        #inputs: Map<keyof ProfileForm, HTMLInputElement | HTMLTextAreaElement> = new Map();
        #nameError: HTMLSpanElement;
        #commandsArea: HTMLTextAreaElement;
        #lineNumbers: HTMLDivElement;
        #saveBtn: HTMLButtonElement;
        #closeBtn: HTMLButtonElement;
        #resetBtn: HTMLButtonElement;
        #copyBtn: HTMLButtonElement;
        #deleteBtn: HTMLButtonElement;
        #saveError: HTMLSpanElement;
        #prompt: PromptLayer;
        #state: ProfileEditorState = initialProfileEditorState;

        constructor() {
            super();
            const shadow = this.attachShadow({ mode: "open" });
            shadow.innerHTML = `
                <style>${PROFILE_EDITOR_STYLES}</style>
                <div class="overlay">
                    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="profile-editor-title">
                        <h2 class="modal-title" id="profile-editor-title">Edit Profile</h2>
                        <div class="panels">
                            ${PROFILE_EDITOR_LEFT_PANEL}
                            ${PROFILE_EDITOR_RIGHT_PANEL}
                        </div>
                    </div>
                </div>
            `;
            const field = (key: string) => shadow.querySelector<HTMLInputElement>(`[data-field="${key}"]`)!;
            this.#profileList = shadow.querySelector<HTMLDivElement>(".profile-list")!;
            this.#createCard = shadow.querySelector<HTMLDivElement>(".create-card")!;
            this.#createRadio = this.#createCard.querySelector<HTMLInputElement>("input")!;
            this.#createBadge = this.#createCard.querySelector<HTMLSpanElement>(".badge")!;
            this.#copyBtn = shadow.querySelector<HTMLButtonElement>(".copy-btn")!;
            this.#deleteBtn = shadow.querySelector<HTMLButtonElement>(".delete-profile-btn")!;
            this.#nameError = shadow.querySelector<HTMLSpanElement>(".name-error")!;
            this.#resetBtn = shadow.querySelector<HTMLButtonElement>(".reset-btn")!;
            this.#lineNumbers = shadow.querySelector<HTMLDivElement>(".line-numbers")!;
            this.#commandsArea = shadow.querySelector<HTMLTextAreaElement>(".commands-area")!;
            this.#saveError = shadow.querySelector<HTMLSpanElement>(".right-panel .save-error")!;
            this.#saveBtn = shadow.querySelector<HTMLButtonElement>(".right-panel .ok-btn")!;
            this.#closeBtn = shadow.querySelector<HTMLButtonElement>(".right-panel .close-btn")!;
            this.#inputs.set("name", field("name"));
            this.#inputs.set("shell", field("shell"));
            this.#inputs.set("title", field("title"));
            this.#inputs.set("workingDirectory", field("working-directory"));
            this.#inputs.set("root", field("root"));
            this.#inputs.set("commands", this.#commandsArea);

            // Last, since setting the template would replace it.
            this.#prompt = buildPromptLayer(shadow);

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
            this.#profileList.addEventListener("click", (e) => {
                const card = (e.target as HTMLElement).closest<HTMLDivElement>(".profile-card");
                const name = card?.dataset["profileName"];
                if (name) void this.#selectProfile(name);
            });
            this.#copyBtn.addEventListener("click", () => void this.#copySelected());
            this.#deleteBtn.addEventListener("click", () => void this.#deleteSelected());
            this.#resetBtn.addEventListener("click", () => this.#dispatch({ type: "resetRequested" }));
            this.#closeBtn.addEventListener("click", () => void this.#requestClose());
            this.#saveBtn.addEventListener("click", () => void this.#handleSave());
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
            this.#createBadge.hidden = !newProfileHasUnsavedChanges(state);
            for (const card of this.#cards()) {
                const name = card.dataset["profileName"]!;
                const selected = name === state.selected;
                card.toggleAttribute("selected", selected);
                card.querySelector<HTMLInputElement>("input")!.checked = selected;
                card.querySelector<HTMLSpanElement>(".badge")!.hidden = !profileHasUnsavedChanges(state, name);
            }

            for (const [key, input] of this.#inputs) {
                // Values are set only when they differ, so typing isn't disturbed.
                if (input.value !== state.form[key]) input.value = state.form[key];
                // An existing profile's name is fixed; nothing changes while a save is in flight.
                input.readOnly = busy || (key === "name" && state.selected !== null);
            }
            this.#syncLineNumbers();
            this.#nameError.classList.toggle("visible", isReservedProfileName(state.form.name));

            const status = state.error ?? (state.notice ? `✓ ${state.notice}` : null);
            this.#saveError.textContent = status ?? "";
            this.#saveError.classList.toggle("visible", status !== null);
            this.#saveError.classList.toggle("notice", state.error === null && state.notice !== null);

            this.#copyBtn.disabled = !canCopyProfile(state);
            this.#deleteBtn.disabled = !canDeleteProfile(state);
            this.#resetBtn.disabled = !canResetProfile(state);
            // Close too, so the editor can't close and reopen under a save in flight.
            this.#closeBtn.disabled = busy;
            this.#saveBtn.disabled = !canSaveProfile(state);
        }

        #cards(): HTMLDivElement[] {
            return Array.from(this.#profileList.querySelectorAll<HTMLDivElement>(".profile-card"));
        }

        #cardFor(name: string): HTMLDivElement | undefined {
            return this.#cards().find((card) => card.dataset["profileName"] === name);
        }

        /** Rebuilds the profile cards when the list of names has changed. */
        #renderCards(): void {
            const rendered = this.#cards().map((card) => card.dataset["profileName"]);
            const names = this.#state.profileNames;
            if (rendered.length === names.length && rendered.every((name, i) => name === names[i])) return;
            this.#profileList.replaceChildren(...names.map((name) => this.#makeProfileCard(name)));
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
            lbl.title = name;
            const badge = document.createElement("span");
            badge.className = "badge accent";
            badge.textContent = UNSAVED_BADGE;
            badge.hidden = true;
            card.append(radio, lbl, badge);
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
            if (this.#state !== before && this.#state.phase === "loading") await this.#loadProfile(name);
        }

        async #loadProfile(name: string): Promise<void> {
            try {
                const profile = await getProfileConfig(name);
                this.#dispatch({ type: "profileLoaded", name, profile });
            } catch (err) {
                const message = err instanceof Error ? err.message : String(err);
                this.#dispatch({ type: "profileLoadFailed", name, message });
            }
        }

        /** Tells terminal.ts that the profiles changed, with the updated names. */
        #profileEdited(name: string | null, response: EditProfileResponse): void {
            this.dispatchEvent(
                new CustomEvent("b3tty-profile-edited", {
                    detail: { name, response },
                    bubbles: true,
                    composed: true,
                })
            );
        }

        /** Applies a copy's or delete's updated names, selecting `select` and loading it when needed. */
        async #profilesChanged(profileNames: string[], select?: string): Promise<void> {
            this.#dispatch({ type: "profilesChanged", profileNames, select });
            const state = this.#state;
            if (state.selected === null) return;
            this.#cardFor(state.selected)?.scrollIntoView({ block: "nearest" });
            if (state.phase === "loading") await this.#loadProfile(state.selected);
        }

        async #copySelected(): Promise<void> {
            const name = this.#state.selected;
            if (name === null || !canCopyProfile(this.#state)) return;
            const bold = document.createElement("b");
            bold.textContent = name;
            await this.#prompt.ask({
                title: "Copy Profile",
                message: ["Save a copy of ", bold, " under a new name."],
                input: {
                    label: "New profile name",
                    value: copyProfileName(this.#state.profileNames, name),
                    validate: (value) => copyProfileNameProblem(this.#state.profileNames, value),
                },
                actionLabel: "Copy",
                opener: this.#copyBtn,
                submit: async (newName) => {
                    // The saved profile is copied, not unsaved edits, as in the theme editor.
                    const profile = await getProfileConfig(name);
                    const response = await postEditProfile(newName, profile);
                    this.#profileEdited(newName, response);
                    await this.#profilesChanged(response.profileNames, newName);
                },
            });
        }

        async #deleteSelected(): Promise<void> {
            const name = this.#state.selected;
            if (name === null || !canDeleteProfile(this.#state)) return;
            const bold = document.createElement("b");
            bold.textContent = name;
            await this.#prompt.ask({
                title: "Delete Profile",
                message: ["Delete ", bold, "? This can't be undone."],
                actionLabel: "Delete",
                destructive: true,
                opener: this.#deleteBtn,
                submit: async () => {
                    const response = await postDeleteProfile(name);
                    this.#profileEdited(null, response);
                    await this.#profilesChanged(response.profileNames);
                },
            });
        }

        /** Closes, first asking to discard unsaved changes when there are any. */
        async #requestClose(): Promise<void> {
            const state = this.#state;
            if (isProfileEditorBusy(state)) return;
            if (!hasAnyUnsavedChanges(state)) {
                this.close();
                return;
            }
            const { names, newProfile } = unsavedChanges(state);
            const discard = await askToDiscardChanges(
                this.#prompt,
                names,
                newProfile ? "a new profile" : null,
                this.#closeBtn
            );
            if (discard) this.close();
        }

        async #handleSave(): Promise<void> {
            const before = this.#state;
            this.#dispatch({ type: "saveSubmitted" });
            if (this.#state === before) return;
            const name = this.#state.form.name.trim();
            try {
                const response = await postEditProfile(name, profileFromForm(this.#state.form));
                this.#profileEdited(name, response);
                this.#dispatch({ type: "saveSucceeded", name, profileNames: response.profileNames });
                // A profile just saved for the first time gets a card; bring it into view.
                this.#cardFor(name)?.scrollIntoView({ block: "nearest" });
            } catch (err) {
                // Keep editor open so the user can retry, but say why the save
                // didn't visibly do anything.
                const message = err instanceof Error ? err.message : "Failed to save profile.";
                this.#dispatch({ type: "saveFailed", message });
            }
        }
    }

    customElements.define("b3tty-profile-editor", B3ttyProfileEditorImpl);
}
