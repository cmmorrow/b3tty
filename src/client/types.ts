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
    themeNames?: string[];
    allThemeNames?: string[];
    builtinThemeNames?: string[];
    profileNames?: string[];
    activeTheme?: string;
    showMenubar?: string;
}

export interface ThemeActivateResponse extends ThemeConfigBase {
    hasBackgroundImage: boolean;
    themeNames?: string[];
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
