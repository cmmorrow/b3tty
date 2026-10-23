interface ThemeConfigBase {
    foreground?: string;
    background?: string;
    cursor?: string;
    cursorAccent?: string;
    black?: string;
    brightBlack?: string;
    red?: string;
    brightRed?: string;
    green?: string;
    brightGreen?: string;
    yellow?: string;
    brightYellow?: string;
    blue?: string;
    brightBlue?: string;
    magenta?: string;
    brightMagenta?: string;
    cyan?: string;
    brightCyan?: string;
    white?: string;
    brightWhite?: string;
    selectionForeground?: string;
    selectionBackground?: string;
    /** Server-side file path of the theme's background image. */
    backgroundImage?: string;
    /**
     * 0–100: how strongly the theme background tints the background image
     * (alpha = value / 100, so higher is fainter). Absent means
     * DEFAULT_BACKGROUND_IMAGE_TRANSPARENCY.
     */
    backgroundImageTransparency?: number;
}

export interface ThemeConfig extends ThemeConfigBase {
    [key: string]: string | number | undefined;
}

/** Background image transparency used when a theme doesn't set one (alpha 0.5). */
export const DEFAULT_BACKGROUND_IMAGE_TRANSPARENCY = 50;

export interface TermConfig {
    tls: boolean;
    uri: string;
    port: number;
    fontSize: number;
    fontFamily: string;
    autoResize?: boolean;
    rows: number;
    columns: number;
    theme: ThemeConfig;
    debug?: boolean;
    backgroundImage?: boolean;
    /** The Themes menu, in menu order. */
    themeNames?: string[];
    /** Every theme of every source, sorted by name. */
    themes?: ThemeEntry[];
    profileNames?: string[];
    activeTheme?: string;
    showMenubar?: string;
}

/** Where a theme comes from: built into b3tty, installed from a package, or the user's own. */
export type ThemeSource = "builtin" | "installed" | "user";

/**
 * One theme as the server describes it (ThemeEntry in src/theme_catalog.go). author,
 * themeVersion, and repo are set only for installed themes; repo was validated as an
 * https URL on import. modified marks an installed theme whose package changed since
 * it was installed. Only user themes can be edited or exported.
 */
export interface ThemeEntry {
    name: string;
    source: ThemeSource;
    inMenu: boolean;
    author?: string;
    themeVersion?: string;
    repo?: string;
    modified?: boolean;
}

/**
 * The theme lists the server pushes to every session in a "themes" control message
 * when themes are added or removed (see broadcastThemeLists), and returns from the
 * endpoints that change them, with the same meaning as the TermConfig fields of the
 * same names.
 */
export interface ThemeLists {
    themeNames: string[];
    themes: ThemeEntry[];
}

export interface ThemeActivateResponse extends ThemeConfigBase {
    hasBackgroundImage: boolean;
    themeNames?: string[];
    themes?: ThemeEntry[];
}

/** Response of POST /import-theme. */
export interface ImportThemeResponse extends ThemeLists {
    name: string;
    theme: ThemeActivateResponse;
    upgraded: boolean;
    unchanged: boolean;
}

/** Runtime type guard for ThemeLists, and so for the delete-theme and reload-themes responses. */
export function isThemeLists(val: unknown): val is ThemeLists {
    if (typeof val !== "object" || val === null) return false;
    const r = val as Record<string, unknown>;
    return Array.isArray(r["themeNames"]) && Array.isArray(r["themes"]);
}

/** Runtime type guard for ImportThemeResponse. */
export function isImportThemeResponse(val: unknown): val is ImportThemeResponse {
    if (!isThemeLists(val)) return false;
    const r = val as unknown as Record<string, unknown>;
    return typeof r["name"] === "string" && isThemeActivateResponse(r["theme"]);
}

/**
 * Runtime type guard for ThemeActivateResponse. Validates the minimum required shape
 * of a parsed JSON response before it is used as a ThemeActivateResponse.
 */
export function isThemeActivateResponse(val: unknown): val is ThemeActivateResponse {
    return (
        typeof val === "object" &&
        val !== null &&
        typeof (val as Record<string, unknown>)["hasBackgroundImage"] === "boolean"
    );
}

export interface ProfileConfig {
    shell: string;
    workingDirectory: string;
    title: string;
    root: string;
    commands: string[];
}

export interface EditProfileResponse {
    profileNames: string[];
}

export function isEditProfileResponse(val: unknown): val is EditProfileResponse {
    return typeof val === "object" && val !== null && Array.isArray((val as Record<string, unknown>)["profileNames"]);
}

/**
 * Information recorded in an exported .b3theme package's manifest
 * (ThemeMetadata in src/theme_package.go). Every field is required; the server
 * trims them, repo must be an https URL, and themeVersion is free-form. The
 * server saves them with the theme after each export, and GET /theme-package
 * returns them (or {} for a theme never exported) to fill in the next one.
 */
export interface ThemeMetadata {
    repo: string;
    author: string;
    description: string;
    themeVersion: string;
}

/** Response of POST /export-theme: the path the .b3theme package was written to. */
export interface ExportThemeResponse {
    path: string;
}

export function isExportThemeResponse(val: unknown): val is ExportThemeResponse {
    return typeof val === "object" && val !== null && typeof (val as Record<string, unknown>)["path"] === "string";
}

export interface ClientConfig {
    fontFamily: string;
    fontSize: number;
    rows: number;
    columns: number;
}

export interface SocketLike {
    readyState: number;
    send(data: string): void;
}

export interface BellElementLike {
    style: { display: string };
}

export interface TerminalLike {
    _initialized?: boolean;
    write(data: string, callback?: () => void): void;
    writeln(data: string): void;
    onData(listener: (data: string) => void): void;
    onBell(listener: () => void): void;
}

export interface SocketMessageEvent {
    data: ArrayBuffer | string;
}

export interface Palette {
    bg: string;
    fg: string;
    selBg: string;
    cursor: string;
    normal: string[];
    bright: string[];
}

export interface SettingsServerConfig {
    port: number;
    noAuth: boolean;
    noBrowser: boolean;
    showMenubar: "hover" | "visible" | "disable";
}

export interface TerminalClient {
    fontFamily: string;
    fontSize: number;
    autoResize: boolean;
    rows: number;
    columns: number;
}

export interface SettingsConfig {
    server: SettingsServerConfig;
    terminal: TerminalClient;
}

export function isSettingsConfig(val: unknown): val is SettingsConfig {
    if (typeof val !== "object" || val === null) return false;
    const v = val as Record<string, unknown>;
    return (
        typeof v["server"] === "object" &&
        v["server"] !== null &&
        typeof v["terminal"] === "object" &&
        v["terminal"] !== null
    );
}

declare global {
    interface Window {
        B3TTY?: TermConfig;
    }
}

/**
 * One entry in the command palette. `id` is what the palette reports back when
 * the command is run; `label` is what is shown and searched; `group` is the
 * short category shown at the right of the row (e.g. "Themes").
 */
export interface PaletteCommand {
    id: string;
    label: string;
    group: string;
}
