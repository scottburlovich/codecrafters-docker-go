package main

import (
	"errors"
	"fmt"
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
	exitCode, err := runCommand(commandName, args)
	if err != nil {
		fmt.Printf("Error running command: %v\n", err)
	}

	os.Exit(exitCode)
}

func runCommand(commandName string, args []string) (int, error) {
	command := exec.Command(commandName, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	err := command.Start()
	if err != nil {
		return 1, err
	}
	err = command.Wait()
	if err != nil {
		return determineExitCode(err), err
	}

	return 0, nil
}

func determineExitCode(err error) int {
	var exitError *exec.ExitError

	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	} else {
		return 1
	}
}
