package main

import (
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func sshClient(username, host string) (*ssh.Client, error) {
	agentconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, err
	}

	aclient := agent.NewClient(agentconn)

	conf := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(aclient.Signers),
		},
	}

	if strings.Index(host, ":") < 0 {
		host = host + ":22"
	}

	cli, err := ssh.Dial("tcp", host, conf)
	if err != nil {
		return nil, err
	}

	return cli, nil
}
