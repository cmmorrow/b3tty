package src

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteLockFile(t *testing.T) {
	t.Run("creates the file with correct content", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		require.NoError(t, WriteLockFile(8080, 12345, "testtoken", "http"))

		path, err := lockFilePath()
		require.NoError(t, err)
		_, err = os.Stat(path)
		assert.NoError(t, err, "lock file should exist after write")
	})

	t.Run("creates parent directory if missing", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		require.NoError(t, WriteLockFile(8080, 1, "tok", "http"))

		path, err := lockFilePath()
		require.NoError(t, err)
		_, err = os.Stat(filepath.Dir(path))
		assert.NoError(t, err, "config directory should be created")
	})

	t.Run("overwrites an existing lock file", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		require.NoError(t, WriteLockFile(8080, 1, "first", "http"))
		require.NoError(t, WriteLockFile(9090, 2, "second", "https"))

		lf, err := ReadLockFile()
		require.NoError(t, err)
		require.NotNil(t, lf)
		assert.Equal(t, 9090, lf.Port)
		assert.Equal(t, "second", lf.Token)
	})

	t.Run("empty token is written correctly for no-auth mode", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		require.NoError(t, WriteLockFile(8080, 1, "", "http"))

		lf, err := ReadLockFile()
		require.NoError(t, err)
		require.NotNil(t, lf)
		assert.Equal(t, "", lf.Token)
	})
}

func TestReadLockFile(t *testing.T) {
	t.Run("returns nil when no file exists", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		lf, err := ReadLockFile()
		assert.NoError(t, err)
		assert.Nil(t, lf)
	})

	t.Run("returns correct values after write", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		require.NoError(t, WriteLockFile(9090, 99999, "abc123", "https"))

		lf, err := ReadLockFile()
		require.NoError(t, err)
		require.NotNil(t, lf)
		assert.Equal(t, 9090, lf.Port)
		assert.Equal(t, 99999, lf.PID)
		assert.Equal(t, "abc123", lf.Token)
		assert.Equal(t, "https", lf.Protocol)
	})

	t.Run("returns error for malformed JSON", func(t *testing.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		lockDir := filepath.Join(tmpHome, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH)
		require.NoError(t, os.MkdirAll(lockDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, LOCK_FILE_NAME), []byte("not valid json"), 0600))

		lf, err := ReadLockFile()
		assert.Error(t, err)
		assert.Nil(t, lf)
	})
}

func TestRemoveLockFile(t *testing.T) {
	t.Run("removes an existing lock file", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		require.NoError(t, WriteLockFile(8080, 1, "tok", "http"))
		require.NoError(t, RemoveLockFile())

		path, err := lockFilePath()
		require.NoError(t, err)
		_, statErr := os.Stat(path)
		assert.True(t, errors.Is(statErr, os.ErrNotExist), "lock file should not exist after remove")
	})

	t.Run("no error when file does not exist", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		assert.NoError(t, RemoveLockFile())
	})
}

func TestLockFileRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := LockFile{Port: 8443, PID: 55555, Token: "secrettoken", Protocol: "https"}
	require.NoError(t, WriteLockFile(want.Port, want.PID, want.Token, want.Protocol))

	got, err := ReadLockFile()
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want, *got)
}
