import { isThemeActivateResponse, isEditProfileResponse } from "./types.ts";
import type { ThemeActivateResponse, Palette, ProfileConfig, EditProfileResponse } from "./types.ts";

/**
 * POSTs to /add-theme to apply and persist the chosen theme.
 * Returns the activated theme config so the caller can apply it to the terminal.
 * Throws if the request fails or the response fails the type guard.
 */
export async function postAddTheme(name: string): Promise<ThemeActivateResponse> {
    const res = await fetch("/add-theme", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ theme: name }),
    });
    if (!res.ok) throw new Error(`Failed to select theme "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (!isThemeActivateResponse(parsed)) throw new Error(`Unexpected add-theme response shape`);
    return parsed;
}

/**
 * POSTs terminal dimensions to /size (URL pre-built by buildSizeUrl).
 * Throws if the server returns a non-ok status.
 */
export async function postSize(url: string): Promise<void> {
    const res = await fetch(url, { method: "POST" });
    if (!res.ok) throw new Error(`Failed to set terminal size: ${res.status}`);
}

/**
 * POSTs to /theme-config to activate the named theme.
 * Throws if the request fails or the response fails the type guard.
 */
export async function postThemeConfig(name: string): Promise<ThemeActivateResponse> {
    const res = await fetch(`/theme-config?name=${encodeURIComponent(name)}`, { method: "POST" });
    if (!res.ok) throw new Error(`Failed to activate theme "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (!isThemeActivateResponse(parsed)) throw new Error(`Unexpected theme-config response shape`);
    return parsed;
}

/**
 * GETs /theme?name=<name> to retrieve palette preview data.
 * Throws if the request fails or the response shape is invalid.
 */
export async function getThemePalette(name: string): Promise<Palette> {
    const res = await fetch(`/theme?name=${encodeURIComponent(name)}`);
    if (!res.ok) throw new Error(`Failed to fetch palette for theme "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (
        typeof parsed !== "object" ||
        parsed === null ||
        !Array.isArray((parsed as Record<string, unknown>)["normal"]) ||
        !Array.isArray((parsed as Record<string, unknown>)["bright"])
    ) {
        throw new Error(`Unexpected palette response shape for theme "${name}"`);
    }
    return parsed as Palette;
}

/**
 * POSTs to /save-config with the selected theme name.
 * Does not check the response status (fire-and-forget, caller handles reload).
 */
export async function postSaveConfig(theme: string): Promise<void> {
    await fetch("/save-config", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ theme }),
    });
}

/**
 * GETs /theme-config?name=<name> to retrieve all color fields for a theme.
 * Throws if the request fails or the response fails the type guard.
 */
export async function getThemeConfig(name: string): Promise<ThemeActivateResponse> {
    const res = await fetch(`/theme-config?name=${encodeURIComponent(name)}`);
    if (!res.ok) throw new Error(`Failed to fetch config for theme "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (!isThemeActivateResponse(parsed)) throw new Error(`Unexpected theme-config response shape`);
    return parsed;
}

/**
 * GETs /profile-config?name=<name> to retrieve all fields for a stored profile.
 * Throws if the request fails or the response shape is invalid.
 */
export async function getProfileConfig(name: string): Promise<ProfileConfig> {
    const res = await fetch(`/profile-config?name=${encodeURIComponent(name)}`);
    if (!res.ok) throw new Error(`Failed to fetch config for profile "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (
        typeof parsed !== "object" ||
        parsed === null ||
        !Array.isArray((parsed as Record<string, unknown>)["commands"])
    ) {
        throw new Error(`Unexpected profile-config response shape for profile "${name}"`);
    }
    return parsed as ProfileConfig;
}

/**
 * POSTs to /edit-profile to create or overwrite a named profile.
 * Returns the updated list of non-default profile names.
 * Throws if the request fails or the response fails the type guard.
 */
export async function postEditProfile(name: string, profile: ProfileConfig): Promise<EditProfileResponse> {
    const res = await fetch("/edit-profile", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, profile }),
    });
    if (!res.ok) throw new Error(`Failed to edit profile "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (!isEditProfileResponse(parsed)) throw new Error(`Unexpected edit-profile response shape`);
    return parsed;
}

/**
 * POSTs to /delete-profile to remove a named profile.
 * Returns the updated list of non-default profile names.
 * Throws if the request fails or the response fails the type guard.
 */
export async function postDeleteProfile(name: string): Promise<EditProfileResponse> {
    const res = await fetch("/delete-profile", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
    });
    if (!res.ok) throw new Error(`Failed to delete profile "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (!isEditProfileResponse(parsed)) throw new Error(`Unexpected delete-profile response shape`);
    return parsed;
}

/**
 * POSTs to /edit-theme to create or overwrite a theme with the given name and colors.
 * Returns the activated theme config so the caller can apply it to the terminal.
 * Throws if the request fails or the response fails the type guard.
 */
export async function postEditTheme(name: string, theme: Record<string, string>): Promise<ThemeActivateResponse> {
    const res = await fetch("/edit-theme", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, theme }),
    });
    if (!res.ok) throw new Error(`Failed to edit theme "${name}": ${res.status}`);
    const parsed: unknown = await res.json();
    if (!isThemeActivateResponse(parsed)) throw new Error(`Unexpected edit-theme response shape`);
    return parsed;
}
