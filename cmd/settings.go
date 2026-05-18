package cmd

import (
	"bytes"
	cryptotls "crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/cmmorrow/b3tty/src"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	setPort        int
	setNoAuth      bool
	setNoBrowser   bool
	setFontFamily  string
	setFontSize    int
	setCursorBlink bool
	setAutoResize  bool
	setRows        int
	setColumns     int
	editEditor     string
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "View or modify b3tty settings",
}

var settingsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show current settings from the config file",
	Run: func(cmd *cobra.Command, args []string) {
		currentCursorBlink := src.DEFAULT_CURSOR_BLINK
		if viper.IsSet("terminal.cursor-blink") {
			currentCursorBlink = viper.GetBool("terminal.cursor-blink")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Server:")
		fmt.Fprintf(w, "  port:\t%d\n", port)
		fmt.Fprintf(w, "  no-auth:\t%v\n", noAuth)
		fmt.Fprintf(w, "  no-browser:\t%v\n", noBrowser)
		fmt.Fprintln(w, "\nTerminal:")
		fmt.Fprintf(w, "  font-family:\t%s\n", fontFamily)
		fmt.Fprintf(w, "  font-size:\t%d\n", fontSize)
		fmt.Fprintf(w, "  cursor-blink:\t%v\n", currentCursorBlink)
		fmt.Fprintf(w, "  auto-resize:\t%v\n", autoResize)
		if viper.IsSet("terminal.rows") {
			fmt.Fprintf(w, "  rows:\t%d\n", rows)
		}
		if viper.IsSet("terminal.columns") {
			fmt.Fprintf(w, "  columns:\t%d\n", columns)
		}
		w.Flush()
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Update settings in the config file",
	Long: `Updates one or more settings in the b3tty config file. Only the flags you
explicitly provide are changed; all other settings keep their current values.

Server settings (--port, --no-auth, --no-browser) and terminal dimensions
(--rows, --columns) require a server restart to take effect. Font, font-size,
and cursor-blink apply to live sessions immediately.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !anySettingFlagChanged(cmd) {
			cmdLog.Fatal("no settings specified; run 'b3tty settings set --help' to see available flags")
		}

		configPath := viper.ConfigFileUsed()

		currentPort := port
		currentNoAuth := noAuth
		currentNoBrowser := noBrowser
		currentFontFamily := fontFamily
		currentFontSize := fontSize
		currentCursorBlink := src.DEFAULT_CURSOR_BLINK
		if viper.IsSet("terminal.cursor-blink") {
			currentCursorBlink = viper.GetBool("terminal.cursor-blink")
		}
		currentAutoResize := autoResize
		currentRows := rows
		currentColumns := columns

		if cmd.Flags().Changed("port") {
			currentPort = setPort
		}
		if cmd.Flags().Changed("no-auth") {
			currentNoAuth = setNoAuth
		}
		if cmd.Flags().Changed("no-browser") {
			currentNoBrowser = setNoBrowser
		}
		if cmd.Flags().Changed("font-family") {
			currentFontFamily = setFontFamily
		}
		if cmd.Flags().Changed("font-size") {
			currentFontSize = setFontSize
		}
		if cmd.Flags().Changed("cursor-blink") {
			currentCursorBlink = setCursorBlink
		}
		if cmd.Flags().Changed("auto-resize") {
			currentAutoResize = setAutoResize
		}
		if cmd.Flags().Changed("rows") {
			currentRows = setRows
		}
		if cmd.Flags().Changed("columns") {
			currentColumns = setColumns
		}

		if !src.ValidatePortNumber(currentPort) {
			cmdLog.Fatalf("invalid port %d: must be between 1 and 65535", currentPort)
		}
		if currentFontSize < 1 {
			cmdLog.Fatalf("invalid font-size %d: must be 1 or greater", currentFontSize)
		}
		if currentRows < 10 {
			cmdLog.Fatalf("invalid rows %d: must be 10 or greater", currentRows)
		}
		if currentColumns < 10 {
			cmdLog.Fatalf("invalid columns %d: must be 10 or greater", currentColumns)
		}
		serverCfg := src.SettingsServerConfig{
			Port:      currentPort,
			NoAuth:    currentNoAuth,
			NoBrowser: currentNoBrowser,
		}
		terminalCfg := src.SettingsTerminalConfig{
			FontFamily:  currentFontFamily,
			FontSize:    currentFontSize,
			CursorBlink: currentCursorBlink,
			AutoResize:  currentAutoResize,
			Rows:        currentRows,
			Columns:     currentColumns,
		}

		if err := src.SaveSettingsToConfig(configPath, serverCfg, terminalCfg); err != nil {
			cmdLog.Fatalf("failed to save settings: %v", err)
		}
		cmdLog.Info("settings saved")

		notifyRunningServer(currentPort, serverCfg, terminalCfg)

		needsRestart := false
		for _, f := range []string{"port", "no-auth", "no-browser", "auto-resize"} {
			if cmd.Flags().Changed(f) {
				needsRestart = true
				break
			}
		}
		if needsRestart {
			cmdLog.Warn("note: port, no-auth, no-browser, and auto-resize require a server restart to take effect")
		}
	},
}

var settingsEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config file in an editor",
	Run: func(cmd *cobra.Command, args []string) {
		configPath := viper.ConfigFileUsed()
		if cfgFile != "" {
			configPath = cfgFile
		}
		if configPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				cmdLog.Fatalf("could not determine home directory: %v", err)
			}
			configPath = filepath.Join(home, src.DOT_CONFIG_PATH, src.B3TTY_CONFIG_PATH, src.CONFIG_FILE_NAME)
		}

		editor := editEditor
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vi"
		}

		editorCmd := exec.Command(editor, configPath)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr
		if err := editorCmd.Run(); err != nil {
			cmdLog.Fatalf("editor exited with error: %v", err)
		}
	},
}

// postToRunningServer fires a best-effort POST to the running b3tty server.
// Errors are silently ignored; the server may simply not be running.
func postToRunningServer(serverPort int, path string, body any) {
	scheme := "http"
	httpClient := &http.Client{Timeout: 2 * time.Second}
	if tls {
		scheme = "https"
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &cryptotls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	url := fmt.Sprintf("%s://localhost:%d%s", scheme, serverPort, path)
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return
	}
	resp.Body.Close()
}

// notifyRunningServer attempts to POST the new settings to the running b3tty
// server so it can update its in-memory state and push the change to any open
// browser sessions. Errors are ignored — the server may simply not be running.
func notifyRunningServer(serverPort int, serverCfg src.SettingsServerConfig, terminalCfg src.SettingsTerminalConfig) {
	payload := struct {
		Server   src.SettingsServerConfig   `json:"server"`
		Terminal src.SettingsTerminalConfig `json:"terminal"`
	}{Server: serverCfg, Terminal: terminalCfg}
	postToRunningServer(serverPort, "/settings", payload)
}

func anySettingFlagChanged(cmd *cobra.Command) bool {
	for _, name := range []string{"port", "no-auth", "no-browser", "font-family", "font-size", "cursor-blink", "auto-resize", "rows", "columns"} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(settingsCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
	settingsCmd.AddCommand(settingsEditCmd)

	settingsSetCmd.Flags().IntVar(&setPort, "port", 8080, "Server port (1–65535)")
	settingsSetCmd.Flags().BoolVar(&setNoAuth, "no-auth", false, "Disable authentication")
	settingsSetCmd.Flags().BoolVar(&setNoBrowser, "no-browser", false, "Disable auto-open browser on start")
	settingsSetCmd.Flags().StringVar(&setFontFamily, "font-family", "", "Terminal font family (empty to use system monospace)")
	settingsSetCmd.Flags().IntVar(&setFontSize, "font-size", 14, "Terminal font size")
	settingsSetCmd.Flags().BoolVar(&setCursorBlink, "cursor-blink", true, "Enable cursor blink (use --cursor-blink=false to disable)")
	settingsSetCmd.Flags().BoolVar(&setAutoResize, "auto-resize", true, "Auto-resize terminal to fit browser window; when false, rows and columns are used (requires restart)")
	settingsSetCmd.Flags().IntVar(&setRows, "rows", src.DEFAULT_ROWS, "Fixed terminal height in rows, minimum 10 (requires restart; ignored when auto-resize is true)")
	settingsSetCmd.Flags().IntVar(&setColumns, "columns", src.DEFAULT_COLS, "Fixed terminal width in columns, minimum 10 (requires restart; ignored when auto-resize is true)")

	settingsEditCmd.Flags().StringVar(&editEditor, "editor", "", "Editor executable to use (defaults to $VISUAL, $EDITOR, or vi)")
}
