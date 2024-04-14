package main

import (
	"log"
	"os"
	"os/exec"
	// Uncomment this block to pass the first stage!
	// "os"
	// "os/exec"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	commandName := os.Args[3]
	args := os.Args[4:]
	runCommand(commandName, args)
}

func runCommand(commandName string, args []string) {
	command := exec.Command(commandName, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
