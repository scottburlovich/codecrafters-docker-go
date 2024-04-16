package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
)

const (
	blobUrl                  = "https://registry.hub.docker.com/v2/%s/blobs/%s"
	manifestUrl              = "https://registry.hub.docker.com/v2/%s/manifests/%s"
	storagePathPrefix        = "/var/lib/mydocker/overlay2"
	sandboxPathPrefix        = storagePathPrefix + "/sandbox"
	imageLayerPathPrefix     = storagePathPrefix + "/image"
	v1ManifestLayerMediaType = "application/vnd.docker.container.image.rootfs.diff.tar.gzip"
	imageIndexMediaType      = "application/vnd.oci.image.index.v1+json"
)

type Config struct {
	MediaType string `json:"mediaType"`
	Size      int    `json:"size"`
	Digest    string `json:"digest"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
	Variant      string `json:"variant"`
}

type ImageIndexEntry struct {
	MediaType string
	Size      int
	Digest    string
	Platform  Platform
}
type ImageIndex struct {
	Manifests []ImageIndexEntry
}

type Jwk struct {
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type Header struct {
	Jwk Jwk    `json:"jwk"`
	Alg string `json:"alg"`
}

type Signature struct {
	Header    Header `json:"header"`
	Signature string `json:"signature"`
	Protected string `json:"protected"`
}

type FsLayer struct {
	BlobSum string `json:"blobSum"`
}

type Compatibility struct {
	V1Compatibility string `json:"v1Compatibility"`
}

type ImageManifestV1 struct {
	SchemaVersion int             `json:"schemaVersion"`
	Name          string          `json:"name"`
	Tag           string          `json:"tag"`
	Architecture  string          `json:"architecture"`
	FsLayers      []FsLayer       `json:"fsLayers"`
	History       []Compatibility `json:"history"`
	Signatures    []Signature     `json:"signatures"`
}

type ImageManifestV1Metadata struct {
	ID           string `json:"id"`
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

type ImageManifestV2 struct {
	SchemaVersion int
	MediaType     string
	Config        Config
	Layers        []Layer
}

type Layer struct {
	MediaType string `json:"mediaType"`
	Size      int    `json:"size"`
	Digest    string `json:"digest"`
}

func requestManifest(image, imageTag, targetOs, targetArch string, authToken AuthToken) (interface{}, error) {
	var err error
	var manifest ImageManifestV2
	if isAuthTokenExpired(authToken) {
		authToken, err = refreshAuthToken(image)
		if err != nil {
			return manifest, fmt.Errorf("Error refreshing auth token: %v\n", err)
		}
	}

	req, err := createManifestRequest(image, imageTag, authToken, imageIndexMediaType)
	if err != nil {
		return manifest, err
	}

	response, err := handleManifestResponse(req)
	if err != nil {
		return manifest, err
	}

	switch response.(type) {
	case ImageManifestV1:
		return response.(ImageManifestV1), nil
	case ImageManifestV2:
		return response.(ImageManifestV2), nil
	case ImageIndex:
		return handleImageIndexResponse(response.(ImageIndex), targetOs, targetArch, image, authToken)
	}

	return manifest, fmt.Errorf("no manifest found for %s/%s", targetOs, targetArch)
}

func downloadAndParseTargetManifest(targetDigest, image string, authToken AuthToken) (interface{}, error) {
	req, err := createManifestRequest(image, targetDigest, authToken, "application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		return nil, err
	}

	response, err := handleManifestResponse(req)
	if err != nil {
		return nil, err
	}

	switch response.(type) {
	case ImageManifestV1:
		return response.(ImageManifestV1), nil
	case ImageManifestV2:
		return response.(ImageManifestV2), nil
	}
	return nil, fmt.Errorf("no target manifest found for specified OS and architecture")
}

func createManifestRequest(image, digest string, authToken AuthToken, acceptHeader string) (*http.Request, error) {
	fmtImageName := fmt.Sprintf("library/%s", image)
	fmtManifestUrl := fmt.Sprintf(manifestUrl, fmtImageName, digest)
	req, err := http.NewRequest("GET", fmtManifestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating request: %v\n", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken.AccessToken))
	req.Header.Set("Accept", acceptHeader)
	return req, nil
}

func handleManifestResponse(req *http.Request) (interface{}, error) {
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error requesting manifest: %v\n", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error downloading manifest: %s", response.Status)
	}
	responseContentType := response.Header.Get("Content-Type")
	return unpackManifestResponse(responseContentType, response.Body)
}

func handleImageIndexResponse(imageIndex ImageIndex, targetOs, targetArch, image string, authToken AuthToken) (interface{}, error) {
	var targetDigest string
	for _, m := range imageIndex.Manifests {
		mp := m.Platform
		if mp.Os == targetOs && mp.Architecture == targetArch {
			targetDigest = m.Digest
			break
		}
	}
	if targetDigest != "" {
		return downloadAndParseTargetManifest(targetDigest, image, authToken)
	}
	return nil, fmt.Errorf("no target manifest found for %s/%s", targetOs, targetArch)
}

func downloadV1ManifestLayers(manifest ImageManifestV1, image string, authToken AuthToken) ([]string, error) {
	var fetchedLayers []string
	var metadata ImageManifestV1Metadata
	err := json.Unmarshal([]byte(manifest.History[0].V1Compatibility), &metadata)
	if err != nil {
		return nil, fmt.Errorf("Error parsing manifest metadata: %v\n", err)
	}
	imageId := metadata.ID

	for _, layer := range manifest.FsLayers {
		var err error
		var compressedLayerFilename string
		var packedLayerFilename string

		if isAuthTokenExpired(authToken) {
			authToken, err = refreshAuthToken(image)
			if err != nil {
				return nil, fmt.Errorf("Error refreshing auth token: %v\n", err)
			}
		}

		fmtImageName := fmt.Sprintf("library/%s", image)
		layerResponse, err := downloadManifestLayer(fmtImageName, layer.BlobSum, v1ManifestLayerMediaType, authToken)
		if err != nil {
			return nil, err
		}
		defer layerResponse.Body.Close()

		fileType, compressor, filename := getLayerFileInfo(layer.BlobSum, v1ManifestLayerMediaType)
		filePath := fmt.Sprintf("%s/%s", imageLayerPathPrefix, imageId)

		err = storeLayer(filePath, filename, layerResponse)
		if err != nil {
			return nil, err
		}

		filename, err = decompressLayer(compressor, compressedLayerFilename, filename, filePath)
		if err != nil {
			return nil, err
		}

		_, err = unpackLayer(fileType, packedLayerFilename, filename, filePath, layer)
		if err != nil {
			return nil, err
		}

		fetchedLayers = append(fetchedLayers, layer.BlobSum)
	}

	return fetchedLayers, nil
}

func downloadV2ManifestLayers(manifest ImageManifestV2, image string, authToken AuthToken) ([]string, error) {
	var fetchedLayers []string
	imageId := manifest.Config.Digest

	for _, layer := range manifest.Layers {
		var err error
		var compressedLayerFilename string
		var packedLayerFilename string

		if isAuthTokenExpired(authToken) {
			authToken, err = refreshAuthToken(image)
			if err != nil {
				return nil, fmt.Errorf("Error refreshing auth token: %v\n", err)
			}
		}

		fmtImageName := fmt.Sprintf("library/%s", image)
		layerResponse, err := downloadManifestLayer(fmtImageName, layer.Digest, layer.MediaType, authToken)
		if err != nil {
			return nil, err
		}
		defer layerResponse.Body.Close()

		fileType, compressor, filename := getLayerFileInfo(layer.Digest, layer.MediaType)
		filePath := fmt.Sprintf("%s/%s", imageLayerPathPrefix, imageId)

		err = storeLayer(filePath, filename, layerResponse)
		if err != nil {
			return nil, err
		}

		filename, err = decompressLayer(compressor, compressedLayerFilename, filename, filePath)
		if err != nil {
			return nil, err
		}

		_, err = unpackLayer(fileType, packedLayerFilename, filename, filePath, layer)
		if err != nil {
			return nil, err
		}

		fetchedLayers = append(fetchedLayers, layer.Digest)
	}

	return fetchedLayers, nil
}

func downloadManifestLayer(fmtImageName string, layerId string, acceptHeader string, authToken AuthToken) (*http.Response, error) {
	layerUrl := fmt.Sprintf(blobUrl, fmtImageName, layerId)
	req, err := http.NewRequest("GET", layerUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating layer request: %v\n", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken.AccessToken))
	req.Header.Set("Accept", acceptHeader)

	layerResponse, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error downloading layer: %v\n", err)
	}

	if layerResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error downloading layer %s: %s", layerId, layerResponse.Status)
	}
	return layerResponse, nil
}

func unpackLayer(fileType string, packedLayerFilename string, filename string, filePath string, layer interface{}) (string, error) {
	var layerExtractDir string
	switch layer.(type) {
	case FsLayer:
		layerExtractDir = path.Join(filePath, layer.(FsLayer).BlobSum, "rootfs")
	case Layer:
		layerExtractDir = path.Join(filePath, layer.(Layer).Digest, "rootfs")
	}
	if fileType == "tar" {
		packedLayerFilename = filename

		err := os.MkdirAll(layerExtractDir, 0755)
		if err != nil {
			return "", fmt.Errorf("Error creating directory: %v\n", err)
		}

		err = exec.Command("tar", "-xf", path.Join(filePath, filename), "-C", layerExtractDir).Run()
		if err != nil {
			return "", fmt.Errorf("Error extracting layer: %v\n", err)
		}

		err = os.Remove(path.Join(filePath, packedLayerFilename))
		if err != nil {
			return "", fmt.Errorf("Error removing packed layer: %v\n", err)
		}
	} else {
		return "", fmt.Errorf("Unsupported file type: %s\n", fileType)
	}
	return layerExtractDir, nil
}

func decompressLayer(compressor string, compressedLayerFilename string, filename string, filePath string) (string, error) {
	if compressor != "" {
		compressedLayerFilename = filename
		switch compressor {
		case "gzip":
			err := exec.Command("gzip", "-d", "-k", path.Join(filePath, filename)).Run()
			if err != nil {
				return "", fmt.Errorf("Error decompressing layer: %v\n", err)
			}
			filename = filename[:len(filename)-3]
		case "zstd":
			err := exec.Command("zstd", "-d", path.Join(filePath, filename)).Run()
			if err != nil {
				return "", fmt.Errorf("Error decompressing layer: %v\n", err)
			}
			filename = filename[:len(filename)-5]
		default:
			return "", fmt.Errorf("Unsupported compressor: %s\n", compressor)
		}

		err := os.Remove(path.Join(filePath, compressedLayerFilename))
		if err != nil {
			return "", fmt.Errorf("Error removing compressed layer: %v\n", err)
		}
	}
	return filename, nil
}

func storeLayer(filePath string, filename string, layerResponse *http.Response) error {
	err := os.MkdirAll(filePath, 0755)
	if err != nil {
		return fmt.Errorf("Error creating directory: %v\n", err)
	}
	writePath := path.Join(filePath, filename)
	file, err := os.Create(writePath)
	if err != nil {
		return fmt.Errorf("Error creating file %s: %v\n", writePath, err)
	}
	defer file.Close()

	_, err = io.Copy(file, layerResponse.Body)
	if err != nil {
		return fmt.Errorf("Error saving layer: %v\n", err)
	}

	return nil
}

func getLayerFileInfo(layerDigest, layerMediaType string) (string, string, string) {
	var filetype, compressor, filename string

	switch layerMediaType {
	case v1ManifestLayerMediaType:
		filetype = "tar"
		compressor = "gzip"
		filename = fmt.Sprintf("%s.tar.gz", layerDigest)
	default:
		filetype, compressor = getFileTypeAndCompressor(layerMediaType)
		filename = buildPackedLayerFilename(layerDigest, filetype, compressor)
	}

	return filetype, compressor, filename
}

func getFileTypeAndCompressor(layerMediaType string) (string, string) {

	mediaTypeParts := strings.Split(layerMediaType, "+")
	filetype := mediaTypeParts[0][strings.LastIndex(mediaTypeParts[0], ".")+1:]
	compressor := ""
	if len(mediaTypeParts) > 1 {
		compressor = mediaTypeParts[1]
	}
	return filetype, compressor
}

func buildPackedLayerFilename(layerDigest string, layerFileType, layerCompressor string) string {
	fileExt0 := ""
	fileExt1 := ""

	switch layerFileType {
	case "tar":
		fileExt0 = "tar"
	}

	switch layerCompressor {
	case "gzip":
		fileExt1 = "gz"
	case "zstd":
		fileExt1 = "zst"
	}

	if fileExt1 == "" {
		return fmt.Sprintf("%s.%s", layerDigest, fileExt0)
	} else {
		return fmt.Sprintf("%s.%s.%s", layerDigest, fileExt0, fileExt1)
	}
}

func unpackManifestResponse(contentType string, body io.ReadCloser) (interface{}, error) {
	defer body.Close()

	switch contentType {
	case "application/vnd.docker.distribution.manifest.v1+prettyjws":
		var manifest ImageManifestV1
		decoder := json.NewDecoder(body)
		if err := decoder.Decode(&manifest); err != nil {
			return nil, err
		}
		return manifest, nil
	case "application/vnd.docker.distribution.manifest.v2+json", "application/vnd.oci.image.manifest.v1+json":
		var manifest ImageManifestV2
		decoder := json.NewDecoder(body)
		if err := decoder.Decode(&manifest); err != nil {
			return nil, err
		}
		return manifest, nil
	case "application/vnd.oci.image.index.v1+json":
		var manifest ImageIndex
		decoder := json.NewDecoder(body)
		if err := decoder.Decode(&manifest); err != nil {
			return nil, err
		}
		return manifest, nil
	default:
		return nil, errors.New("invalid content type")
	}
}
