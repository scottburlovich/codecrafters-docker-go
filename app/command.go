package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func runCommand(commandName string, args []string) (int, error) {
	sandboxPath, err := sandbox()
	if err != nil {
		return 1, err
	}
	defer cleanupSandbox(sandboxPath)

	command := exec.Command(commandName, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{
		Chroot:     sandboxPath,
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNET,
	}

	err = command.Start()
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
