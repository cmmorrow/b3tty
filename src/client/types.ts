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
}

export interface ThemeConfig extends ThemeConfigBase {
    [key: string]: string | undefined;
}

export interface TermConfig {
    tls: boolean;
    uri: string;
    port: number;
    fontSize: number;
    fontFamily: string;
    cursorBlink: boolean;
    rows: number;
    columns: number;
    theme: ThemeConfig;
    debug?: boolean;
    backgroundImage?: boolean;
    themeNames?: string[];
    allThemeNames?: string[];
    profileNames?: string[];
    activeTheme?: string;
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

export interface ClientConfig {
    cursorBlink: boolean;
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

declare global {
    interface Window {
        B3TTY?: TermConfig;
    }
}
