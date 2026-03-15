export interface ThemeConfig {
    foreground?: string;
    background?: string;
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

declare global {
    interface Window {
        B3TTY?: TermConfig;
    }
}
