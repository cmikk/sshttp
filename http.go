package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"

	"golang.org/x/crypto/ssh"
)

func proxyconn(r io.ReadCloser, w io.Writer) {
	io.Copy(w, r)
	r.Close()
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

func httpProxy(sshc *ssh.Client, l net.Listener) error {
	proxy := httputil.ReverseProxy{
		Director:  func(r *http.Request) {},
		Transport: &http.Transport{Dial: sshc.Dial},
	}

	return http.Serve(l, connectProxy(sshc, &proxy))
}
