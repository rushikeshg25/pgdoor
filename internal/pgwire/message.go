// Package pgwire implements the PostgreSQL v3 frontend/backend wire protocol.
//
// Everything here is written against the protocol specification directly, with
// no third-party dependencies. It is the foundation the rest of pgdoor sits on:
// the proxy, the pooler, the cancel registry and the prepared-statement cache
// all speak in terms of the types defined in this package.
//
// # Framing
//
// Every message after the handshake has the same shape:
//
//	┌────────┬────────────────────┬──────────────────────┐
//	│ tag    │ length (int32, BE) │ body                 │
//	│ 1 byte │ INCLUDES itself,   │ length - 4 bytes     │
//	│        │ EXCLUDES the tag   │                      │
//	└────────┴────────────────────┴──────────────────────┘
//
// The startup packet is the one exception: it has no tag byte. See startup.go.
package pgwire

import (
	"bufio"
	"io"
)

// Protocol version and special request codes that may appear in place of a
// version number in the first int32 of a startup packet.
const (
	// ProtocolVersion3 is 3.0 encoded as (major << 16) | minor.
	ProtocolVersion3 = 196608

	// SSLRequestCode asks the server to begin TLS. The server replies with a
	// single byte, 'S' (proceed) or 'N' (refuse) -- NOT a framed message.
	SSLRequestCode = 80877103

	// CancelRequestCode arrives on its own brand-new TCP connection and carries
	// a backend process ID and secret key. See docs and the cancel package.
	CancelRequestCode = 80877102

	// GSSEncRequestCode asks for GSSAPI encryption. pgdoor replies 'N'.
	GSSEncRequestCode = 80877104
)

// MaxMessageSize bounds how large a single message body may be before the
// reader rejects it. Without this a hostile or corrupt peer can convince the
// proxy to allocate an arbitrary amount of memory by sending a large length.
const MaxMessageSize = 1 << 24 // 16 MiB

// Direction identifies which end of the connection produced a message. It is
// required to decode ambiguous tags -- see tags.go.
type Direction int

const (
	// FromFrontend marks messages sent by a client toward the server.
	FromFrontend Direction = iota
	// FromBackend marks messages sent by the server toward a client.
	FromBackend
)

// String renders the direction as "C->S" or "S->C" for trace output.
//
// TODO(phase-1): implement.
func (d Direction) String() string {
	panic("pgwire: Direction.String not implemented (phase 1)")
}

// RawMessage is one framed protocol message: its tag and its body. The body
// excludes the tag and the length prefix.
//
// RawMessage deliberately does not interpret the body. The proxy forwards the
// overwhelming majority of messages untouched, so paying to parse every DataRow
// would be waste; only messages pgdoor must reason about get decoded further.
type RawMessage struct {
	Tag  byte
	Body []byte
}

// WireLen reports how many bytes this message occupies on the wire, including
// the tag byte and the length prefix.
//
// TODO(phase-1): implement.
func (m *RawMessage) WireLen() int {
	panic("pgwire: RawMessage.WireLen not implemented (phase 1)")
}

// AppendTo encodes m onto dst and returns the extended slice. Using an append
// style lets callers reuse a scratch buffer across messages instead of
// allocating per message on a hot relay path.
//
// TODO(phase-1): implement.
func (m *RawMessage) AppendTo(dst []byte) []byte {
	panic("pgwire: RawMessage.AppendTo not implemented (phase 1)")
}

// Describe renders a one-line, human-readable summary of the message for the
// -trace logger, e.g.:
//
//	Query          "SELECT 1"
//	ReadyForQuery  I
//	DataRow        2 columns
//
// It must be robust against malformed bodies: a trace helper that panics on a
// short body will take down the proxy on exactly the traffic you most wanted to
// look at. Truncate long payloads rather than dumping megabytes.
//
// TODO(phase-1): implement.
func (m *RawMessage) Describe(d Direction) string {
	panic("pgwire: RawMessage.Describe not implemented (phase 1)")
}

// Reader reads framed protocol messages from a connection.
//
// A Reader is not safe for concurrent use. In pgdoor each connection has
// exactly one reading goroutine, which is what makes that acceptable.
type Reader struct {
	br *bufio.Reader
}

// NewReader wraps r. The caller keeps ownership of r.
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReaderSize(r, 32*1024)}
}

// ReadMessage reads the next framed message.
//
// The returned RawMessage's Body is only valid until the next call to
// ReadMessage if you choose to reuse an internal buffer -- document whichever
// ownership rule you pick and hold to it, because a subtle aliasing bug here
// will surface as corrupted query text three phases from now.
//
// It must reject:
//   - a length field below 4 (the length includes itself)
//   - a length implying a body larger than MaxMessageSize
//   - a short read mid-body (return io.ErrUnexpectedEOF, not a partial message)
//
// TODO(phase-1): implement.
func (r *Reader) ReadMessage() (*RawMessage, error) {
	panic("pgwire: Reader.ReadMessage not implemented (phase 1)")
}

// Writer writes framed protocol messages to a connection.
//
// Writer buffers. Nothing reaches the peer until Flush is called, which matters
// enormously for a proxy: forgetting to flush before blocking on a read is the
// classic way to deadlock both sides of a relay.
type Writer struct {
	bw *bufio.Writer
}

// NewWriter wraps w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{bw: bufio.NewWriterSize(w, 32*1024)}
}

// WriteMessage encodes and buffers m. Call Flush to push it to the peer.
//
// TODO(phase-1): implement.
func (w *Writer) WriteMessage(m *RawMessage) error {
	panic("pgwire: Writer.WriteMessage not implemented (phase 1)")
}

// Flush pushes buffered bytes to the underlying writer.
func (w *Writer) Flush() error { return w.bw.Flush() }
