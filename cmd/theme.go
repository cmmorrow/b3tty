package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
)

var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Manage b3tty themes",
}

var themeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available themes",
	Long: `Lists every theme, grouped by source: your own themes (defined in the config
file), installed themes (imported .b3theme packages), and the built-in themes.
The active theme is marked with * and themes in the Themes menu with +.
Installed themes also show their author, version, and repository, and are
marked (modified) when their package has changed since it was installed.`,
	Run: func(cmd *cobra.Command, args []string) {
		userNames := make([]string, 0, len(themes))
		for name := range themes {
			userNames = append(userNames, name)
		}
		sort.Strings(userNames)
		installedNames := make([]string, 0, len(installedThemes))
		for name := range installedThemes {
			installedNames = append(installedNames, name)
		}
		sort.Strings(installedNames)

		section := func(title string, names []string, detail func(string) string) {
			if len(names) == 0 {
				return
			}
			fmt.Printf("%s:\n", title)
			for _, name := range names {
				printTheme(name, detail(name))
			}
			fmt.Println()
		}
		section("Your themes", userNames, func(string) string { return "" })
		section("Installed", installedNames, func(name string) string {
			m := installedThemes[name].Manifest
			detail := fmt.Sprintf("by %s · v%s · %s", m.Author, m.ThemeVersion, m.Repo)
			// The CLI loads installed themes without hashing them; list
			// hashes each package itself, which doesn't decompress anything.
			if modified, err := installedThemes[name].CheckModified(); err != nil {
				detail += " (unreadable)"
			} else if modified {
				detail += " (modified)"
			}
			return detail
		})
		section("Built-in", src.GetBuiltinThemeNames(), func(string) string { return "" })
		fmt.Println("(* active theme, + in the Themes menu)")
	},
}

var themeSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set the active theme",
	Long: `Sets the active theme in the b3tty config file and adds it to the Themes
menu if it isn't there. The name may be a built-in, installed, or user-defined
theme. Run 'b3tty theme list' to see all available theme names.

A running b3tty applies the theme right away.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		configPath := resolveConfigPath()

		if _, _, ok := resolveCLITheme(name); !ok {
			cmdLog.Fatalf("theme %q not found; run 'b3tty theme list' to see available themes", name)
		}

		if err := src.ActivateThemeInConfig(configPath, name); err != nil {
			cmdLog.Fatalf("failed to update theme: %v", err)
		}
		cmdLog.Infof("active theme set to %q", name)
		_ = postToRunningServer(fmt.Sprintf("/theme-config?name=%s", url.QueryEscape(name)), struct{}{})
	},
}

var themeImportYes bool

var themeImportCmd = &cobra.Command{
	Use:   "import <file> [--yes]",
	Short: "Install a .b3theme package",
	Long: `Installs a .b3theme theme package. The package is stored unchanged in
~/.config/b3tty/plugins, where b3tty checks at every start that it still
matches what was installed. Installed themes can't be edited or exported; copy
one with 'b3tty theme copy' to make your own version.

The theme's name must not match any other theme's. A package that is a new
version of an installed theme (it has the same package id) replaces it after
you confirm, keeping its place in the Themes menu; --yes confirms without
asking. The confirmation says when the new version has a different author or
repository than the installed one. An upgrade can't rename a theme.

A running b3tty picks the theme up right away.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]
		if configLoadErr != nil {
			cmdLog.Fatalf("could not load config: %v", configLoadErr)
		}
		isUserTheme := func(name string) bool {
			_, ok := themes[name]
			return ok
		}
		result, err := src.ImportThemePackage(pluginStore, path, isUserTheme, themeImportYes)
		var upgrade *src.UpgradeRequiredError
		if errors.As(err, &upgrade) {
			fmt.Println(upgrade.Error())
			if !confirm(fmt.Sprintf("Replace %s with this version?", upgrade.Installed.Name)) {
				cmdLog.Infof("%s was not changed", upgrade.Installed.Name)
				return
			}
			result, err = src.ImportThemePackage(pluginStore, path, isUserTheme, true)
		}
		if err != nil {
			cmdLog.Fatalf("failed to import theme: %v", err)
		}

		m := result.Theme.Manifest
		switch {
		case result.Unchanged:
			// Still reload: the running server may have missed the first
			// import, e.g. when it couldn't be reached.
			cmdLog.Infof("%s %s is already installed", m.Name, m.ThemeVersion)
		case result.Upgraded:
			cmdLog.Infof("upgraded %s to version %s", m.Name, m.ThemeVersion)
		default:
			cmdLog.Infof("installed %s %s by %s; select it with 'b3tty theme set %s'", m.Name, m.ThemeVersion, m.Author, m.Name)
		}
		reloadRunningServer()
	},
}

var themeCopyCmd = &cobra.Command{
	Use:   "copy <name> <new-name>",
	Short: "Copy a theme into a new theme of your own",
	Long: `Copies a built-in, installed, or user-defined theme into a new user theme in
the config file, which you can then edit. The copy is added to the Themes menu
but not activated. It is a new theme: when you export it, it gets its own
package id rather than the original's.

An installed theme's background image lives inside its package, so copying
one saves the image to ~/.config/b3tty/backgrounds and points the copy at it.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name, newName := args[0], args[1]
		if configLoadErr != nil {
			cmdLog.Fatalf("could not load config: %v", configLoadErr)
		}
		source, kind, ok := resolveCLITheme(name)
		if !ok {
			cmdLog.Fatalf("theme %q not found; run 'b3tty theme list' to see available themes", name)
		}
		if _, _, taken := resolveCLITheme(newName); taken {
			cmdLog.Fatalf("a theme named %q already exists", newName)
		}
		var image []byte
		installed := installedThemes[name]
		if kind == src.ThemeSourceInstalled && installed.HasImage() {
			var err error
			if image, err = installed.ReadImage(); err != nil {
				cmdLog.Fatalf("could not read the background image of %q: %v", name, err)
			}
		}
		copied, err := src.CopyTheme(resolveConfigPath(), newName, source, image, installed.ImageExt)
		if err != nil {
			cmdLog.Fatalf("failed to copy theme: %v", err)
		}
		cmdLog.Infof("copied %q to %q", name, newName)
		if len(image) > 0 {
			cmdLog.Infof("its background image was saved to %s", copied.BackgroundImage)
		}
		reloadRunningServer()
	},
}

var themeDeleteYes bool

var themeDeleteCmd = &cobra.Command{
	Use:   "delete <name> [--yes]",
	Short: "Delete a theme, uninstall it, or remove it from the Themes menu",
	Long: `Permanently deletes a theme. What that means depends on the theme:

  - a user-defined theme is removed from the config file
  - an installed theme is uninstalled; you'll need its package file to
    install it again
  - a built-in theme can't be deleted, so it is removed from the Themes menu
    and stays available in Select Theme

There is no undo, so you are asked to confirm unless --yes is given. When the
theme is the active theme, b3tty switches to the default-theme, or to no theme.
A running b3tty picks the change up right away.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if configLoadErr != nil {
			cmdLog.Fatalf("could not load config: %v", configLoadErr)
		}
		_, kind, ok := resolveCLITheme(name)
		if !ok {
			cmdLog.Fatalf("theme %q not found; run 'b3tty theme list' to see available themes", name)
		}
		var prompt, done string
		switch kind {
		case src.ThemeSourceUser:
			prompt, done = fmt.Sprintf("Delete %s? This can't be undone.", name), "deleted %q"
		case src.ThemeSourceInstalled:
			prompt, done = fmt.Sprintf("Uninstall %s? This can't be undone; you'll need the package file to install it again.", name), "uninstalled %q"
		case src.ThemeSourceBuiltin:
			if !slices.Contains(availableThemes, name) {
				cmdLog.Fatalf("%q is a built-in theme and isn't in the Themes menu; built-in themes can't be deleted", name)
			}
			prompt, done = fmt.Sprintf("Remove %s from the Themes menu? It stays available in Select Theme.", name), "removed %q from the Themes menu"
		}
		if !themeDeleteYes && !confirm(prompt) {
			cmdLog.Infof("%s was not changed", name)
			return
		}
		if err := src.DeleteTheme(resolveConfigPath(), pluginStore, name, kind); err != nil {
			cmdLog.Fatalf("failed to delete theme: %v", err)
		}
		cmdLog.Infof(done, name)
		reloadRunningServer()
	},
}

var themeVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Check installed theme packages",
	Long: `Re-hashes every installed theme package and reports packages that changed
since they were installed, packages that are missing or no longer valid, and
orphaned files in the plugin store that no installed theme uses (remove those
with 'b3tty theme gc'). Exits with status 1 when anything is wrong.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if pluginStore == nil {
			cmdLog.Fatalf("the plugin store is not available")
		}
		report, err := pluginStore.Verify()
		if err != nil {
			cmdLog.Fatalf("could not verify installed themes: %v", err)
		}
		for _, name := range report.Modified {
			fmt.Printf("modified: %s (its package changed since it was installed)\n", name)
		}
		for _, name := range report.Missing {
			fmt.Printf("missing:  %s (its package is gone; uninstall it with 'b3tty theme delete %s')\n", name, name)
		}
		invalid := make([]string, 0, len(report.Invalid))
		for name := range report.Invalid {
			invalid = append(invalid, name)
		}
		sort.Strings(invalid)
		for _, name := range invalid {
			fmt.Printf("invalid:  %s (%s)\n", name, report.Invalid[name])
		}
		for _, path := range report.Orphans {
			fmt.Printf("orphan:   %s\n", path)
		}
		if !report.OK() {
			os.Exit(1)
		}
		fmt.Printf("%d installed theme(s) verified\n", report.Checked)
	},
}

var themeGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Remove orphaned files from the plugin store",
	Long:  "Removes files under ~/.config/b3tty/plugins/themes that no installed theme uses, such as packages left behind by an interrupted install.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if pluginStore == nil {
			cmdLog.Fatalf("the plugin store is not available")
		}
		removed, err := pluginStore.GC()
		for _, path := range removed {
			fmt.Printf("removed %s\n", path)
		}
		if err != nil {
			cmdLog.Fatalf("could not remove orphaned files: %v", err)
		}
		if len(removed) == 0 {
			fmt.Println("nothing to remove")
		}
	},
}

var themeExportOutput string
var themeExportMeta src.ThemeMetadata

var themeExportCmd = &cobra.Command{
	Use:   "export <name> --output <path> [--repo <url>] [--author <name>] [--description <text>] [--theme-version <version>]",
	Short: "Export a user-defined theme as a .b3theme package",
	Long: `Exports a user-defined theme from the config file as a .b3theme package: a zip
holding the theme's colors, its background image (if any), and a manifest. The
package contains nothing else from the config file, so it is safe to share.

The output path may start with ~ for your home directory, and .b3theme is
appended if it is missing. The directory must already exist, and an existing
file is never overwritten. Built-in and installed themes cannot be exported.

Every package records a repo (an https URL of the theme's source repository),
an author, a description, and a theme version. The theme version is free-form
(e.g. 1.2.0 or 2024-spring) and is unrelated to the b3tty version. All four are
required on a theme's first export. b3tty saves them, along with the package
id it generates, in the theme's package: block in the config file, so later
exports reuse the id and only need the flags whose values change — typically
--theme-version.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if configLoadErr != nil {
			cmdLog.Fatalf("could not load config: %v", configLoadErr)
		}
		if src.IsBuiltinThemeName(name) {
			cmdLog.Fatalf("%q is a built-in theme; only user-defined themes can be exported", name)
		}
		if _, ok := installedThemes[name]; ok {
			cmdLog.Fatalf("%q is an installed theme and can't be exported; copy it with 'b3tty theme copy' to make your own", name)
		}
		theme, ok := themes[name]
		if !ok {
			cmdLog.Fatalf("theme %q not found in config; run 'b3tty theme list' to see available themes", name)
		}
		configPath := resolveConfigPath()
		saved, err := src.ReadThemePackageDetails(configPath, name)
		if err != nil {
			cmdLog.Fatalf("could not read saved package details: %v", err)
		}

		// Each flag that wasn't passed falls back to the theme's last export.
		meta := themeExportMeta
		var missing []string
		for _, f := range []struct {
			flag  string
			value *string
			saved func(src.ThemeMetadata) string
		}{
			{"repo", &meta.Repo, func(m src.ThemeMetadata) string { return m.Repo }},
			{"author", &meta.Author, func(m src.ThemeMetadata) string { return m.Author }},
			{"description", &meta.Description, func(m src.ThemeMetadata) string { return m.Description }},
			{"theme-version", &meta.ThemeVersion, func(m src.ThemeMetadata) string { return m.ThemeVersion }},
		} {
			if !cmd.Flags().Changed(f.flag) && saved != nil {
				*f.value = f.saved(saved.ThemeMetadata)
			}
			if strings.TrimSpace(*f.value) == "" {
				missing = append(missing, "--"+f.flag)
			}
		}
		if len(missing) > 0 {
			cmdLog.Fatalf("missing %s (required on a theme's first export)", strings.Join(missing, ", "))
		}
		if saved != nil && strings.TrimSpace(meta.ThemeVersion) == saved.ThemeVersion {
			cmdLog.Warnf("theme version %q is the same as the last export; bump it so people who installed the theme see an upgrade", saved.ThemeVersion)
		}

		path, _, err := src.ExportUserTheme(configPath, name, theme, meta, themeExportOutput, Version, time.Now())
		if err != nil {
			cmdLog.Fatalf("failed to export theme: %v", err)
		}
		cmdLog.Infof("exported theme %q to %s", name, path)
	},
}

// printTheme prints one theme of 'b3tty theme list': * marks the active theme
// and + a theme in the Themes menu.
func printTheme(name, detail string) {
	active, menu := " ", " "
	if name == activeThemeName {
		active = "*"
	}
	if slices.Contains(availableThemes, name) {
		menu = "+"
	}
	if detail != "" {
		detail = "  " + detail
	}
	fmt.Printf("  %s%s %s%s\n", active, menu, name, detail)
}

// resolveCLITheme looks name up among every theme source as loaded by
// initConfig; see src.ResolveTheme.
func resolveCLITheme(name string) (src.Theme, src.ThemeSource, bool) {
	return src.ResolveTheme(name, installedThemes, themes)
}

// confirm asks a yes/no question on stdin and reports whether the answer was
// yes. Anything else, including no input at all, is no.
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// reloadRunningServer asks a running b3tty to reload its themes after a theme
// command changed the config file or the plugin store. When a server is
// running but can't be reached, it says to restart it instead.
func reloadRunningServer() {
	if err := postToRunningServer("/reload-themes", struct{}{}); err != nil && !errors.Is(err, errNoRunningServer) {
		cmdLog.Warnf("couldn't reach the running b3tty (%v); restart b3tty to load the change", err)
	}
}

func init() {
	rootCmd.AddCommand(themeCmd)
	themeCmd.AddCommand(themeListCmd)
	themeCmd.AddCommand(themeSetCmd)
	themeCmd.AddCommand(themeExportCmd)
	themeCmd.AddCommand(themeImportCmd)
	themeCmd.AddCommand(themeCopyCmd)
	themeCmd.AddCommand(themeDeleteCmd)
	themeCmd.AddCommand(themeVerifyCmd)
	themeCmd.AddCommand(themeGCCmd)
	themeImportCmd.Flags().BoolVarP(&themeImportYes, "yes", "y", false, "replace an installed version of the theme without asking")
	themeDeleteCmd.Flags().BoolVarP(&themeDeleteYes, "yes", "y", false, "delete without asking for confirmation")
	themeExportCmd.Flags().StringVarP(&themeExportOutput, "output", "o", "", "file path to write the .b3theme package to (required)")
	_ = themeExportCmd.MarkFlagRequired("output")
	themeExportCmd.Flags().StringVar(&themeExportMeta.Repo, "repo", "", "https URL of the theme's source repository (defaults to the last export's)")
	themeExportCmd.Flags().StringVar(&themeExportMeta.Author, "author", "", "author of the theme (defaults to the last export's)")
	themeExportCmd.Flags().StringVar(&themeExportMeta.Description, "description", "", "description of the theme (defaults to the last export's)")
	themeExportCmd.Flags().StringVar(&themeExportMeta.ThemeVersion, "theme-version", "", "free-form version of the theme (defaults to the last export's)")
}
