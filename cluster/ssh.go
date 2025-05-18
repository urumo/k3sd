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
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session: %v", err)
		}
		defer session.Close()

		stdoutPipe, err := session.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to get stdout pipe: %v", err)
		}

		stderrPipe, err := session.StderrPipe()
		if err != nil {
			return fmt.Errorf("failed to get stderr pipe: %v", err)
		}

		go streamOutput(stdoutPipe, false)
		go streamOutput(stderrPipe, true)

		utils.Log("Running command: %s", cmd)

		if err := session.Run(cmd); err != nil {
			return fmt.Errorf("command failed: %v", err)
		}
	}
	return nil
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

	cmd := fmt.Sprintf("bash -c '%s'", script)
	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("error executing script: %v, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
