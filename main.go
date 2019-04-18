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

func printConfig(pc proxyConfig) {
	if strings.HasSuffix(os.Getenv("SHELL"), "csh") {
		fmt.Printf("setenv http_proxy %s;\n", pc.ProxyAddr)
		fmt.Printf("setenv https_proxy %s;\n", pc.ProxyAddr)
		fmt.Printf("setenv SSHTTP_PID %d;\n", pc.ProxyPid)
	} else {
		fmt.Printf("http_proxy=%s; export http_proxy;\n", pc.ProxyAddr)
		fmt.Printf("https_proxy=%s; export https_proxy;\n", pc.ProxyAddr)
		fmt.Printf("SSHTTP_PID=%d; export SSHTTP_PID;\n", pc.ProxyPid)
	}
	fmt.Printf("echo sshttp running, pid %d;\n", pc.ProxyPid)
}

func runWithConfig(pc proxyConfig, command []string) error {
	os.Setenv("http_proxy", pc.ProxyAddr)
	os.Setenv("https_proxy", pc.ProxyAddr)
	os.Setenv("PROXY_PID", strconv.FormatInt(int64(pc.ProxyPid), 10))
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [options] [user@]hostname [command ...]\n"+
				"       %s -query [options] [command ...]\n"+
				"       %s -kill [-query]\n",
			os.Args[0], os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}
	u, err := user.Current()
	if err != nil {
		log.Fatal("Could not determine current user:", err.Error())
	}

	username := flag.String("user", u.Username, "Username on server")
	port := flag.Int("port", 8123, "Local proxy port")
	foreground := flag.Bool("foreground", false, "Run in foreground")
	kill := flag.Bool("kill", false, "Kill running sshttp")
	query := flag.Bool("query", false, "Use existing sshttp proxy")

	flag.Parse()

	if *query {
		pc, err := queryConfig(*port)
		if err != nil {
			log.Fatal(err)
		}

		if *kill {
			if err := syscall.Kill(pc.ProxyPid, syscall.SIGKILL); err != nil {
				log.Fatal("Kill failed:", err.Error())
			}
			return
		}

		command := flag.Args()
		if len(command) > 0 {
			if err := runWithConfig(pc, command); err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					log.Println("command:", err)
				}
				os.Exit(1)
			}
			return
		}

		printConfig(pc)
		return
	}

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

	pc := proxyConfig{
		ProxyAddr: fmt.Sprintf("localhost:%d", *port),
		ProxyPid:  os.Getpid(),
	}

	hostname := flag.Arg(0)

	if hostname == "" {
		log.Fatal("No ssh hostname specified.")
	}

	command := flag.Args()[1:]

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

		l, err := net.Listen("tcp", pc.ProxyAddr)
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
		if err := runWithConfig(pc, command); err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				log.Println("command: ", err)
			}
			os.Exit(1)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-foreground",
		"-port", strconv.FormatInt(int64(*port), 10),
		"-user", *username,
		hostname)
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
	pc.ProxyPid = cmd.Process.Pid
	printConfig(pc)
}
