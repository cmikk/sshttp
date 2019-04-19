package main

import (
	"flag"
	"fmt"
	"io"
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

func runWithConfig(pc proxyConfig, command []string) {
	os.Setenv("http_proxy", pc.ProxyAddr)
	os.Setenv("https_proxy", pc.ProxyAddr)
	os.Setenv("PROXY_PID", strconv.FormatInt(int64(pc.ProxyPid), 10))
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			log.Println("command: ", err)
		}
		os.Exit(1)
	}
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
		log.Fatal("Could not determine current user: ", err.Error())
	}

	username := flag.String("user", u.Username, "Username on server")
	port := flag.Int("port", 8123, "Local proxy port")
	kill := flag.Bool("kill", false, "Kill running sshttp")
	query := flag.Bool("query", false, "Use existing sshttp proxy")

	flag.Parse()

	pc := proxyConfig{
		ProxyAddr: fmt.Sprintf("localhost:%d", *port),
		ProxyPid:  os.Getpid(),
	}

	epid, err := strconv.Atoi(os.Getenv("SSHTTP_PID"))
	if err == nil {
		pc.ProxyPid = epid
	}

	if *query {
		pc, err = queryConfig(*port)
		switch {
		case err != nil:
			log.Fatal("query: ", err)
		case *kill:
			break
		case flag.NArg() == 0:
			printConfig(pc)
			return
		default:
			runWithConfig(pc, flag.Args())
			return
		}
	}

	if *kill {
		if pc.ProxyPid == os.Getpid() {
			log.Fatal("kill: unable to determine proxy pid")
		}

		if err = syscall.Kill(pc.ProxyPid, syscall.SIGKILL); err != nil {
			log.Fatal("Kill failed: ", err.Error())
		}
		return
	}

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	hostname := flag.Arg(0)
	command := flag.Args()[1:]

	if strings.Index(hostname, "@") > 0 {
		l := strings.Split(hostname, "@")
		*username = l[0]
		hostname = l[1]
	}

	// When starting a proxy process to run in the background, we use
	// an environment variable set by the parent process to distinguish
	// the child invocation from the parent.
	//
	// In C, we could use the differing return values from fork() for
	// this purpose, but cleanly running a detached background clone of
	// ourselves in go requires re-executing ourselves, which hides the
	// fork() result.
	bgProxy := (os.Getenv(fmt.Sprintf("SSHTTP_%d", os.Getppid())) != "")

	if !bgProxy && len(command) == 0 {
		var outp io.ReadCloser
		var b []byte

		// We are not a background proxy process, and have no command
		// to run, so we start up a background proxy process and print
		// a proxy config.
		os.Setenv(fmt.Sprintf("SSHTTP_%d", os.Getpid()), "1")

		cmd := exec.Command(os.Args[0],
			"-port", strconv.FormatInt(int64(*port), 10),
			"-user", *username,
			hostname)
		outp, err = cmd.StdoutPipe()
		if err != nil {
			log.Fatal(err)
		}
		cmd.Stderr = os.Stderr
		cmd.Start()
		b, err = ioutil.ReadAll(outp)
		if err != nil {
			log.Fatal(err)
		}
		if len(b) == 0 {
			os.Exit(1)
		}
		pc.ProxyPid = cmd.Process.Pid
		printConfig(pc)
		return
	}

	// We are running a command or a background proxy version of
	// ourselves. In either case, we need to set up the ssh client
	// connection and HTTP listener.
	sshc, err := sshClient(*username, hostname)
	if err != nil {
		log.Fatal(err)
	}

	l, err := net.Listen("tcp", pc.ProxyAddr)
	if err != nil {
		log.Fatal("http: ", err)
	}

	if bgProxy {
		// If we are running the background proxy version of
		// ourselves, we inform our parent of our successful
		// startup by printing the string "OK" to Stdout, then
		// start the proxy process, and exit when that finishes
		// or fails..
		os.Stdout.Write([]byte("OK\n"))
		os.Stdout.Close()
		err = httpProxy(sshc, l)
		if err != nil {
			log.Fatal("proxy: ", err)
		}
		return
	}

	// Otherwise, if we are running the command directly,
	// we start the proxy process in a separate goroutine,
	// then run the command with the proper environment.
	go func() {
		log.Fatal("proxy: ", httpProxy(sshc, l))
	}()
	runWithConfig(pc, command)
}
