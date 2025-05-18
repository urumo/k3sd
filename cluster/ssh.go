package cluster

import (
	"bytes"
	"fmt"
	"geet.svck.dev/urumo/k3sd/utils"
	"golang.org/x/crypto/ssh"
	"os"
)

func ExecuteCommands(client *ssh.Client, commands []string) error {
	for _, cmd := range commands {
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session: %v", err)
		}
		defer session.Close()

		session.Stdout = os.Stdout
		session.Stderr = os.Stderr

		utils.Log("Running command: %s\n", cmd)
		if err := session.Run(cmd); err != nil {
			return fmt.Errorf("failed to run command: %v", err)
		}
	}
	return nil
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
