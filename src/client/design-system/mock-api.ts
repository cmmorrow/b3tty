/**
 * Stand-in for ../api.ts with the same exports, answered from fixture data
 * instead of the b3tty server. build.ts swaps it in for every import of
 * ./api.ts, so the real components can open, load and save inside a design
 * system preview that has no server behind it.
 */
import THEMES from "b3tty:builtin-themes";
import type { EditProfileResponse, Palette, ProfileConfig, SettingsConfig, ThemeActivateResponse } from "../types.ts";

type ThemeFixture = Record<string, string | number>;

const wait = (ms = 120): Promise<void> => new Promise((r) => setTimeout(r, ms));

/** User-defined themes, so the theme editor has an existing entry to edit. */
export const USER_THEMES: Record<string, ThemeFixture> = {
    "midnight-ops": {
        ...THEMES["tokyo-night"],
        background: "#0f1117",
        backgroundImage: "~/Pictures/grid.png",
        backgroundImageTransparency: 70,
    },
};

const profiles: Record<string, ProfileConfig> = {
    "api-server": {
        shell: "$SHELL",
        workingDirectory: "~/repos/api",
        title: "API",
        root: "",
        commands: ["source .venv/bin/activate", "make run"],
    },
    logs: {
        shell: "/bin/zsh",
        workingDirectory: "/var/log",
        title: "Logs",
        root: "",
        commands: ["tail -f system.log"],
    },
};

export const PROFILE_NAMES = ["default", ...Object.keys(profiles)];

export const SAMPLE_SETTINGS: SettingsConfig = {
    server: { port: 8080, noAuth: false, noBrowser: false, showMenubar: "hover" },
    terminal: { fontFamily: "JetBrains Mono", fontSize: 14, autoResize: true, rows: 24, columns: 80 },
};

let settings: SettingsConfig = structuredClone(SAMPLE_SETTINGS);

function findTheme(name: string): ThemeFixture {
    const t = USER_THEMES[name] ?? THEMES[name];
    if (!t) throw new Error(`Unknown theme "${name}"`);
    return t;
}

// ANSI display order, matching themePaletteHandler.
const ANSI_ORDER = ["black", "red", "yellow", "green", "cyan", "blue", "magenta", "white"];

export function paletteOf(t: ThemeFixture): Palette {
    const color = (key: string): string => String(t[key] ?? "");
    return {
        bg: color("background"),
        fg: color("foreground"),
        selBg: color("selectionBackground"),
        cursor: color("cursor"),
        normal: ANSI_ORDER.map(color),
        bright: ANSI_ORDER.map((k) => color("bright" + k.charAt(0).toUpperCase() + k.slice(1))),
    };
}

export async function getThemePalette(name: string): Promise<Palette> {
    await wait();
    return paletteOf(findTheme(name));
}

export async function getThemeConfig(name: string): Promise<ThemeActivateResponse> {
    await wait();
    const t = findTheme(name);
    return { ...t, hasBackgroundImage: !!t["backgroundImage"] };
}

export const postThemeConfig = getThemeConfig;
export const postAddTheme = getThemeConfig;

export async function postSaveConfig(_theme: string): Promise<void> {
    await wait();
}

export async function postEditTheme(name: string, theme: ThemeFixture): Promise<ThemeActivateResponse> {
    await wait(300);
    const image = String(theme["backgroundImage"] ?? "");
    // Mirrors the server's image-type check so the editor's error state can be previewed.
    if (image && !/\.(png|jpe?g|gif|webp)$/i.test(image)) {
        throw new Error(`background image ${image}: unsupported image type`);
    }
    USER_THEMES[name] = { ...theme };
    return { ...theme, hasBackgroundImage: !!image, themeNames: Object.keys(USER_THEMES).sort() };
}

export async function getProfileConfig(name: string): Promise<ProfileConfig> {
    await wait();
    const p = profiles[name];
    if (!p) throw new Error(`Unknown profile "${name}"`);
    return p;
}

export async function postEditProfile(name: string, profile: ProfileConfig): Promise<EditProfileResponse> {
    await wait(300);
    profiles[name] = profile;
    return { profileNames: Object.keys(profiles).sort() };
}

export async function postDeleteProfile(name: string): Promise<EditProfileResponse> {
    await wait(300);
    delete profiles[name];
    return { profileNames: Object.keys(profiles).sort() };
}

export async function getSettings(): Promise<SettingsConfig> {
    await wait();
    return structuredClone(settings);
}

export async function postSettings(s: SettingsConfig): Promise<SettingsConfig> {
    await wait(300);
    settings = structuredClone(s);
    return structuredClone(settings);
}
