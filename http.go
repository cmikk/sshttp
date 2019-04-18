package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

func proxyconn(r io.ReadCloser, w io.Writer) {
	io.Copy(w, r)
	r.Close()
}

func connectHandler(sshc *ssh.Client, dh http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			dh.ServeHTTP(w, r)
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

func jsonHandler(v interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.Encode(v)
	}
}

func httpProxy(sshc *ssh.Client, l net.Listener) error {
	proxy := httputil.ReverseProxy{
		Director:  func(r *http.Request) {},
		Transport: &http.Transport{Dial: sshc.Dial},
	}
	mux := http.NewServeMux()

	mux.Handle("/", &proxy)
	mux.Handle("/config", jsonHandler(&proxyConfig{
		ProxyAddr: l.Addr().String(),
		ProxyPid:  os.Getpid(),
	}))

	// connectHandler must be spliced in here because the CONNECT
	// method URI has no "/" which means mux will not see it.
	return http.Serve(l, connectHandler(sshc, mux))
}

func queryConfig(port int) (pc proxyConfig, err error) {
	var resp *http.Response
	cli := &http.Client{}

	resp, err = cli.Get(fmt.Sprintf("http://localhost:%d/config", port))
	if err != nil {
		return
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("http: %s", resp.Status)
		return
	}

	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&pc)
	return
}
