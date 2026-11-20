package testutil

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

func InstalledLilt() (mygo.SecretsModule, string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return mygo.SecretsModule{}, "", err
	}
	return mygo.Secrets.Service("dev.amamiya.qqmusic"), filepath.Join(configDir, "Lilt"), nil
}
