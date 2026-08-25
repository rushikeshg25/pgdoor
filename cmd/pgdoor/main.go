// Command pgdoor is a PostgreSQL connection pooler built from scratch.
//
// Phase 0 (implemented): a transparent byte relay. Point psql at it and every
// feature of Postgres works, because pgdoor is not yet doing anything.
//
//	pgdoor -listen :6432 -upstream localhost:5432
//	psql -h localhost -p 6432 -d postgres
//
// Phase 1 (in progress): -trace decodes and logs every protocol message while
// still forwarding it unchanged. It depends on internal/pgwire, so it starts
// working as soon as that package is implemented.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rushikeshg25/pgdoor/internal/proxy"
)

func main() {
	var (
		listenAddr   = flag.String("listen", ":6432", "address to listen on (pgbouncer's conventional port)")
		upstreamAddr = flag.String("upstream", "localhost:5432", "PostgreSQL server to forward to")
		trace        = flag.Bool("trace", false, "decode and log every protocol message (requires phase 1)")
		dialTimeout  = flag.Duration("dial-timeout", 5*time.Second, "timeout for connecting upstream")
	)
	flag.Parse()

	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		logger.Fatalf("listen %s: %v", *listenAddr, err)
	}
	logger.Printf("pgdoor listening on %s -> %s (trace=%v)", ln.Addr(), *upstreamAddr, *trace)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Unblock Accept on shutdown.
	go func() {
		<-ctx.Done()
		logger.Printf("shutting down, no longer accepting connections")
		_ = ln.Close()
	}()

	var wg sync.WaitGroup
	for {
		client, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			logger.Printf("accept: %v", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle(client, *upstreamAddr, *dialTimeout, *trace, logger)
		}()
	}

	// Let in-flight sessions finish. Phases 4+ will replace this with a real
	// drain that waits for transaction boundaries rather than disconnections.
	wg.Wait()
	logger.Printf("bye")
}

func handle(client net.Conn, upstream string, dialTimeout time.Duration, trace bool, logger *log.Logger) {
	defer client.Close()

	peer := client.RemoteAddr()
	logger.Printf("client connected: %s", peer)
	defer func() { logger.Printf("client disconnected: %s", peer) }()

	// Nagle's algorithm batches small writes, which is exactly wrong for a
	// request/response protocol: it adds latency to every round trip.
	if tc, ok := client.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	server, err := net.DialTimeout("tcp", upstream, dialTimeout)
	if err != nil {
		logger.Printf("dial upstream %s: %v", upstream, err)
		return
	}
	defer server.Close()
	if tc, ok := server.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	if trace {
		prefix := log.New(os.Stderr, "["+peer.String()+"] ", log.LstdFlags|log.Lmicroseconds)
		err = proxy.RelayTraced(client, server, prefix)
	} else {
		err = proxy.Relay(client, server)
	}
	if err != nil {
		logger.Printf("session %s ended with error: %v", peer, err)
	}
}
