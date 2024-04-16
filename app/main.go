package main

import (
	"fmt"
	"os"
)

func main() {
	image := os.Args[2]
	commandName := os.Args[3]
	args := os.Args[4:]

	imageId, err := resolveImageDigest(image)
	if err != nil {
		fmt.Printf("Error resolving image digest: %v\n", err)
		os.Exit(1)
	}

	imagePath := fmt.Sprintf("%s/%s", imageLayerPathPrefix, imageId)

	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		_, pullErr := pullImage(image)
		if pullErr != nil {
			fmt.Printf("Error fetching image: %v\n", err)
			os.Exit(1)
		}
	}

	exitCode, err := runCommand(imageId, commandName, args)
	if err != nil {
		fmt.Printf("Error running command: %v\n", err)
	}

	os.Exit(exitCode)
}
