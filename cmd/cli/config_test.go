package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigReturnsDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, NewDefaultConfig(), cfg)
	require.Equal(t, tileThemeEmoji, cfg.TileTheme)
}

func TestLoadConfigParsesPartialFileAndFillsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("nickname = \"racoo\"\n"), 0o600))
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, "racoo", cfg.Nickname)
	require.Equal(t, defaultServerURL, cfg.ServerURL)
	require.Equal(t, tileThemeEmoji, cfg.TileTheme)
	require.Equal(t, defaultClaimTimeout, cfg.ClaimTimeoutMS)
}

func TestLoadConfigRejectsMalformedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("not = valid = toml"), 0o600))
	_, err := LoadConfig(path)
	require.Error(t, err)
}

func TestSaveConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lsp", "config.toml")
	want := Config{
		Nickname:       "alice",
		ServerURL:      "wss://example.com/ws",
		SessionToken:   "tok-123",
		TileTheme:      tileThemeASCII,
		ClaimTimeoutMS: 3000,
	}
	require.NoError(t, SaveConfig(path, want))
	got, err := LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestSaveConfigNormalizesInvalidTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, SaveConfig(path, Config{Nickname: "bob", TileTheme: "weird"}))
	got, err := LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, tileThemeEmoji, got.TileTheme)
}

func TestSaveConfigUsesAtomicRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, SaveConfig(path, Config{Nickname: "carl"}))
	tmp := path + ".tmp"
	_, err := os.Stat(tmp)
	require.True(t, os.IsNotExist(err), "tmp file should be removed after rename")
}

func TestSaveConfigErrorsOnEmptyPath(t *testing.T) {
	err := SaveConfig("", Config{Nickname: "x"})
	require.Error(t, err)
}
