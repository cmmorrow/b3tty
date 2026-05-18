package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	profileShell    string
	profileDir      string
	profileTitle    string
	profileRoot     string
	profileCommands []string
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage b3tty profiles",
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles",
	Run: func(cmd *cobra.Command, args []string) {
		nonDefault := 0
		for name := range profiles {
			if name != src.DEFAULT_PROFILE_NAME {
				nonDefault++
			}
		}
		if nonDefault == 0 {
			fmt.Println("No profiles configured.")
			return
		}

		names := make([]string, 0, len(profiles))
		for name := range profiles {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			if name == src.DEFAULT_PROFILE_NAME {
				continue
			}
			p := profiles[name]
			fmt.Printf("  %s\n", name)
			if p.Shell != "" {
				fmt.Printf("    shell: %s\n", p.Shell)
			}
			if p.WorkingDirectory != "" {
				fmt.Printf("    dir:   %s\n", p.WorkingDirectory)
			}
			if p.Title != "" {
				fmt.Printf("    title: %s\n", p.Title)
			}
			if p.Root != "" {
				fmt.Printf("    root:  %s\n", p.Root)
			}
			if len(p.Commands) > 0 {
				fmt.Printf("    commands: %s\n", strings.Join(p.Commands, "; "))
			}
		}
	},
}

var profileOpenCmd = &cobra.Command{
	Use:   "open <name>",
	Short: "Open a profile in the browser",
	Long:  `Opens the given profile in the default browser. b3tty must already be running.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		if _, ok := profiles[name]; !ok {
			cmdLog.Fatalf("profile %q not found; run 'b3tty profile list' to see configured profiles", name)
		}

		lf, err := src.ReadLockFile()
		if err != nil {
			cmdLog.Fatalf("could not read lock file: %v", err)
		}
		if lf == nil {
			cmdLog.Fatal("b3tty server is not running")
		}

		tokenQuery := ""
		if lf.Token != "" {
			tokenQuery = "?token=" + lf.Token
		}

		var url string
		if name == src.DEFAULT_PROFILE_NAME {
			url = fmt.Sprintf("%s://localhost:%d/%s", lf.Protocol, lf.Port, tokenQuery)
		} else if tokenQuery != "" {
			url = fmt.Sprintf("%s://localhost:%d/%s&profile=%s", lf.Protocol, lf.Port, tokenQuery, name)
		} else {
			url = fmt.Sprintf("%s://localhost:%d/?profile=%s", lf.Protocol, lf.Port, name)
		}

		cmdLog.Infof("opening %s", url)
		if err := src.OpenBrowser(url); err != nil {
			cmdLog.Fatalf("failed to open browser: %v", err)
		}
	},
}

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add or update a profile",
	Long: `Creates a new profile or updates an existing one in the config file. When
updating an existing profile, only explicitly provided flags are changed;
all other fields retain their current values.

Changes are persisted immediately but do not affect any running b3tty session.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if name == src.DEFAULT_PROFILE_NAME {
			cmdLog.Fatalf("cannot modify the %q profile", src.DEFAULT_PROFILE_NAME)
		}

		configPath := viper.ConfigFileUsed()

		shell := src.DEFAULT_SHELL
		dir := src.DEFAULT_WORKING_DIRECTORY
		title := src.DEFAULT_TITLE
		root := src.DEFAULT_ROOT
		var commands []string

		if existing, ok := profiles[name]; ok {
			shell = existing.Shell
			dir = existing.WorkingDirectory
			title = existing.Title
			root = existing.Root
			commands = existing.Commands
		}

		if cmd.Flags().Changed("shell") {
			shell = profileShell
		}
		if cmd.Flags().Changed("dir") {
			dir = profileDir
		}
		if cmd.Flags().Changed("title") {
			title = profileTitle
		}
		if cmd.Flags().Changed("root") {
			root = profileRoot
		}
		if cmd.Flags().Changed("commands") {
			commands = profileCommands
		}

		p := src.NewProfile(shell, dir, root, title, commands)
		if err := src.SaveProfileToConfig(configPath, name, p); err != nil {
			cmdLog.Fatalf("failed to save profile: %v", err)
		}
		cmdLog.Infof("profile %q saved", name)
	},
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if name == src.DEFAULT_PROFILE_NAME {
			cmdLog.Fatalf("cannot delete the %q profile", src.DEFAULT_PROFILE_NAME)
		}

		configPath := viper.ConfigFileUsed()
		if err := src.DeleteProfileFromConfig(configPath, name); err != nil {
			cmdLog.Fatalf("failed to delete profile: %v", err)
		}
		cmdLog.Infof("profile %q deleted", name)
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileOpenCmd)
	profileCmd.AddCommand(profileAddCmd)
	profileCmd.AddCommand(profileDeleteCmd)

	profileAddCmd.Flags().StringVar(&profileShell, "shell", src.DEFAULT_SHELL, "Shell to use (e.g. /bin/zsh)")
	profileAddCmd.Flags().StringVar(&profileDir, "dir", src.DEFAULT_WORKING_DIRECTORY, "Working directory")
	profileAddCmd.Flags().StringVar(&profileTitle, "title", src.DEFAULT_TITLE, "Profile title shown in the tab label")
	profileAddCmd.Flags().StringVar(&profileRoot, "root", src.DEFAULT_ROOT, "Root path for the profile")
	profileAddCmd.Flags().StringArrayVar(&profileCommands, "commands", nil, "Commands to run on start (repeatable)")
}
