package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

type proxyConfig struct {
	ProxyAddr string
	ProxyPid  int
}

func main() {
	u, err := user.Current()
	if err != nil {
		log.Fatal("Could not determine current user:", err.Error())
	}

	username := flag.String("user", u.Username, "Username on server")
	port := flag.Int("port", 8123, "Local proxy port")
	foreground := flag.Bool("foreground", false, "Run in foreground")
	kill := flag.Bool("kill", false, "Kill running sshttp")

	flag.Parse()

	if *kill {
		pid, err := strconv.Atoi(os.Getenv("SSHTTP_PID"))
		if err != nil {
			log.Fatal("Invalid or missing SSHTTP_PID:", err.Error())
		}
		err = syscall.Kill(pid, syscall.SIGKILL)
		if err != nil {
			log.Fatal("Kill failed:", err.Error())
		}
		return
	}

	hostname := flag.Arg(0)

	if hostname == "" {
		log.Fatal("No ssh hostname specified.")
	}

	command := flag.Args()[1:]

	proxyAddr := fmt.Sprintf("localhost:%d", *port)

	if strings.Index(hostname, "@") > 0 {
		l := strings.Split(hostname, "@")
		*username = l[0]
		hostname = l[1]
	}

	if len(command) > 0 || *foreground {
		sshc, err := sshClient(*username, hostname)
		if err != nil {
			log.Fatal(err)
		}

		l, err := net.Listen("tcp", proxyAddr)
		if err != nil {
			log.Fatal("http: ", err)
		}

		if *foreground {
			//
			// Write a zero-width space to stdout as a signal to our
			// caller that we have successfully set up the ssh connection
			// and http proxy listener. If called manually with -foreground,
			// this space is invisible to the user.
			//
			os.Stdout.Write([]byte("\uFEFF"))
			os.Stdout.Close()
			err := httpProxy(sshc, l)
			if err != nil {
				log.Fatal("proxy: ", err)
			}
			return
		}

		go func() {
			log.Fatal("proxy: ", httpProxy(sshc, l))
		}()
		os.Setenv("http_proxy", proxyAddr)
		os.Setenv("https_proxy", proxyAddr)
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		cmd.Run()
	}

	args := append([]string{"-foreground"}, os.Args[1:]...)
	cmd := exec.Command(os.Args[0], args...)
	outp, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	cmd.Start()
	b, err := ioutil.ReadAll(outp)
	if err != nil {
		log.Fatal(err)
	}
	if len(b) == 0 {
		os.Exit(1)
	}
	pid := cmd.Process.Pid
	if strings.HasSuffix(os.Getenv("SHELL"), "csh") {
		fmt.Printf("setenv http_proxy %s;\n", proxyAddr)
		fmt.Printf("setenv https_proxy %s;\n", proxyAddr)
		fmt.Printf("setenv SSHTTP_PID %d;\n", pid)
	} else {
		fmt.Printf("http_proxy=%s; export http_proxy;\n", proxyAddr)
		fmt.Printf("https_proxy=%s; export https_proxy;\n", proxyAddr)
		fmt.Printf("SSHTTP_PID=%d; export SSHTTP_PID;\n", pid)
	}
	fmt.Printf("echo sshttp running, pid %d;\n", pid)
}
