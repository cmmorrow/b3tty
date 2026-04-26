package cmd

import (
	"os"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Version = "latest"

var cfgFile string
var profiles map[string]src.Profile
var configFileFound bool
var activeThemeName string
var themes = make(map[string]src.Theme)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Version: Version,
	Use:     "b3tty",
	Short:   "A better, browser based TTY",
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

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "b3tty config file to use")
	viper.BindPFlags(startCmd.Flags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	profiles = make(map[string]src.Profile)
	profiles[src.DEFAULT_PROFILE_NAME] = src.NewProfile(src.DEFAULT_SHELL, src.DEFAULT_WORKING_DIRECTORY, src.DEFAULT_ROOT, src.DEFAULT_TITLE, []string{})

	viper.SetConfigName("conf")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/b3tty")
	viper.AddConfigPath("$HOME/.b3tty")
	viper.AddConfigPath("/etc/b3tty")
	viper.SetConfigFile(cfgFile)
	if err := viper.ReadInConfig(); err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			if len(cfgFile) > 0 {
				src.Warnf("%s not found", cfgFile)
			}
		default:
			f := ""
			if len(cfgFile) > 0 {
				f = cfgFile
			}
			src.Errorf("error loading config file %s", f)
			os.Exit(1)
		}
	}

	configFileFound = viper.ConfigFileUsed() != "" || cfgFile != ""

	if configFileFound {
		src.Infof("using config file: %s", viper.ConfigFileUsed())

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
				src.Errorf("cannot find theme %s", themeName)
				os.Exit(3)
			}
			theme.MapToTheme(themeCfg.AllSettings())
			activeThemeName = themeName
		}

		if viper.IsSet("themes") {
			// ReadThemeNames reads directly from YAML to preserve key case;
			// viper.GetStringMap lowercases all keys.
			themeNames, err := src.ReadThemeNames(viper.ConfigFileUsed())
			if err != nil {
				src.Warnf("could not read theme names from config: %v", err)
			}
			for _, name := range themeNames {
				var t src.Theme
				if themeCfg := viper.Sub("themes." + name); themeCfg != nil {
					t.MapToTheme(themeCfg.AllSettings())
				}
				themes[name] = t
			}
		}

		// Guarantee the active theme is always in themes when the themes section
		// is missing or empty (some YAML parser versions return an empty map).
		if themeName != "" {
			if _, exists := themes[themeName]; !exists {
				themes[themeName] = theme
			}
		}

		if viper.IsSet("profiles") {
			profileNames := viper.GetStringMap("profiles")
			for name := range profileNames {
				profileCfg := viper.Sub("profiles." + name)
				if profileCfg == nil {
					continue
				}
				profileCfg.SetDefault("root", src.DEFAULT_ROOT)
				profileCfg.SetDefault("working-directory", src.DEFAULT_WORKING_DIRECTORY)
				profileCfg.SetDefault("shell", src.DEFAULT_SHELL)
				profileCfg.SetDefault("title", src.DEFAULT_TITLE)
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
