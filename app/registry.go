package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	authenticationUrl = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull"
)

type AuthToken struct {
	AccessToken string `json:"access_token"`
	Issued      string `json:"issued_at"`
	Expires     int    `json:"expires_in"`
}

func getHostPlatform() Platform {
	return Platform{
		Architecture: runtime.GOARCH,
		Os:           runtime.GOOS,
	}
}

func resolveImageDigest(image string) (string, error) {
	imageName, imageTag := parseImage(image)
	authToken, err := requestAuthToken(imageName)
	if err != nil {
		return "", fmt.Errorf("Error requesting registry auth token: %v\n", err)
	}

	manifest, err := requestManifest(imageName, imageTag, getHostPlatform().Os, getHostPlatform().Architecture, authToken)
	if err != nil {
		return "", fmt.Errorf("Error requesting image manifest: %v\n", err)
	}

	switch manifest.(type) {
	case ImageManifestV1:
		compatData := manifest.(ImageManifestV1).History[0].V1Compatibility
		var metadataStruct ImageManifestV1Metadata
		err = json.Unmarshal([]byte(compatData), &metadataStruct)
		if err != nil {
			return "", fmt.Errorf("Error parsing manifest metadata: %v\n", err)
		}
		return metadataStruct.ID, nil
	case ImageManifestV2:
		return manifest.(ImageManifestV2).Config.Digest, nil
	}

	return "", errors.New("invalid manifest type")
}

func pullImage(image string) (string, error) {
	imageName, imageTag := parseImage(image)

	accessToken, err := requestAuthToken(imageName)
	if err != nil {
		fmt.Printf("Error requesting registry auth token: %v\n", err)
		return "", fmt.Errorf("Error requesting registry auth token: %v\n", err)
	}

	manifest, err := requestManifest(imageName, imageTag, getHostPlatform().Os, getHostPlatform().Architecture, accessToken)
	if err != nil {
		fmt.Printf("Error requesting image manifest: %v\n", err)
		return "", fmt.Errorf("Error requesting image manifest: %v\n", err)
	}

	var imagePath string
	switch manifest.(type) {
	case ImageManifestV1:
		_, err = downloadV1ManifestLayers(manifest.(ImageManifestV1), imageName, accessToken)
		if err != nil {
			fmt.Printf("Error downloading image layers: %v\n", err)
			return "", fmt.Errorf("Error downloading image layers: %v\n", err)
		}
		compat := manifest.(ImageManifestV1).History[0].V1Compatibility
		var metadata ImageManifestV1Metadata
		err = json.Unmarshal([]byte(compat), &metadata)
		if err != nil {
			fmt.Printf("Error parsing manifest metadata: %v\n", err)
			return "", fmt.Errorf("Error parsing manifest metadata: %v\n", err)
		}
		imagePath = fmt.Sprintf("%s/%s", imageLayerPathPrefix, metadata.ID)
	case ImageManifestV2:
		_, err = downloadV2ManifestLayers(manifest.(ImageManifestV2), imageName, accessToken)
		if err != nil {
			fmt.Printf("Error downloading image layers: %v\n", err)
			return "", fmt.Errorf("Error downloading image layers: %v\n", err)
		}
		imagePath = fmt.Sprintf("%s/%s", imageLayerPathPrefix, manifest.(ImageManifestV2).Config.Digest)
	}

	return imagePath, nil
}

func parseImage(image string) (string, string) {
	imageParts := strings.Split(image, ":")
	if len(imageParts) == 1 {
		return imageParts[0], "latest"
	}
	return imageParts[0], imageParts[1]
}

func requestAuthToken(repository string) (AuthToken, error) {
	var token AuthToken

	if !strings.Contains(repository, "library/") {
		repository = fmt.Sprintf("library/%s", repository)
	}

	tokenResponse, err := http.Get(fmt.Sprintf(authenticationUrl, repository))
	if err != nil {
		return token, fmt.Errorf("Error requesting auth token: %v\n", err)
	}
	defer tokenResponse.Body.Close()

	if tokenResponse.StatusCode != http.StatusOK {
		return token, fmt.Errorf("error fetching auth token: %s", tokenResponse.Status)
	}

	err = json.NewDecoder(tokenResponse.Body).Decode(&token)
	if err != nil {
		return token, fmt.Errorf("Error decoding auth token: %v\n", err)
	}

	return token, nil
}

func isAuthTokenExpired(token AuthToken) bool {
	issuedTime, err := time.Parse(time.RFC3339, token.Issued)
	if err != nil {
		return true
	}
	expirationTime := issuedTime.Add(time.Duration(token.Expires) * time.Second)

	return time.Now().After(expirationTime)
}

func refreshAuthToken(image string) (AuthToken, error) {
	newAuthToken, err := requestAuthToken(image)
	if err != nil {
		return AuthToken{}, err
	}

	return newAuthToken, nil
}
