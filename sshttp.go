package main

import (
	"flag"
	"fmt"
	"log"
	"io"
	"strings"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/user"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var username, host string
var port int

func init() {
	u, err := user.Current()
	if err != nil {
		log.Fatal("Could not determine current user:", err)
	}
	flag.StringVar(&username, "user", u.Username, "Username on server")
	flag.IntVar(&port, "port", 8123, "Local proxy port")
}

func proxyconn(r io.ReadCloser, w io.Writer) {
	n, err := io.Copy(w, r)
	log.Printf("io.Copy -> %d, %v // closing", n, err)
	_ = r.Close()
}

func main() {
	flag.Parse()
	host = flag.Arg(0)

	if host == "" {
		log.Fatal("No ssh hostname specified.")
	}
	cli := newClient()

	sshTransport := &http.Transport{
		Dial: cli.Dial,
	}

	proxy := httputil.ReverseProxy{
		Director: func(r *http.Request) {
			log.Print(r)
		},
		Transport: sshTransport,
	}

	laddr := fmt.Sprintf("localhost:%d", port)

	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			log.Printf("Connecting to %s", r.URL.Host)
			s, err := cli.Dial("tcp", r.URL.Host)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				w.Write([]byte(err.Error()))
				return
			}
			w.WriteHeader(http.StatusOK)
			c, _, err := w.(http.Hijacker).Hijack()
			log.Printf("client: %t", c)
			log.Printf("server: %t", s)
			go proxyconn(c, s)
			go proxyconn(s, c)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	log.Fatal(http.ListenAndServe(laddr, srv))
}

func newClient() *ssh.Client {
	agentconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal("ssh-agent connection:", err)
	}

	agentauth := ssh.PublicKeysCallback(
		func() ([]ssh.Signer, error) {
			return agent.NewClient(agentconn).Signers()
		})

	conf := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{agentauth},
	}

	if strings.Index(host, ":") < 0 {
		host = host + ":22"
	}

	cli, err := ssh.Dial("tcp", host, conf)
	if err != nil {
		log.Fatal("ssh.Dial:", err)
	}

	return cli
}
