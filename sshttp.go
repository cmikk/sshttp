package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/user"
	"strings"

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

func sshClient() *ssh.Client {
	agentconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal("ssh-agent connection:", err)
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
		log.Fatal("ssh.Dial:", err)
	}

	return cli
}

func proxyconn(r io.ReadCloser, w io.Writer) {
	_, _ = io.Copy(w, r)
	_ = r.Close()
}

func connectProxy(sshc *ssh.Client, h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			h.ServeHTTP(w, r)
			return
		}
		host := r.URL.Host
		if strings.Index(host, ":") < 0 {
			host += ":80"
		}
		sconn, err := sshc.Dial("tcp", host)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(err.Error()))
			return
		}

		w.WriteHeader(http.StatusOK)
		cconn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			cconn.Close()
			sconn.Close()
			log.Print("CONNECT hijack error: ", err)
			return
		}
		go proxyconn(cconn, sconn)
		go proxyconn(sconn, cconn)
	}
}

func main() {
	flag.Parse()
	host = flag.Arg(0)

	if host == "" {
		log.Fatal("No ssh hostname specified.")
	}
	sshc := sshClient()

	proxy := httputil.ReverseProxy{
		Director:  func(r *http.Request) {},
		Transport: &http.Transport{Dial: sshc.Dial},
	}

	laddr := fmt.Sprintf("localhost:%d", port)

	log.Fatal(http.ListenAndServe(laddr, connectProxy(sshc, &proxy)))
}
