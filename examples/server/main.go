// Command server runs coding agents for clients elsewhere, such as
// examples/client: it serves them over TCP, its agents working in -dir.
//
//	go run ./examples/server -listen 127.0.0.1:7788 -token secret
//
// A client controls agents with full access to this computer's files, so a
// server reachable beyond this computer needs a token.
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/quota"
	"github.com/oneo-me/nsgo-agio/wire"
)

func main() {
	home, _ := os.UserHomeDir()
	listen := flag.String("listen", "127.0.0.1:7788", "the TCP `address` to serve on")
	token := flag.String("token", os.Getenv("AGIO_TOKEN"), "the `token` clients must give (default $AGIO_TOKEN)")
	dir := flag.String("dir", home, "the `directory` new conversations work in")
	flag.Parse()
	if *token == "" && !loopback(*listen) {
		log.Fatal("serving beyond this computer needs -token")
	}
	h := host.New(host.Options{Dir: *dir, ReadQuota: quota.Reader(home)})
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
		<-quit
		h.Close()
		os.Exit(0)
	}()
	log.Printf("serving agents in %s on %s", *dir, *listen)
	if err := wire.ListenAndServe(h, *listen, *token); err != nil {
		h.Close()
		log.Fatal(err)
	}
}

// loopback reports an address only this computer reaches.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
