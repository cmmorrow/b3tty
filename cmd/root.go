package cmd

import (
	"fmt"
	"os"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var profiles map[string]src.Profile

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "b3tty",
	Short: ".... but browser based TTY!",
	Long: `b3tty is a terminal emulator accessible entirely from your web browser. It is
built using xterm.js which provides the terminal look and feel using Javascript
and CSS. A small web server acts as a proxy between a psuedo terminal and the
browser, which communicates over web sockets.

The terminal appearance and server can be configured with command-line flags or
a configuration yaml file. Use the following command to display availabe server
and terminal configuration options:

	b3tty start --help`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file to use")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	viper.BindPFlags(startCmd.Flags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)

		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				fmt.Fprintf(os.Stderr, "%s not found\n", viper.ConfigFileUsed())
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Error loading config file: %s\n", viper.ConfigFileUsed())
			os.Exit(2)
		}

		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		if viper.IsSet("server.port") {
			port = viper.GetInt("server.port")
		}
		if viper.IsSet("server.tls") {
			tls = viper.GetBool("server.tls")
		}
		if viper.IsSet("server.cert-file") {
			certFile = viper.GetString("server.cert-file")
		}
		if viper.IsSet("server.key-file") {
			keyFile = viper.GetString("server.key-file")
		}
		if viper.IsSet("server.no-auth") {
			noAuth = viper.GetBool("server.no-auth")
		}
		if viper.IsSet("server.no-browser") {
			noBrowser = viper.GetBool("server.no-browser")
		}
		if viper.IsSet("terminal.rows") {
			rows = viper.GetInt("terminal.rows")
		}
		if viper.IsSet("terminal.columns") {
			columns = viper.GetInt("terminal.columns")
		}
		if viper.IsSet("terminal.font-family") {
			fontFamily = viper.GetString("terminal.font-family")
		}
		if viper.IsSet("server.font-size") {
			fontSize = viper.GetInt("terminal.font-size")
		}
		if viper.IsSet("theme") {
			themeName = viper.GetString("theme")
			themeCfg := viper.Sub("themes." + themeName)
			if themeCfg == nil {
				fmt.Fprintf(os.Stderr, "cannot find theme %s\n", themeName)
				os.Exit(3)
			}
			theme.MapToTheme(themeCfg.AllSettings())
		}

		if viper.IsSet("profiles") {
			profiles = make(map[string]src.Profile)
			profileNames := viper.GetStringMap("profiles")
			profiles["default"] = src.NewProfile(DEFAULT_SHELL, DEFAULT_WORKING_DIRECTORY, DEFAULT_ROOT, DEFAULT_TITLE, []string{})
			for name := range profileNames {
				profileCfg := viper.Sub("profiles." + name)
				if profileCfg == nil {
					continue
				}
				profileCfg.SetDefault("root", DEFAULT_ROOT)
				profileCfg.SetDefault("working-directory", DEFAULT_WORKING_DIRECTORY)
				profileCfg.SetDefault("shell", DEFAULT_SHELL)
				profileCfg.SetDefault("title", DEFAULT_TITLE)
				profileCfg.SetDefault("commands", []string{})
				root := profileCfg.GetString("root")
				workingDirectory := profileCfg.GetString("working-directory")
				shell := profileCfg.GetString("shell")
				title := profileCfg.GetString("title")
				commands := profileCfg.GetStringSlice("commands")
				profiles[name] = src.NewProfile(shell, workingDirectory, root, title, commands)
			}
		}

	}

	// viper.AutomaticEnv() // read in environment variables that match
}
