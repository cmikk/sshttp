package main

import (
	"flag"
	"fmt"
	"log"
	"os/user"
	"strings"
)

func main() {
	u, err := user.Current()
	if err != nil {
		log.Fatal("Could not determine current user:", err.Error())
	}
	username := flag.String("user", u.Username, "Username on server")
	port := flag.Int("port", 8123, "Local proxy port")
	flag.Parse()
	hostname := flag.Arg(0)

	if hostname == "" {
		log.Fatal("No ssh hostname specified.")
	}

	if strings.Index(hostname, "@") > 0 {
		l := strings.Split(hostname, "@")
		*username = l[0]
		hostname = l[1]
	}

	proxyAddr := fmt.Sprintf("localhost:%d", *port)

	sshc, err := sshClient(*username, hostname)
	if err != nil {
		log.Fatal("Could not establish ssh connection:", err.Error())
	}
	log.Fatal(httpProxy(sshc, proxyAddr).Error())
}
