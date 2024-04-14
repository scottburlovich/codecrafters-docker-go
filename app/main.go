package main

import (
	"fmt"
	"os"
	// Uncomment this block to pass the first stage!
	// "os"
	// "os/exec"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	commandName := os.Args[3]
	args := os.Args[4:]
	exitCode, err := runCommand(commandName, args)
	if err != nil {
		fmt.Printf("Error running command: %v\n", err)
	}

	os.Exit(exitCode)
}
