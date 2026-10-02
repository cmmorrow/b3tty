/**
 * Builds the component bundle for the b3tty design system artifact:
 *
 *   bun design-system/build.ts <out-dir>     (run from src/client; see `make design-system`)
 *
 * Writes <out-dir>/bundle.js, one minified classic script (IIFE) containing
 * the real components.ts and design.ts, with ../api.ts replaced by
 * mock-api.ts so no b3tty server is needed. It defines every b3tty-* custom
 * element and window.B3tty. Upload it as the design system's
 * project/components/bundle.js.
 */
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";

const clientDir = resolve(import.meta.dir, "..");
const repoDir = resolve(clientDir, "../..");
const themesDir = join(repoDir, "src/default_themes");

// Preview card names in display order; recorded in the bundle's header.
const COMPONENTS = [
    "Buttons",
    "FormInputs",
    "TextRoles",
    "EditorCards",
    "PaletteCard",
    "MenuBar",
    "Dialog",
    "AboutDialog",
    "ThemeSelector",
    "ThemePicker",
    "ThemeEditor",
    "ProfileEditor",
    "SettingsEditor",
];

const outDir = process.argv[2];
if (!outDir) {
    console.error("usage: bun design-system/build.ts <out-dir>");
    process.exit(2);
}

const themes: Record<string, unknown> = {};
for (const file of readdirSync(themesDir)
    .filter((f) => f.endsWith(".json"))
    .sort()) {
    themes[basename(file, ".json").replaceAll("_", "-")] = JSON.parse(readFileSync(join(themesDir, file), "utf8"));
}

const version = readFileSync(join(repoDir, "VERSION"), "utf8").trim();

const result = await Bun.build({
    entrypoints: [join(import.meta.dir, "entry.ts")],
    target: "browser",
    format: "iife",
    minify: true,
    define: { __B3TTY_VERSION__: JSON.stringify(version) },
    plugins: [
        {
            name: "b3tty-design-system",
            setup(build) {
                // components.ts imports "./api.ts"; serve the fixture-backed stand-in instead.
                build.onResolve({ filter: /^\.\/api\.ts$/ }, () => ({ path: join(import.meta.dir, "mock-api.ts") }));
                build.onResolve({ filter: /^b3tty:builtin-themes$/ }, (args) => ({
                    path: args.path,
                    namespace: "b3tty",
                }));
                build.onLoad({ filter: /.*/, namespace: "b3tty" }, () => ({
                    contents: `export default ${JSON.stringify(themes)};`,
                    loader: "js",
                }));
            },
        },
    ],
});
if (!result.success || !result.outputs[0]) {
    console.error(result.logs);
    process.exit(1);
}

const js = await result.outputs[0].text();
// The design system inlines the bundle into <script> elements, which these would end or escape.
if (/<\/script|<!--/i.test(js)) {
    console.error("bundle contains a literal </script or <!--, which the design system cannot inline");
    process.exit(1);
}

const header = { format: 4, namespace: "B3tty", components: COMPONENTS.map((name) => ({ name })) };
mkdirSync(outDir, { recursive: true });
const outFile = join(outDir, "bundle.js");
writeFileSync(outFile, `/* @ds-bundle: ${JSON.stringify(header)} */\n${js}`);
console.log(`wrote ${outFile} (${js.length} bytes, version ${version})`);
