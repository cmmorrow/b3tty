package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cmmorrow/b3tty/src"
)

var debug bool
var cursorBlink bool
var fontFamily string
var fontSize int
var rows int
var columns int
var uri string
var port int
var themeName string
var theme src.Theme
var tls bool
var certFile string
var keyFile string
var noAuth bool
var noBrowser bool
var startupProfile string

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the b3tty server",
	Long: `Starts the b3tty proxy server. The server is only accessible from the machine
where it's started as a security feature and this behavior cannot be disabled.
B3tty also enables access via a randomly generated API token each time the
server is started to prevent a user without access to the shell where b3tty is
running from accessing the user's shell. This behavior can be disabled through
configuration. For additional security, b3tty supports TLS over https and wss.`,
	Run: func(cmd *cobra.Command, args []string) {
		src.SetDebug(debug)
		if cfgPath := viper.ConfigFileUsed(); cfgPath != "" {
			if err := src.ValidateConfig(cfgPath); err != nil {
				src.Fatalf("config validation error: %v", err)
			}
		}
		if err := src.ValidateTheme(&theme); err != nil {
			src.Fatalf("theme validation error: %v", err)
		}
		if !src.ValidatePortNumber(port) {
			src.Fatalf("port number must be 1 - 65535")
		}
		if tls {
			// Remap the default TLS port
			if port == 8080 {
				port = 8443
			}
		}
		if startupProfile != "" {
			if _, ok := profiles[startupProfile]; !ok {
				src.Fatalf("profile %q not found in config", startupProfile)
			}
		} else {
			startupProfile = src.DEFAULT_PROFILE_NAME
		}
		ts := src.TerminalServer{
			Client:         src.NewClient(&rows, &columns, &cursorBlink, &fontFamily, &fontSize, &theme),
			Server:         src.NewServer(&uri, &port, &noAuth, &src.TLS{CertFilePath: certFile, KeyFilePath: keyFile, Enabled: tls}),
			Profiles:       profiles,
			Themes:         themes,
			OrgCols:        src.DEFAULT_COLS,
			OrgRows:        src.DEFAULT_ROWS,
			ProfileName:    "",
			StartupProfile: startupProfile,
			ActiveTheme:    activeThemeName,
			FirstRun:       !configFileFound,
			AuthSleep:      time.Sleep,
		}
		src.Serve(&ts, !noBrowser, tls)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	// Setting these parameters from the command-line has been deprecated but can still
	// be set from a config file.
	uri = src.DEFAULT_URI
	cursorBlink = src.DEFAULT_CURSOR_BLINK
	fontFamily = src.DEFAULT_FONT_FAMILY
	fontSize = src.DEFAULT_FONT_SIZE
	startCmd.Flags().IntVar(&rows, "rows", src.DEFAULT_ROWS, "The number of lines displayed by the TTY.")
	startCmd.Flags().IntVar(&columns, "columns", src.DEFAULT_COLS, "The character number width of the TTY. If 0, auto fit to the browser window size. (default 0)")
	startCmd.Flags().MarkHidden("rows")
	startCmd.Flags().MarkHidden("columns")

	startCmd.Flags().IntVar(&port, "port", 8080, "The port b3tty is accessible from. If using TLS, the default port is 8443.")
	startCmd.Flags().BoolVar(&tls, "tls", false, "Enable HTTPS via TLS. Requires cert-file and key-file to be provided.")
	startCmd.Flags().StringVar(&certFile, "cert-file", "", "Path to TLS certificate file.")
	startCmd.Flags().StringVar(&keyFile, "key-file", "", "Path to TLS private key file.")
	startCmd.Flags().BoolVar(&noAuth, "no-auth", false, "Disable API token verification. Using this flag will reduce security posture.")
	startCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Disables opening b3tty in the default browser.")
	startCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging.")
	startCmd.Flags().StringVar(&startupProfile, "profile", "", "Profile to load on startup. Must exist in the config file.")
}
