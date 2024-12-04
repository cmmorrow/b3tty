package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Version = "latest"

var cfgFile string
var profiles map[string]src.Profile

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Version: Version,
	Use:     "b3tty",
	Short:   ".... but browser based TTY!",
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
	rootCmd.SetVersionTemplate("{{ .Version }}\n")
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
	viper.BindPFlags(startCmd.Flags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	profiles = make(map[string]src.Profile)
	profiles["default"] = src.NewProfile(DEFAULT_SHELL, DEFAULT_WORKING_DIRECTORY, DEFAULT_ROOT, DEFAULT_TITLE, []string{})

	viper.SetConfigName("conf")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/b3tty")
	viper.AddConfigPath("/etc/b3tty")
	viper.AddConfigPath("$HOME/repos/b3tty/")
	viper.SetConfigFile(cfgFile)
	if err := viper.ReadInConfig(); err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			if len(cfgFile) > 0 {
				log.Printf("%s not found\n", cfgFile)
			}
		default:
			f := ""
			if len(cfgFile) > 0 {
				f = cfgFile
			}
			fmt.Fprintf(os.Stderr, "Error loading config file %s\n", f)
			os.Exit(1)
		}
	}

	if len(viper.ConfigFileUsed()) > 0 {
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
		if viper.IsSet("terminal.font-size") {
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
			profileNames := viper.GetStringMap("profiles")
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
