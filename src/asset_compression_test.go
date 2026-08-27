package src

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// contentTypeFor
// ---------------------------------------------------------------------------

func TestContentTypeFor(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		content  []byte
		expected string
	}{
		{
			name:     "javascript extension",
			fileName: "assets/terminal.min.js",
			content:  []byte("console.log('hi')"),
			expected: "text/javascript; charset=utf-8",
		},
		{
			name:     "css extension",
			fileName: "assets/terminal.css",
			content:  []byte("body { color: red; }"),
			expected: "text/css; charset=utf-8",
		},
		{
			name:     "ico extension not covered by Go's builtin mime table",
			fileName: "assets/favicon.ico",
			content:  []byte{0x00, 0x00, 0x01, 0x00},
			expected: "image/x-icon",
		},
		{
			name:     "unrecognized extension falls back to content sniffing",
			fileName: "assets/mystery.zzz123",
			content:  []byte("plain text content"),
			expected: "text/plain; charset=utf-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, contentTypeFor(tt.fileName, tt.content))
		})
	}
}

// ---------------------------------------------------------------------------
// compressBytes
// ---------------------------------------------------------------------------

func TestCompressBytes(t *testing.T) {
	t.Run("highly compressible data shrinks under both algorithms", func(t *testing.T) {
		data := bytes.Repeat([]byte("hello world "), 200)
		gz, br := compressBytes(data)
		require.NotNil(t, gz)
		require.NotNil(t, br)
		assert.Less(t, len(gz), len(data))
		assert.Less(t, len(br), len(data))
	})

	t.Run("gzip output round-trips to the original", func(t *testing.T) {
		data := bytes.Repeat([]byte("round trip me "), 100)
		gz, _ := compressBytes(data)
		require.NotNil(t, gz)
		r, err := gzip.NewReader(bytes.NewReader(gz))
		require.NoError(t, err)
		decoded, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, data, decoded)
	})

	t.Run("brotli output round-trips to the original", func(t *testing.T) {
		data := bytes.Repeat([]byte("round trip me "), 100)
		_, br := compressBytes(data)
		require.NotNil(t, br)
		decoded, err := io.ReadAll(brotli.NewReader(bytes.NewReader(br)))
		require.NoError(t, err)
		assert.Equal(t, data, decoded)
	})

	t.Run("tiny input that would not shrink returns nil for both", func(t *testing.T) {
		gz, br := compressBytes([]byte("a"))
		assert.Nil(t, gz)
		assert.Nil(t, br)
	})

	t.Run("empty input returns nil for both", func(t *testing.T) {
		gz, br := compressBytes([]byte{})
		assert.Nil(t, gz)
		assert.Nil(t, br)
	})
}

// ---------------------------------------------------------------------------
// buildCompressedAssets
// ---------------------------------------------------------------------------

func TestBuildCompressedAssets(t *testing.T) {
	t.Run("walks every file and skips directories", func(t *testing.T) {
		fsys := fstest.MapFS{
			"assets/terminal.min.js": {Data: bytes.Repeat([]byte("x"), 100)},
			"assets/terminal.css":    {Data: bytes.Repeat([]byte("y"), 100)},
		}
		result, err := buildCompressedAssets(fsys)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Contains(t, result, "assets/terminal.min.js")
		assert.Contains(t, result, "assets/terminal.css")
	})

	t.Run("original bytes are preserved exactly", func(t *testing.T) {
		content := []byte("some file content")
		fsys := fstest.MapFS{"assets/file.txt": {Data: content}}
		result, err := buildCompressedAssets(fsys)
		require.NoError(t, err)
		assert.Equal(t, content, result["assets/file.txt"].original)
	})
}

// ---------------------------------------------------------------------------
// mustBuildCompressedAssets
// ---------------------------------------------------------------------------

func TestMustBuildCompressedAssets(t *testing.T) {
	t.Run("valid fs does not panic", func(t *testing.T) {
		fsys := fstest.MapFS{"assets/file.txt": {Data: []byte("content")}}
		assert.NotPanics(t, func() { mustBuildCompressedAssets(fsys) })
	})

	t.Run("real embedded assets build without panicking", func(t *testing.T) {
		// compressedAssets is already computed at package init from the real
		// assets embed.FS; this just asserts it actually happened and is
		// non-empty rather than re-invoking the panic path.
		assert.NotEmpty(t, compressedAssets)
	})
}

// ---------------------------------------------------------------------------
// assetsHandler
// ---------------------------------------------------------------------------

func TestAssetsHandler(t *testing.T) {
	t.Run("GET without Accept-Encoding returns the original bytes", func(t *testing.T) {
		path, asset := anyCompressedAsset(t)
		req := httptest.NewRequest("GET", "/"+path, nil)
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, 200, w.Code)
		assert.Empty(t, w.Header().Get("Content-Encoding"))
		assert.Equal(t, asset.original, w.Body.Bytes())
		assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))
	})

	t.Run("GET with br in Accept-Encoding serves brotli when available", func(t *testing.T) {
		path, asset := assetWithCompressionOrSkip(t, func(a compressedAsset) bool { return a.brotli != nil })
		req := httptest.NewRequest("GET", "/"+path, nil)
		req.Header.Set("Accept-Encoding", "gzip, br")
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, "br", w.Header().Get("Content-Encoding"))
		assert.Equal(t, asset.brotli, w.Body.Bytes())
	})

	t.Run("GET with only gzip in Accept-Encoding serves gzip", func(t *testing.T) {
		path, asset := assetWithCompressionOrSkip(t, func(a compressedAsset) bool { return a.gzip != nil })
		req := httptest.NewRequest("GET", "/"+path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
		assert.Equal(t, asset.gzip, w.Body.Bytes())
	})

	t.Run("Content-Length matches the actual served body", func(t *testing.T) {
		path, _ := assetWithCompressionOrSkip(t, func(a compressedAsset) bool { return a.gzip != nil })
		req := httptest.NewRequest("GET", "/"+path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, strconv.Itoa(w.Body.Len()), w.Header().Get("Content-Length"))
	})

	t.Run("HEAD returns headers with no body", func(t *testing.T) {
		path, asset := anyCompressedAsset(t)
		req := httptest.NewRequest("HEAD", "/"+path, nil)
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, 200, w.Code)
		assert.Empty(t, w.Body.Bytes())
		assert.Equal(t, strconv.Itoa(len(asset.original)), w.Header().Get("Content-Length"))
	})

	t.Run("POST is rejected with 405", func(t *testing.T) {
		path, _ := anyCompressedAsset(t)
		req := httptest.NewRequest("POST", "/"+path, nil)
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, 405, w.Code)
	})

	t.Run("unknown path returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/assets/does-not-exist.js", nil)
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, 404, w.Code)
	})
}

// ---------------------------------------------------------------------------
// distHandler / compressedDistAssets
// ---------------------------------------------------------------------------

func TestDistHandler(t *testing.T) {
	t.Run("serves a real dist asset with the correct bytes", func(t *testing.T) {
		var path string
		var asset compressedAsset
		for p, a := range compressedDistAssets {
			path, asset = p, a
			break
		}
		require.NotEmpty(t, path, "compressedDistAssets is empty")

		req := httptest.NewRequest("GET", "/"+path, nil)
		w := httptest.NewRecorder()
		distHandler(w, req)
		assert.Equal(t, 200, w.Code)
		assert.Equal(t, asset.original, w.Body.Bytes())
	})

	t.Run("a dist-only path is not reachable through assetsHandler, and vice versa", func(t *testing.T) {
		var distPath string
		for p := range compressedDistAssets {
			distPath = p
			break
		}
		require.NotEmpty(t, distPath)

		req := httptest.NewRequest("GET", "/"+distPath, nil)
		w := httptest.NewRecorder()
		assetsHandler(w, req)
		assert.Equal(t, 404, w.Code, "dist/ path must not be reachable via the /assets/ handler")

		assetsPath, _ := anyCompressedAsset(t)
		req = httptest.NewRequest("GET", "/"+assetsPath, nil)
		w = httptest.NewRecorder()
		distHandler(w, req)
		assert.Equal(t, 404, w.Code, "assets/ path must not be reachable via the /dist/ handler")
	})
}

// anyCompressedAsset returns an arbitrary path/asset pair from the real,
// package-init-computed compressedAssets map, failing the test if it's empty.
func anyCompressedAsset(t *testing.T) (string, compressedAsset) {
	t.Helper()
	for path, asset := range compressedAssets {
		return path, asset
	}
	t.Fatal("compressedAssets is empty")
	return "", compressedAsset{}
}

// assetWithCompressionOrSkip returns the first real asset matching pred, or
// skips the test if none of the embedded assets happen to satisfy it (e.g.
// no file compresses under a given algorithm).
func assetWithCompressionOrSkip(t *testing.T, pred func(compressedAsset) bool) (string, compressedAsset) {
	t.Helper()
	for path, asset := range compressedAssets {
		if pred(asset) {
			return path, asset
		}
	}
	t.Skip("no embedded asset satisfies the predicate")
	return "", compressedAsset{}
}
