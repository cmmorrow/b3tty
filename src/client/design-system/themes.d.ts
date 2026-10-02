// Virtual module served by build.ts: every src/default_themes/*.json file,
// keyed by built-in theme name (the file name with "_" replaced by "-").
declare module "b3tty:builtin-themes" {
    const themes: Record<string, Record<string, string>>;
    export default themes;
}
