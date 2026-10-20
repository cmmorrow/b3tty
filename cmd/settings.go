package cmd

import (
	"bytes"
	cryptotls "crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
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
	setShowMenubar string
	setFontFamily  string
	setFontSize    int
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
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Server:")
		fmt.Fprintf(w, "  port:\t%d\n", port)
		fmt.Fprintf(w, "  no-auth:\t%v\n", noAuth)
		fmt.Fprintf(w, "  no-browser:\t%v\n", noBrowser)
		fmt.Fprintf(w, "  show-menubar:\t%s\n", showMenubar)
		fmt.Fprintln(w, "\nTerminal:")
		fmt.Fprintf(w, "  font-family:\t%s\n", fontFamily)
		fmt.Fprintf(w, "  font-size:\t%d\n", fontSize)
		fmt.Fprintf(w, "  auto-resize:\t%v\n", autoResize)
		if configKeys.Has("terminal.rows") {
			fmt.Fprintf(w, "  rows:\t%d\n", rows)
		}
		if configKeys.Has("terminal.columns") {
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
(--rows, --columns) require a server restart to take effect. Font and
font-size apply to live sessions immediately.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !anySettingFlagChanged(cmd) {
			cmdLog.Fatal("no settings specified; run 'b3tty settings set --help' to see available flags")
		}

		configPath := resolveConfigPath()

		currentPort := port
		currentNoAuth := noAuth
		currentNoBrowser := noBrowser
		currentShowMenubar := showMenubar
		currentFontFamily := fontFamily
		currentFontSize := fontSize
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
		if cmd.Flags().Changed("show-menubar") {
			currentShowMenubar = setShowMenubar
		}
		if cmd.Flags().Changed("font-family") {
			currentFontFamily = setFontFamily
		}
		if cmd.Flags().Changed("font-size") {
			currentFontSize = setFontSize
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
		if !src.ValidateShowMenubar(currentShowMenubar) {
			cmdLog.Fatalf("invalid show-menubar %q: must be one of hover, visible, disable", currentShowMenubar)
		}
		serverCfg := src.NewSettingsServerConfig(currentPort, currentNoAuth, currentNoBrowser, currentShowMenubar)
		terminalCfg := src.TerminalClient{
			FontFamily: currentFontFamily,
			FontSize:   currentFontSize,
			AutoResize: currentAutoResize,
			Rows:       currentRows,
			Columns:    currentColumns,
		}

		if err := src.SaveSettingsToConfig(configPath, serverCfg, terminalCfg); err != nil {
			cmdLog.Fatalf("failed to save settings: %v", err)
		}
		cmdLog.Info("settings saved")

		notifyRunningServer(serverCfg, terminalCfg)

		needsRestart := false
		for _, f := range []string{"port", "no-auth", "no-browser", "show-menubar", "auto-resize"} {
			if cmd.Flags().Changed(f) {
				needsRestart = true
				break
			}
		}
		if needsRestart {
			cmdLog.Warn("note: port, no-auth, no-browser, show-menubar, and auto-resize require a server restart to take effect")
		}
	},
}

var settingsEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config file in an editor",
	Run: func(cmd *cobra.Command, args []string) {
		configPath := resolveConfigPath()

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

// errNoRunningServer is returned by postToRunningServer when there is no lock
// file, i.e. no b3tty server is running.
var errNoRunningServer = errors.New("no b3tty server is running")

// postToRunningServer fires a best-effort POST to the running b3tty server,
// located through its lock file rather than the configured port: the lock
// file holds the port the server actually listens on (which `settings set
// --port` may have just changed in the config), its protocol, and the auth
// token the server requires on its API. It returns errNoRunningServer when
// there is no lock file, and any other error when the server couldn't be
// reached or didn't answer with a 2xx status. Callers for which the update is
// optional ignore the error; the server may simply not be running.
func postToRunningServer(path string, body any) error {
	lock, err := src.ReadLockFile()
	if err != nil {
		return err
	}
	if lock == nil {
		return errNoRunningServer
	}
	scheme := "http"
	httpClient := &http.Client{Timeout: 2 * time.Second}
	if lock.Protocol == "https" {
		scheme = "https"
		httpClient.Transport = &http.Transport{TLSClientConfig: runningServerTLSConfig(certFile)}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s://localhost:%d%s", scheme, lock.Port, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if lock.Token != "" {
		req.Header.Set("Authorization", "Bearer "+lock.Token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s answered %s", path, resp.Status)
	}
	return nil
}

// runningServerTLSConfig returns the TLS settings for reaching the running
// b3tty server over https. When the configured certificate file can be read,
// the connection is accepted only if the server presents exactly that
// certificate: it is pinned rather than verified against a CA, so the
// self-signed and often expired certificates a local server uses still work —
// their dates and host name don't matter when the CLI is asking for the one
// certificate it already has — while a different program listening on the
// port is refused before it can receive the token. When the file can't be
// read, verification is skipped, best effort.
func runningServerTLSConfig(certFile string) *cryptotls.Config {
	// Normal verification is replaced by VerifyPeerCertificate, which Go
	// still calls when InsecureSkipVerify is set.
	cfg := &cryptotls.Config{InsecureSkipVerify: true} //nolint:gosec — pinned below when the cert is readable
	pinned := readPEMCertificates(certFile)
	if len(pinned) == 0 {
		return cfg
	}
	cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) > 0 {
			for _, cert := range pinned {
				if bytes.Equal(rawCerts[0], cert) {
					return nil
				}
			}
		}
		return errors.New("the server's certificate doesn't match the configured cert-file")
	}
	return cfg
}

// readPEMCertificates returns the DER bytes of every certificate in the PEM
// file at path, or nil when there is none or it can't be read.
func readPEMCertificates(path string) [][]byte {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var certs [][]byte
	for {
		var block *pem.Block
		if block, data = pem.Decode(data); block == nil {
			return certs
		}
		if block.Type == "CERTIFICATE" {
			certs = append(certs, block.Bytes)
		}
	}
}

// notifyRunningServer attempts to POST the new settings to the running b3tty
// server so it can update its in-memory state and push the change to any open
// browser sessions. Errors are ignored — the server may simply not be running.
func notifyRunningServer(serverCfg src.SettingsServerConfig, terminalCfg src.TerminalClient) {
	payload := struct {
		Server   src.SettingsServerConfig `json:"server"`
		Terminal src.TerminalClient       `json:"terminal"`
	}{Server: serverCfg, Terminal: terminalCfg}
	_ = postToRunningServer("/settings", payload)
}

// resolveConfigPath returns the active config file path, falling back to the
// default location (~/.config/b3tty/conf.yaml) when no config file was loaded.
func resolveConfigPath() string {
	if p := viper.ConfigFileUsed(); p != "" {
		return p
	}
	if cfgFile != "" {
		return cfgFile
	}
	home, err := os.UserHomeDir()
	if err != nil {
		cmdLog.Fatalf("could not determine home directory: %v", err)
	}
	return filepath.Join(home, src.DOT_CONFIG_PATH, src.B3TTY_CONFIG_PATH, src.CONFIG_FILE_NAME)
}

func anySettingFlagChanged(cmd *cobra.Command) bool {
	for _, name := range []string{"port", "no-auth", "no-browser", "show-menubar", "font-family", "font-size", "auto-resize", "rows", "columns"} {
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
	settingsSetCmd.Flags().StringVar(&setShowMenubar, "show-menubar", src.DEFAULT_SHOW_MENUBAR, "Menu bar visibility: hover, visible, or disable (requires restart)")
	settingsSetCmd.Flags().StringVar(&setFontFamily, "font-family", "", "Terminal font family (empty to use system monospace)")
	settingsSetCmd.Flags().IntVar(&setFontSize, "font-size", 14, "Terminal font size")
	settingsSetCmd.Flags().BoolVar(&setAutoResize, "auto-resize", true, "Auto-resize terminal to fit browser window; when false, rows and columns are used (requires restart)")
	settingsSetCmd.Flags().IntVar(&setRows, "rows", src.DEFAULT_ROWS, "Fixed terminal height in rows, minimum 10 (requires restart; ignored when auto-resize is true)")
	settingsSetCmd.Flags().IntVar(&setColumns, "columns", src.DEFAULT_COLS, "Fixed terminal width in columns, minimum 10 (requires restart; ignored when auto-resize is true)")

	settingsEditCmd.Flags().StringVar(&editEditor, "editor", "", "Editor executable to use (defaults to $VISUAL, $EDITOR, or vi)")
}
