package cluster

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/argon-chat/k3sd/utils"
	"golang.org/x/crypto/ssh"
)

// sshConnect establishes an SSH connection to a remote host.
//
// Parameters:
// - user: The username for the SSH connection.
// - pass: The password for the SSH connection.
// - host: The address of the remote host.
//
// Returns:
// - A pointer to an ssh.Client instance.
// - An error if the connection fails.
func sshConnect(userName, password, host string) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("could not get current user: %w", err)
	}
	sshDir := filepath.Join(usr.HomeDir, ".ssh")

	err = filepath.WalkDir(sshDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		if strings.HasSuffix(d.Name(), ".pub") {
			return nil
		}

		if _, err := os.Stat(path + ".pub"); err == nil {
			keyBytes, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			signer, err := ssh.ParsePrivateKey(keyBytes)
			if err != nil {
				return nil
			}
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed loading SSH keys: %w", err)
	}

	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no usable SSH authentication methods found")
	}

	cfg := &ssh.ClientConfig{
		User:            userName,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return ssh.Dial("tcp", host+":22", cfg)
}

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
	defer func(session *ssh.Session) {
		err := session.Close()
		if err != nil {
			logger.LogErr("Error closing SSH session: %v\n", err)
		} else {
			logger.Log("SSH session closed successfully.\n")
		}
	}(session)

	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	go streamOutput(stdout, false, logger)
	go streamOutput(stderr, true, logger)

	logger.LogCmd(cmd)
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
func ExecuteRemoteScript(client *ssh.Client, script string, logger *utils.Logger) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer func(session *ssh.Session) {
		err := session.Close()
		if err != nil {
			logger.LogErr("Error closing SSH session: %v\n", err)
		} else {
			logger.Log("SSH session closed successfully.\n")
		}
	}(session)

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	command := fmt.Sprintf("bash -c '%s'", script)
	logger.LogCmd(command)
	if err := session.Run(command); err != nil {
		return "", fmt.Errorf("error executing script: %v, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
