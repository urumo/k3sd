package cluster

import (
	"bufio"
	"bytes"
	"fmt"
	"geet.svck.dev/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"io"
)

func ExecuteCommands(client *ssh.Client, commands []string) error {
	for _, cmd := range commands {
		if err := runCommand(client, cmd); err != nil {
			return err
		}
	}
	return nil
}

func runCommand(client *ssh.Client, cmd string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	go streamOutput(stdout, false)
	go streamOutput(stderr, true)

	utils.Log("Running command: %s", cmd)
	return session.Run(cmd)
}

func streamOutput(r io.Reader, isErr bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if isErr {
			utils.Log("[error] %s", line)
		} else {
			utils.Log("%s", line)
		}
	}
}

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
