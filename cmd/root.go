package cmd

import (
	"os"
	"time"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Version = "latest"

// startTime is captured as early as achievable in the process's lifetime —
// as a package-level var it's initialized during Go's package-init phase,
// which runs before main() executes. Used by `start` to log, in debug mode,
// how long the server took to become interactive (see cmd/start.go and
// TerminalServer.StartTime).
var startTime = time.Now()

var cfgFile string
var cmdLog = src.NewCommandLogger()
var profiles map[string]src.Profile
var configFileFound bool
var activeThemeName string
var themes = make(map[string]src.Theme)

// configKeys records which dotted keys the config file actually contained,
// and configLoadErr the error from decoding it (nil on success, and always
// nil when no config file was found). Both are populated by initConfig.
var configKeys src.ConfigKeys
var configLoadErr error

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

	// A config file that failed to load is a warning, not a failure, for every
	// subcommand except start: `b3tty settings edit` in particular has to stay
	// usable against a broken config, since it is how the user repairs it.
	// startCmd declares its own PersistentPreRun to make the same condition
	// fatal — cobra runs only the nearest one it finds walking up from the
	// command being executed, so this hook does not run for `b3tty start`.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if configLoadErr != nil {
			cmdLog.Warnf("config validation error: %v", configLoadErr)
		}
	},
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
	if !configFileFound {
		return
	}

	configPath := viper.ConfigFileUsed()
	src.Debugf("using config file: %s", configPath)

	// Pre-seed the decode target with the current flag values. Cobra parses
	// flags before OnInitialize runs, so each field already holds the CLI
	// value when one was passed and the flag's default otherwise; yaml.v3
	// leaves fields the file does not mention untouched, which is what makes
	// the Changed() guards below sufficient to give CLI > config > default.
	cfg := src.Config{
		Server: src.ServerConfig{
			TLS:                  src.TLS{Enabled: tls, CertFilePath: certFile, KeyFilePath: keyFile},
			SettingsServerConfig: src.NewSettingsServerConfig(port, noAuth, noBrowser, showMenubar),
		},
		Terminal: src.TerminalClient{
			FontFamily: fontFamily,
			FontSize:   fontSize,
			AutoResize: autoResize,
			Rows:       rows,
			Columns:    columns,
		},
	}

	var err error
	configKeys, err = src.LoadConfig(configPath, &cfg)
	if err != nil {
		// Recorded rather than handled here: `b3tty start` treats this as
		// fatal, every other subcommand warns and carries on with flag
		// defaults so `b3tty settings edit` can still open the file that
		// needs repairing. See the PersistentPreRun hooks on rootCmd and
		// startCmd.
		configLoadErr = err
		return
	}

	// Config-file values are only applied when the corresponding flag was not
	// explicitly passed on the command line, so CLI flag > config file > flag
	// default holds for every flag-backed setting below.
	if !startCmd.Flags().Changed("port") {
		port = cfg.Server.Port
	}
	if !startCmd.Flags().Changed("tls") {
		tls = cfg.Server.Enabled
	}
	if !startCmd.Flags().Changed("cert-file") {
		certFile = cfg.Server.CertFilePath
	}
	if !startCmd.Flags().Changed("key-file") {
		keyFile = cfg.Server.KeyFilePath
	}
	if !startCmd.Flags().Changed("no-auth") {
		noAuth = cfg.Server.NoAuth
	}
	if !startCmd.Flags().Changed("no-browser") {
		noBrowser = cfg.Server.NoBrowser
	}
	if !startCmd.Flags().Changed("show-menubar") {
		showMenubar = cfg.Server.ShowMenubar
	}
	if !startCmd.Flags().Changed("rows") {
		rows = cfg.Terminal.Rows
	}
	if !startCmd.Flags().Changed("columns") {
		columns = cfg.Terminal.Columns
	}
	if !startCmd.Flags().Changed("auto-resize") {
		autoResize = cfg.Terminal.AutoResize
	}
	// font-family and font-size have no corresponding start flag (CLI setting
	// was deprecated for these — see startCmd's init()), so there is no
	// precedence to guard; the seed value is already the effective default.
	fontFamily = cfg.Terminal.FontFamily
	fontSize = cfg.Terminal.FontSize

	// Decoding straight into map[string]Theme preserves each theme name's
	// exact case, which viper.GetStringMap does not — the reason the active
	// theme and the themes list used to be read back out of the raw YAML
	// separately.
	for name, t := range cfg.Themes {
		themes[name] = t
	}
	if cfg.Theme != "" {
		t, ok := cfg.Themes[cfg.Theme]
		if !ok {
			src.Errorf("cannot find theme %s", cfg.Theme)
			os.Exit(3)
		}
		theme = t
		activeThemeName = cfg.Theme
	}

	for name, p := range cfg.Profiles {
		profiles[name] = applyProfileDefaults(p)
	}
}

// applyProfileDefaults fills in a default for every profile field the config
// file left empty, replacing the per-field SetDefault calls that the old
// viper.Sub-based profile loading used.
func applyProfileDefaults(p src.Profile) src.Profile {
	if p.Shell == "" {
		p.Shell = src.DEFAULT_SHELL
	}
	if p.WorkingDirectory == "" {
		p.WorkingDirectory = src.DEFAULT_WORKING_DIRECTORY
	}
	if p.Root == "" {
		p.Root = src.DEFAULT_ROOT
	}
	if p.Title == "" {
		p.Title = src.DEFAULT_TITLE
	}
	if p.Commands == nil {
		p.Commands = []string{}
	}
	return p
}
