package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

func sandbox() (string, error) {
	dir, err := createSandboxDir("/tmp", "mydocker-*")
	if err != nil {
		return "", err
	}

	err = prepareSandbox(dir)
	if err != nil {
		return "", err
	}

	return dir, nil
}

func createSandboxDir(dir, pattern string) (string, error) {
	tempDir, err := os.MkdirTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("error creating temp directory: %w", err)
	}

	absTempDir, err := filepath.Abs(tempDir)
	if err != nil {
		return "", fmt.Errorf("error getting absolute path: %w", err)
	}

	return absTempDir, nil
}

func prepareSandbox(sandboxDir string) error {
	sandboxBinDir := path.Join(sandboxDir, "/usr/local/bin")

	err := os.MkdirAll(sandboxBinDir, 0755)
	if err != nil {
		return err
	}

	err = copyFile("/usr/local/bin/docker-explorer", path.Join(sandboxBinDir, "docker-explorer"))
	if err != nil {
		return err
	}

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	srcInfo, err := source.Stat()
	if err != nil {
		return err
	}

	destination, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	return nil
}

func cleanupSandbox(sandboxDir string) {
	_ = os.RemoveAll(sandboxDir)
}
