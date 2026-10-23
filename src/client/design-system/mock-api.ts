/**
 * Stand-in for ../api.ts with the same exports, answered from fixture data
 * instead of the b3tty server. build.ts swaps it in for every import of
 * ./api.ts, so the real components can open, load and save inside a design
 * system preview that has no server behind it.
 */
import THEMES from "b3tty:builtin-themes";
import type {
    EditProfileResponse,
    ExportThemeResponse,
    ImportThemeResponse,
    Palette,
    ProfileConfig,
    SettingsConfig,
    ThemeActivateResponse,
    ThemeEntry,
    ThemeLists,
    ThemeMetadata,
} from "../types.ts";

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

/**
 * Installed themes with their package details. aurora's package is flagged as
 * modified, so the picker's and editor's "Modified" badge can be previewed.
 */
const INSTALLED_THEMES: Record<
    string,
    { theme: ThemeFixture; details: Omit<ThemeEntry, "name" | "source" | "inMenu"> }
> = {
    aurora: {
        theme: { ...THEMES["catppuccin-mocha"], background: "#101820", cursor: "#7fdbca" },
        details: {
            author: "Northern Lights Co.",
            themeVersion: "1.4.0",
            repo: "https://github.com/example/aurora-theme",
            modified: true,
        },
    },
    "paper-white": {
        theme: { ...THEMES["one-light"], background: "#fbfaf7" },
        details: { author: "Jane Doe", themeVersion: "2.0", repo: "https://github.com/example/paper-white" },
    },
};

/** The Themes menu, in order. */
let menu = ["midnight-ops", "dracula", "aurora"];

/** Every theme as the server describes it (see ThemeEntry), sorted by name. */
export function themeEntries(): ThemeEntry[] {
    const entries: ThemeEntry[] = [
        ...Object.keys(THEMES).map((name) => ({ name, source: "builtin" as const, inMenu: menu.includes(name) })),
        ...Object.entries(INSTALLED_THEMES).map(([name, { details }]) => ({
            name,
            source: "installed" as const,
            inMenu: menu.includes(name),
            ...details,
        })),
        ...Object.keys(USER_THEMES).map((name) => ({ name, source: "user" as const, inMenu: menu.includes(name) })),
    ];
    return entries.sort((a, b) => a.name.localeCompare(b.name));
}

function themeLists(): ThemeLists {
    return { themeNames: [...menu], themes: themeEntries() };
}

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
    const t = USER_THEMES[name] ?? INSTALLED_THEMES[name]?.theme ?? THEMES[name];
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
    if (!menu.includes(name)) menu = [...menu, name];
    return { ...theme, hasBackgroundImage: !!image, ...themeLists() };
}

/** Same shape as ../api.ts's ThemeUpgradeRequiredError, which components check with instanceof. */
export class ThemeUpgradeRequiredError extends Error {
    constructor(message: string) {
        super(message);
        this.name = "ThemeUpgradeRequiredError";
    }
}

/**
 * Packages the import dialog can preview: aurora-2.0 is a new version of an installed
 * theme, so it asks to Replace first; sunrise is a new theme. Anything else fails like
 * a missing file.
 */
const IMPORTABLE: Record<
    string,
    { name: string; theme: ThemeFixture; details: { author: string; themeVersion: string; repo: string } }
> = {
    "~/Downloads/aurora-2.0.b3theme": {
        name: "aurora",
        theme: { ...THEMES["catppuccin-mocha"], background: "#0c141c", cursor: "#7fdbca" },
        details: {
            author: "Northern Lights Co.",
            themeVersion: "2.0.0",
            repo: "https://github.com/example/aurora-theme",
        },
    },
    "~/Downloads/sunrise.b3theme": {
        name: "sunrise",
        theme: { ...THEMES["gruvbox-light"], background: "#fff4e0" },
        details: { author: "Dawn Studio", themeVersion: "0.9", repo: "https://github.com/example/sunrise" },
    },
};

export async function postImportTheme(path: string, replace = false): Promise<ImportThemeResponse> {
    await wait(300);
    const pkg = IMPORTABLE[path.trim()];
    if (!pkg) throw new Error(`package: open ${path.trim()}: no such file or directory`);
    const installed = INSTALLED_THEMES[pkg.name];
    if (installed && !replace) {
        throw new ThemeUpgradeRequiredError(
            `${pkg.name} ${installed.details.themeVersion} is installed; this package is version ${pkg.details.themeVersion}.`
        );
    }
    INSTALLED_THEMES[pkg.name] = { theme: pkg.theme, details: { ...pkg.details } };
    return {
        name: pkg.name,
        theme: { ...pkg.theme, hasBackgroundImage: false },
        upgraded: !!installed,
        unchanged: false,
        ...themeLists(),
    };
}

export async function postCopyTheme(name: string, newName: string): Promise<ThemeActivateResponse> {
    await wait(300);
    if (USER_THEMES[newName] || INSTALLED_THEMES[newName] || THEMES[newName]) {
        throw new Error(`a theme named "${newName}" already exists`);
    }
    const theme = { ...findTheme(name) };
    USER_THEMES[newName] = theme;
    menu = [...menu, newName];
    return { ...theme, hasBackgroundImage: !!theme["backgroundImage"], ...themeLists() };
}

export async function postDeleteTheme(name: string): Promise<ThemeLists> {
    await wait(300);
    delete USER_THEMES[name];
    delete INSTALLED_THEMES[name];
    menu = menu.filter((n) => n !== name);
    return themeLists();
}

/** Paths the mock treats as already taken, so the export dialog's error state can be previewed. */
const EXISTING_EXPORTS = new Set(["~/midnight-ops.b3theme"]);

/**
 * Each user theme's last export, as the server saves it. midnight-ops has one, so the
 * export dialog's filled-in fields and its unchanged-version note can be previewed.
 */
const SAVED_PACKAGES: Record<string, ThemeMetadata> = {
    "midnight-ops": {
        repo: "https://github.com/example/midnight-ops",
        author: "Example Author",
        description: "A dark, low-glare theme for long on-call nights.",
        themeVersion: "1.0.0",
    },
};

export async function getThemePackage(name: string): Promise<Partial<ThemeMetadata>> {
    await wait();
    return { ...SAVED_PACKAGES[name] };
}

export async function postExportTheme(
    name: string,
    path: string,
    metadata: ThemeMetadata
): Promise<ExportThemeResponse> {
    await wait(300);
    const full = path.trim().endsWith(".b3theme") ? path.trim() : `${path.trim()}.b3theme`;
    if (EXISTING_EXPORTS.has(full)) throw new Error(`file already exists: ${full}`);
    if (!USER_THEMES[name]) throw new Error(`theme "${name}" not found`);
    EXISTING_EXPORTS.add(full);
    SAVED_PACKAGES[name] = { ...metadata };
    return { path: full };
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
