package cmd

import (
	"fmt"
	"net/url"
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
	Long:  "Lists all themes: built-in themes and any user-defined themes in the config file. The active theme is marked with *.",
	Run: func(cmd *cobra.Command, args []string) {
		activeTheme := activeThemeName

		fmt.Println("Built-in themes:")
		for _, name := range src.GetBuiltinThemeNames() {
			printTheme(name, activeTheme)
		}

		builtinSet := make(map[string]bool)
		for _, name := range src.GetBuiltinThemeNames() {
			builtinSet[name] = true
		}

		var userNames []string
		configPath := resolveConfigPath()
		names, err := src.ReadThemeNames(configPath)
		if err != nil {
			cmdLog.Warnf("could not read user themes: %v", err)
		} else {
			for _, name := range names {
				if !builtinSet[name] {
					userNames = append(userNames, name)
				}
			}
			sort.Strings(userNames)
		}

		if len(userNames) > 0 {
			fmt.Println("\nUser-defined themes:")
			for _, name := range userNames {
				printTheme(name, activeTheme)
			}
		}

		if activeTheme != "" {
			fmt.Printf("\n(* active theme: %s)\n", activeTheme)
		}
	},
}

var themeSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set the active theme",
	Long: `Sets the active theme in the b3tty config file and adds it to the Themes
menu if it isn't there. The name must be a built-in theme or a user-defined
theme already present in the config file. Run 'b3tty theme list' to see all
available theme names.

Changes take effect the next time b3tty is started.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		configPath := resolveConfigPath()

		if !src.IsBuiltinThemeName(name) {
			themeNames, err := src.ReadThemeNames(configPath)
			if err != nil {
				cmdLog.Fatalf("could not read themes from config: %v", err)
			}
			if !slices.Contains(themeNames, name) {
				cmdLog.Fatalf("theme %q not found; run 'b3tty theme list' to see available themes", name)
			}
		}

		if err := src.ActivateThemeInConfig(configPath, name); err != nil {
			cmdLog.Fatalf("failed to update theme: %v", err)
		}
		cmdLog.Infof("active theme set to %q", name)
		postToRunningServer(fmt.Sprintf("/theme-config?name=%s", url.QueryEscape(name)), struct{}{})
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
file is never overwritten. Built-in themes cannot be exported.

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

func printTheme(name, activeTheme string) {
	marker := "  "
	if name == activeTheme {
		marker = "* "
	}
	fmt.Printf("  %s%s\n", marker, name)
}

func init() {
	rootCmd.AddCommand(themeCmd)
	themeCmd.AddCommand(themeListCmd)
	themeCmd.AddCommand(themeSetCmd)
	themeCmd.AddCommand(themeExportCmd)
	themeExportCmd.Flags().StringVarP(&themeExportOutput, "output", "o", "", "file path to write the .b3theme package to (required)")
	_ = themeExportCmd.MarkFlagRequired("output")
	themeExportCmd.Flags().StringVar(&themeExportMeta.Repo, "repo", "", "https URL of the theme's source repository (defaults to the last export's)")
	themeExportCmd.Flags().StringVar(&themeExportMeta.Author, "author", "", "author of the theme (defaults to the last export's)")
	themeExportCmd.Flags().StringVar(&themeExportMeta.Description, "description", "", "description of the theme (defaults to the last export's)")
	themeExportCmd.Flags().StringVar(&themeExportMeta.ThemeVersion, "theme-version", "", "free-form version of the theme (defaults to the last export's)")
}
