package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	appName        = "tpix-cli"
	configFilename = "settings.json"
)

type Config struct {
	AccessToken       string `json:"accessToken"`
	TypstCachePkgPath string `json:"typstCachePkgPath"`
}

var (
	AppConfig Config
	configDir string
)

func init() {
	dir, err := getConfigDir()
	if err != nil {
		fmt.Println("Get config dir error: ", err)
		return
	}

	configDir = dir
}

func Load() error {
	path := filepath.Join(configDir, configFilename)

	configFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	defer configFile.Close()

	err = json.NewDecoder(configFile).Decode(&AppConfig)
	if err != nil {
		return err
	}

	if AppConfig.TypstCachePkgPath == "" {
		AppConfig.TypstCachePkgPath = defaultCacheDir()
	}

	return nil

}

func Save() error {
	path := filepath.Join(configDir, configFilename)
	configFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	defer configFile.Close()

	if AppConfig.TypstCachePkgPath == "" {
		AppConfig.TypstCachePkgPath = defaultCacheDir()
	}

	err = json.NewEncoder(configFile).Encode(&AppConfig)
	if err != nil {
		return err
	}

	return nil
}

func getConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(configDir, appName)

	_, err = os.Stat(configPath)
	if os.IsNotExist(err) {
		err = os.MkdirAll(configPath, 0755)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	return configPath, nil
}

// defaultCacheDir returns the default typst package cache dir, according to
// https://github.com/typst/packages/blob/main/README.md.
func defaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	cacheDir := filepath.Join(dir, "typst", "packages")

	_, err = os.Stat(cacheDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(cacheDir, 0755)
		if err != nil {
			fmt.Printf("Creating default cache dir failed: %v", err)
		}
	}

	return cacheDir
}
