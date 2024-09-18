/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cmmorrow/b3tty/src"
)

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
		src.InitClient = src.NewClient(&rows, &columns, &cursorBlink, &fontFamily, &fontSize, &theme)
		src.InitServer = src.NewServer(&uri, &port, &noAuth, &src.TLS{CertFilePath: certFile, KeyFilePath: keyFile, Enabled: tls})
		if tls {
			// Remap the default TLS port
			if port == 8080 {
				src.InitServer.Port = 8443
			}
			src.ServeTLS(!noBrowser)
		}
		src.Serve(!noBrowser)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// startCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:

	// startCmd.Flags().StringVar(&uri, "uri", "localhost", "")
	uri = "localhost"
	startCmd.Flags().IntVar(&port, "port", 8080, "The port b3tty is accessible from. If using TLS, the default port is 8443.")
	startCmd.Flags().IntVar(&rows, "rows", 24, "The number of lines displayed by the TTY.")
	startCmd.Flags().IntVar(&columns, "columns", 0, "The character number width of the TTY. If 0, auto fit to the browser window size. (default 0)")
	startCmd.Flags().BoolVar(&cursorBlink, "cursor-blink", true, "Enables cursor blink in the browser. May not work in all situations.")
	startCmd.Flags().StringVar(&fontFamily, "font-family", "monospace", "The default font to use. NOTE: Some browsers do not support custom fonts.")
	startCmd.Flags().IntVar(&fontSize, "font-size", 14, "The terminal text size.")
	startCmd.Flags().BoolVar(&tls, "tls", false, "Enable HTTPS via TLS. Requires cert-file and key-file to be provided.")
	startCmd.Flags().StringVar(&certFile, "cert-file", "", "Path to TLS certificate file.")
	startCmd.Flags().StringVar(&keyFile, "key-file", "", "Path to TLS private key file.")
	startCmd.Flags().BoolVar(&noAuth, "no-auth", false, "Disable API token verification. Using this flag will reduce security posture.")
	startCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Disables opening b3tty in the default browser.")

	// initConfig()

	// viper.BindPFlags(startCmd.Flags())
	// println(uri)
	// println(port)

	// viper.BindPFlag("uri", startCmd.Flags().Lookup("uri"))
	// viper.BindPFlag("port", startCmd.Flags().Lookup("port"))
}

// initConfig reads in config file and ENV variables if set.
// func initConfig() {
// 	println("called")
// 	if configFile != "" {
// 		println("config found")
// 		// Use config file from the flag.
// 		viper.SetConfigFile(configFile)

// 		// If a config file is found, read it in.
// 		err := viper.ReadInConfig()
// 		if err != nil {
// 			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
// 				fmt.Fprintf(os.Stderr, "%s not found\n", viper.ConfigFileUsed())
// 				os.Exit(1)
// 			}
// 			fmt.Fprintf(os.Stderr, "Error loading config file: %s\n", viper.ConfigFileUsed())
// 			os.Exit(2)
// 		}
// 		for _, val := range viper.AllKeys() {
// 			println(val)
// 		}
// 		if viper.IsSet("server.uri") {
// 			uri = viper.GetString("server.uri")
// 			println("uri set")
// 		}
// 		if viper.IsSet("server.port") {
// 			port = viper.GetInt("server.port")
// 			println("port set")
// 		}

// 		println(uri)
// 		println(port)
// 	}
// }
