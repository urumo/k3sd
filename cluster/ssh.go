package cluster

import (
	"bytes"
	"fmt"
	"geet.svck.dev/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
)

// ExecuteCommands runs a series of commands on a remote server using an SSH client.
// It creates a new session for each command, executes it, and streams the output to the local stdout and stderr.
//
// Parameters:
//   - client: An established SSH client connection.
//   - commands: A slice of strings, where each string is a command to be executed.
//
// Returns:
//   - error: An error if any command fails to execute, or nil if all commands succeed.
func ExecuteCommands(client *ssh.Client, commands []string) error {
	for _, cmd := range commands {
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session: %v", err)
		}
		defer session.Close()

		//session.Stdout = os.Stdout
		//session.Stderr = os.Stderr
		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr

		utils.Log("Running command: %s\n", cmd)
		if err := session.Run(cmd); err != nil {
			return fmt.Errorf("failed to run command: %v", err)
		}

		utils.Log(stdout.String())
		if stderr.Len() > 0 {
			utils.Log("Error output: %s", stderr.String())
		}
	}
	return nil
}

// ExecuteRemoteScript runs a script on a remote server using an SSH client and returns its output.
// It creates a single session, executes the script, and captures both stdout and stderr.
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

	cmd := fmt.Sprintf("bash -c '%s'", script)
	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("error executing script: %v, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
