package main

const (
	authenticationUrl        = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull"
	blobUrl                  = "https://registry.hub.docker.com/v2/%s/blobs/%s"
	manifestUrl              = "https://registry.hub.docker.com/v2/%s/manifests/%s"
	storagePathPrefix        = "/var/lib/mydocker/overlay2"
	imageLayerPathPrefix     = storagePathPrefix + "/image"
	v1ManifestLayerMediaType = "application/vnd.docker.container.image.rootfs.diff.tar.gzip"
	imageIndexMediaType      = "application/vnd.oci.image.index.v1+json"
)
