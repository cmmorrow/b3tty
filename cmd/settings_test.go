package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cmmorrow/b3tty/src"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runningServer starts a test server recording the last request it received,
// and writes a lock file (under a temp HOME) pointing at it with the given
// token, as a running b3tty server would.
func runningServer(t *testing.T, token string) *http.Request {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	got := &http.Request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = *r
		got.Body = io.NopCloser(nil)
		got.Header = r.Header.Clone()
		got.Form = url.Values{"body": {string(body)}}
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	require.NoError(t, src.WriteLockFile(port, 1, token, "http"))
	return got
}

func TestPostToRunningServer(t *testing.T) {
	t.Run("posts to the port in the lock file with the token as a bearer header", func(t *testing.T) {
		got := runningServer(t, "lock-token")
		require.NoError(t, postToRunningServer("/settings", map[string]int{"n": 1}))
		assert.Equal(t, http.MethodPost, got.Method)
		assert.Equal(t, "/settings", got.URL.Path)
		assert.Equal(t, "Bearer lock-token", got.Header.Get("Authorization"))
		assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
		var body map[string]int
		require.NoError(t, json.Unmarshal([]byte(got.Form.Get("body")), &body))
		assert.Equal(t, 1, body["n"])
	})

	t.Run("sends no Authorization header when the server runs without auth", func(t *testing.T) {
		got := runningServer(t, "")
		postToRunningServer("/settings", struct{}{})
		assert.Equal(t, "/settings", got.URL.Path)
		assert.Empty(t, got.Header.Get("Authorization"))
	})

	t.Run("reports when no server is running", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		assert.ErrorIs(t, postToRunningServer("/settings", struct{}{}), errNoRunningServer)
	})

	t.Run("reports a server that can't be reached or refuses", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusInternalServerError)
		}))
		u, err := url.Parse(server.URL)
		require.NoError(t, err)
		port, err := strconv.Atoi(u.Port())
		require.NoError(t, err)
		require.NoError(t, src.WriteLockFile(port, 1, "", "http"))
		require.ErrorContains(t, postToRunningServer("/reload-themes", struct{}{}), "500")

		server.Close()
		err = postToRunningServer("/reload-themes", struct{}{})
		require.Error(t, err)
		assert.NotErrorIs(t, err, errNoRunningServer)
	})
}

// expiredCert returns a self-signed certificate for localhost that expired a
// year ago, and its PEM encoding.
func expiredCert(t *testing.T) (cryptotls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-2 * 365 * 24 * time.Hour),
		NotAfter:     time.Now().Add(-365 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return cryptotls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestPostToRunningServerTLS(t *testing.T) {
	// runningTLSServer starts an https server presenting cert and writes a
	// lock file pointing at it.
	runningTLSServer := func(t *testing.T, cert cryptotls.Certificate) {
		t.Helper()
		t.Setenv("HOME", t.TempDir())
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.TLS = &cryptotls.Config{Certificates: []cryptotls.Certificate{cert}}
		server.StartTLS()
		t.Cleanup(server.Close)
		u, err := url.Parse(server.URL)
		require.NoError(t, err)
		port, err := strconv.Atoi(u.Port())
		require.NoError(t, err)
		require.NoError(t, src.WriteLockFile(port, 1, "token", "https"))
	}
	useCertFile := func(t *testing.T, path string) {
		t.Helper()
		saved := certFile
		certFile = path
		t.Cleanup(func() { certFile = saved })
	}

	t.Run("accepts the configured certificate even when it has expired", func(t *testing.T) {
		cert, certPEM := expiredCert(t)
		runningTLSServer(t, cert)
		path := filepath.Join(t.TempDir(), "cert.pem")
		require.NoError(t, os.WriteFile(path, certPEM, 0600))
		useCertFile(t, path)
		require.NoError(t, postToRunningServer("/reload-themes", struct{}{}))
	})

	t.Run("refuses a server presenting a different certificate", func(t *testing.T) {
		cert, _ := expiredCert(t)
		runningTLSServer(t, cert)
		_, otherPEM := expiredCert(t)
		path := filepath.Join(t.TempDir(), "cert.pem")
		require.NoError(t, os.WriteFile(path, otherPEM, 0600))
		useCertFile(t, path)
		require.ErrorContains(t, postToRunningServer("/reload-themes", struct{}{}), "doesn't match the configured cert-file")
	})

	t.Run("falls back to not verifying when the cert file can't be read", func(t *testing.T) {
		cert, _ := expiredCert(t)
		runningTLSServer(t, cert)
		useCertFile(t, filepath.Join(t.TempDir(), "missing.pem"))
		require.NoError(t, postToRunningServer("/reload-themes", struct{}{}))
	})
}
