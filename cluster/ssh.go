package cluster

import (
	"bufio"
	"bytes"
	"fmt"
	"geet.svck.dev/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"io"
)

// ExecuteCommands runs a list of commands on a remote server via SSH.
//
// Parameters:
//   - client: An established SSH client connection.
//   - commands: A slice of strings, where each string is a command to be executed.
//
// Returns:
//   - error: An error if any command fails to execute, or nil if all commands succeed.
func ExecuteCommands(client *ssh.Client, commands []string, logger *utils.Logger) error {
	for _, cmd := range commands {
		if err := runCommand(client, cmd, logger); err != nil {
			return err
		}
	}
	return nil
}

// runCommand creates an SSH session, streams the command's output, and executes the command.
//
// Parameters:
//   - client: An established SSH client connection.
//   - cmd: A string representing the command to be executed.
//
// Returns:
//   - error: An error if the command fails to execute, or nil if it succeeds.
func runCommand(client *ssh.Client, cmd string, logger *utils.Logger) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	go streamOutput(stdout, false, logger)
	go streamOutput(stderr, true, logger)

	logger.Log("Running command: %s", cmd)
	return session.Run(cmd)
}

// streamOutput reads from an io.Reader and logs each line of output.
//
// Parameters:
//   - r: The io.Reader to read from (e.g., stdout or stderr).
//   - isErr: A boolean indicating whether the output is from stderr (true) or stdout (false).
func streamOutput(r io.Reader, isErr bool, logger *utils.Logger) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if isErr {
			logger.LogErr("%s", line)
		} else {
			logger.Log("%s", line)
		}
	}
}

// ExecuteRemoteScript runs a script on a remote server via SSH and returns its output.
//
// Parameters:
//   - client: An established SSH client connection.
//   - script: A string containing the script to be executed remotely.
//
// Returns:
//   - string: The standard output of the script execution.
//   - error: An error if the script fails to execute, or nil if it succeeds.
func ExecuteRemoteScript(client *ssh.Client, script string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(fmt.Sprintf("bash -c '%s'", script)); err != nil {
		return "", fmt.Errorf("error executing script: %v, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
