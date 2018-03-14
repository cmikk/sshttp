package main

import (
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func sshClient(username, host string) (*ssh.Client, error) {
	agentconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, err
	}

	aclient := agent.NewClient(agentconn)
	callback, err := knownhosts.New(os.Getenv("HOME") + "/.ssh/known_hosts")
	if err != nil {
		callback = ssh.InsecureIgnoreHostKey()
	}

	conf := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(aclient.Signers),
		},
		HostKeyCallback: callback,
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
