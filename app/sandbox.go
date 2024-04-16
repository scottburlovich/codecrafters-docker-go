package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

const (
	sandboxPathPrefix = storagePathPrefix + "/sandbox"
)

func sandbox(imageId string) (string, error) {
	dir, err := createSandboxDir()
	if err != nil {
		return "", err
	}

	err = prepareSandbox(imageId, dir)
	if err != nil {
		return "", err
	}

	return dir, nil
}

func createSandboxDir() (string, error) {
	sandboxId, err := GenerateRandomSHA256()
	if err != nil {
		return "", fmt.Errorf("error generating random sandbox id: %w", err)
	}

	sandboxIdPath := "sha256:" + sandboxId
	sandboxDir := path.Join(sandboxPathPrefix, sandboxIdPath)

	if _, err := os.Stat(sandboxDir); !os.IsNotExist(err) {
		fmt.Printf("sandbox %s does not exist\n", sandboxIdPath)
		err = os.MkdirAll(sandboxDir, 0755)
		if err != nil {
			return "", fmt.Errorf("error creating temp directory: %w", err)
		}
	}

	return sandboxDir, nil
}

func prepareSandbox(imageId, sandboxDir string) error {
	imageLayerPath := path.Join(imageLayerPathPrefix, imageId, "/")
	if _, err := os.Stat(imageLayerPath); os.IsNotExist(err) {
		fmt.Printf("image %s layers path not found: %s\n", imageId, imageLayerPath)
		return fmt.Errorf("image %s layers path not found: %s", imageId, imageLayerPath)
	}

	layerIds, err := os.ReadDir(imageLayerPath)
	if err != nil {
		return fmt.Errorf("error reading image %s layer(s) path: %w", imageId, err)
	}

	for _, layerId := range layerIds {
		layerIdPath := path.Join(imageLayerPath, layerId.Name(), "/")

		err = copyDir(layerIdPath, sandboxDir)
		if err != nil {
			errorMessage := fmt.Sprintf("Error copying image %s layer(s) to sandbox: %v", imageId, err)
			fmt.Println(errorMessage)
			return fmt.Errorf(errorMessage)
		}
	}

	return nil
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relativePath)

		if info.Mode()&os.ModeSymlink != 0 {
			return copySymLink(path, destPath)
		}

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

func copySymLink(source, dest string) error {
	linkDest, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(linkDest, dest)
}

func copyFile(source, dest string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("Error opening source file: %v\n", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("Error creating destination file: %v\n", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("Error copying file: %v\n", err)
	}

	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("Error getting source file info: %v\n", err)
	}

	return os.Chmod(dest, sourceInfo.Mode())
}

func cleanupSandbox(sandboxDir string) {
	_ = os.RemoveAll(sandboxDir)
}

func GenerateRandomSHA256() (string, error) {
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(randomBytes)

	return hex.EncodeToString(hash[:]), nil
}
