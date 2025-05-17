package cluster

import (
	"bytes"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"path/filepath"
)

func ScpFile(client *ssh.Client, localFilePath, remoteFilePath string) error {
	localFile, err := os.Open(localFilePath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %v", err)
	}
	defer localFile.Close()

	fileInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()

		fmt.Fprintf(w, "C0644 %d %s\n", fileInfo.Size(), filepath.Base(remoteFilePath))
		io.Copy(w, localFile)
		fmt.Fprint(w, "\x00")
	}()

	if err := session.Run(fmt.Sprintf("scp -t %s", filepath.Dir(remoteFilePath))); err != nil {
		return fmt.Errorf("failed to run scp command: %v", err)
	}

	return nil
}

func ExecuteCommands(client *ssh.Client, commands []string) error {
	for _, cmd := range commands {
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session: %v", err)
		}
		defer session.Close()

		session.Stdout = os.Stdout
		session.Stderr = os.Stderr

		fmt.Printf("Running command: %s\n", cmd)
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
