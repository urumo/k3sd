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

func sshConnect(userName, password, host string) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("could not get current user: %w", err)
	}
	sshDir := filepath.Join(usr.HomeDir, ".ssh")

	_ = filepath.WalkDir(sshDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".pub") {
			return nil
		}
		keyBytes, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
		return nil
	})

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
func ExecuteCommands(client *ssh.Client, commands []string, logger *utils.Logger) error {
	for _, cmd := range commands {
		if err := runCommand(client, cmd, logger); err != nil {
			return err
		}
	}
	return nil
}
func runCommand(client *ssh.Client, cmd string, logger *utils.Logger) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer func(session *ssh.Session) {
		err := session.Close()
		if err != nil && err.Error() != "EOF" {
			logger.LogErr("Error closing SSH session: %v\n", err)
		}
	}(session)

	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	go streamOutput(stdout, false, logger)
	go streamOutput(stderr, true, logger)

	logger.LogCmd(cmd)
	return session.Run(cmd)
}
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
func ExecuteRemoteScript(client *ssh.Client, script string, logger *utils.Logger) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer func(session *ssh.Session) {
		err := session.Close()
		if err != nil && err.Error() != "EOF" {
			logger.LogErr("Error closing SSH session: %v\n", err)
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
