// Package proxy carries a client connection through to a Postgres server.
//
// Phase 0 is a byte relay: it proves the plumbing, half-close handling and
// shutdown path work before any protocol knowledge is involved. Phase 1 adds a
// message-aware relay that decodes and logs every message while still
// forwarding it unchanged -- that trace mode becomes the debugger for every
// later phase, so it is worth more than it looks.
//
// Later phases replace Relay entirely with a ClientSession that terminates auth
// and borrows server connections from a pool.
package proxy

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"

	"github.com/rushikeshg25/pgdoor/internal/pgwire"
)

// halfCloser is implemented by *net.TCPConn. Closing only the write side lets
// the peer observe EOF and drain its side cleanly, which is what makes a client
// disconnect look like a clean "Terminate" to Postgres rather than a reset.
type halfCloser interface {
	CloseWrite() error
}

// Relay copies bytes between client and server until either side closes. It
// understands nothing about the protocol.
//
// It returns the first non-EOF error observed in either direction.
func Relay(client, server net.Conn) error {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	record := func(err error) {
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	pipe := func(dst, src net.Conn) {
		defer wg.Done()
		_, err := io.Copy(dst, src)
		record(err)
		// Signal EOF downstream without tearing down the other direction.
		if hc, ok := dst.(halfCloser); ok {
			_ = hc.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}

	wg.Add(2)
	go pipe(server, client)
	go pipe(client, server)
	wg.Wait()

	return firstErr
}

// RelayTraced forwards messages in both directions while logging a decoded
// summary of each one. Byte-for-byte, what it forwards is identical to what
// Relay would have forwarded; only the observation is added.
//
// It depends on the pgwire package, so it starts working the moment phase 1 is
// implemented and panics with a clear "not implemented" until then.
func RelayTraced(client, server net.Conn, logger *log.Logger) error {
	// The first packet is untagged and may not be a session start at all.
	cr := pgwire.NewReader(client)
	first, err := cr.ReadFirstPacket()
	if err != nil {
		return err
	}

	switch first.Kind {
	case pgwire.KindStartup:
		logger.Printf("C->S  Startup        user=%q database=%q version=%d",
			first.Startup.User(), first.Startup.Database(), first.Startup.ProtocolVersion)
	case pgwire.KindCancelRequest:
		logger.Printf("C->S  CancelRequest  pid=%d secret=%#x",
			first.Cancel.ProcessID, first.Cancel.SecretKey)
	case pgwire.KindSSLRequest:
		logger.Printf("C->S  SSLRequest")
	case pgwire.KindGSSEncRequest:
		logger.Printf("C->S  GSSEncRequest")
	}

	if _, err := server.Write(first.Raw); err != nil {
		return err
	}

	// A cancellation connection carries nothing else; let the server see EOF.
	if first.Kind == pgwire.KindCancelRequest {
		if hc, ok := server.(halfCloser); ok {
			_ = hc.CloseWrite()
		}
		return nil
	}

	// Once TLS is negotiated the traffic is opaque to us until phase 8
	// terminates TLS properly. Fall back to a byte relay and say so.
	if first.Kind == pgwire.KindSSLRequest || first.Kind == pgwire.KindGSSEncRequest {
		logger.Printf("      (encrypted from here -- falling back to raw relay; see phase 8)")
		return Relay(client, server)
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	record := func(err error) {
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	forward := func(r *pgwire.Reader, dst net.Conn, dir pgwire.Direction) {
		defer wg.Done()
		w := pgwire.NewWriter(dst)
		for {
			m, err := r.ReadMessage()
			if err != nil {
				record(err)
				break
			}
			logger.Printf("%s  %s", dir, m.Describe(dir))
			if err := w.WriteMessage(m); err != nil {
				record(err)
				break
			}
			// Flush eagerly. A proxy that buffers a message and then blocks on
			// its next read deadlocks both peers, and the symptom -- "psql
			// hangs, sometimes" -- is miserable to chase down.
			if err := w.Flush(); err != nil {
				record(err)
				break
			}
		}
		if hc, ok := dst.(halfCloser); ok {
			_ = hc.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}

	wg.Add(2)
	go forward(cr, server, pgwire.FromFrontend)
	go forward(pgwire.NewReader(server), client, pgwire.FromBackend)
	wg.Wait()

	return firstErr
}
