package cmd

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		activeTheme := viper.GetString("theme")

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
	Long: `Sets the active theme in the b3tty config file. The name must be a built-in
theme or a user-defined theme already present in the config file. Run
'b3tty theme list' to see all available theme names.

Changes take effect the next time b3tty is started.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		configPath := resolveConfigPath()

		colors, isBuiltin := src.GetBuiltinTheme(name)
		if !isBuiltin {
			themeNames, err := src.ReadThemeNames(configPath)
			if err != nil {
				cmdLog.Fatalf("could not read themes from config: %v", err)
			}
			found := false
			for _, n := range themeNames {
				if n == name {
					found = true
					break
				}
			}
			if !found {
				cmdLog.Fatalf("theme %q not found; run 'b3tty theme list' to see available themes", name)
			}
		}

		if err := src.UpdateThemeInConfig(configPath, name, colors); err != nil {
			cmdLog.Fatalf("failed to update theme: %v", err)
		}
		cmdLog.Infof("active theme set to %q", name)
		postToRunningServer(port, fmt.Sprintf("/theme-config?name=%s", url.QueryEscape(name)), struct{}{})
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
}
