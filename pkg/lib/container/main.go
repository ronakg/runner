package main

import (
	"os"
	"os/exec"
)

func main() {
	// TODO: cgroups set up for os.Getpid()

	// Run the command under a shell so that we can support pipes, input/output redirection, command
	// chains
	cmd := exec.Command("sh", []string{"-c", os.Args[1]}...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Exit(cmd.ProcessState.ExitCode())
	}
}
