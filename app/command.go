package main

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"syscall"
)

func runCommand(imageId, commandName string, args []string) (int, error) {
	sandboxPath, err := sandbox(imageId)
	if err != nil {
		return 1, err
	}
	defer cleanupSandbox(sandboxPath)

	sandboxRootFsPath := path.Join(sandboxPath, "rootfs")

	command := exec.Command(commandName, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{
		Chroot:     sandboxRootFsPath,
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
