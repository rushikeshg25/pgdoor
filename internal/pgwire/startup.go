package pgwire

// The startup phase is the part of the protocol that does not look like the
// rest of the protocol. Three things make it special:
//
//  1. The first packet from a client has NO tag byte -- just length + payload.
//  2. The first int32 of that payload is not always a version. It may be one of
//     the magic request codes (SSL, Cancel, GSSEnc), each of which means the
//     connection is not a normal session at all.
//  3. The reply to SSLRequest is a single raw byte, not a framed message.
//
// A client may legitimately send SSLRequest, get 'S', complete a TLS handshake,
// and only THEN send a real startup packet. So "read a startup packet" is a
// loop, not a single call.

// Startup is a parsed protocol-3.0 startup packet.
type Startup struct {
	// ProtocolVersion is normally ProtocolVersion3. A client may request a
	// higher minor version; the correct response to an unsupported minor
	// version is NegotiateProtocolVersion ('v'), not an error.
	ProtocolVersion uint32

	// Parameters holds the null-terminated key/value pairs that follow the
	// version. "user" is mandatory. "database" defaults to the user name when
	// absent -- a detail that matters, because pgdoor keys its pools on
	// (user, database) and must resolve that default the same way the server
	// would or it will open a second, redundant pool.
	Parameters map[string]string
}

// User returns the "user" parameter.
//
// TODO(phase-1): implement.
func (s *Startup) User() string {
	panic("pgwire: Startup.User not implemented (phase 1)")
}

// Database returns the "database" parameter, falling back to the user name when
// it is absent, matching the server's own default.
//
// TODO(phase-1): implement.
func (s *Startup) Database() string {
	panic("pgwire: Startup.Database not implemented (phase 1)")
}

// CancelRequest is a parsed cancellation packet. It arrives on a fresh TCP
// connection that will carry nothing else and then close.
//
// SECURITY: in transaction pooling, forwarding the server's real ProcessID and
// SecretKey to clients means a client's Ctrl-C cancels whatever query happens to
// be running on that server connection right now -- which may belong to another
// client entirely. pgdoor must hand clients synthetic keys and translate. See
// phase 5.
type CancelRequest struct {
	ProcessID uint32
	SecretKey uint32
}

// StartupKind classifies what a client's first packet turned out to be.
type StartupKind int

const (
	// KindStartup is a normal protocol-3.0 session start.
	KindStartup StartupKind = iota
	// KindSSLRequest asks to negotiate TLS before starting up.
	KindSSLRequest
	// KindGSSEncRequest asks to negotiate GSSAPI encryption.
	KindGSSEncRequest
	// KindCancelRequest is an out-of-band query cancellation.
	KindCancelRequest
)

// FirstPacket is the discriminated result of reading a client's first packet.
// Exactly one of Startup or Cancel is non-nil, per Kind.
type FirstPacket struct {
	Kind    StartupKind
	Startup *Startup
	Cancel  *CancelRequest

	// Raw is the exact bytes of the packet as they arrived, length prefix
	// included. Phase 1 forwards these verbatim, which lets the trace proxy
	// observe the handshake before pgdoor is able to re-encode one itself.
	Raw []byte
}

// ReadFirstPacket reads and classifies the untagged first packet on a
// connection.
//
// It must reject a length that is absurd or below 8, and must not read past the
// declared length -- over-reading here silently eats the first bytes of the
// following TLS ClientHello, which produces a spectacularly confusing failure.
//
// TODO(phase-1): implement.
func (r *Reader) ReadFirstPacket() (*FirstPacket, error) {
	panic("pgwire: Reader.ReadFirstPacket not implemented (phase 1)")
}

// AppendStartup encodes s as an untagged startup packet onto dst. pgdoor needs
// this because it originates its own connections to Postgres as a client.
//
// TODO(phase-2): implement.
func AppendStartup(dst []byte, s *Startup) []byte {
	panic("pgwire: AppendStartup not implemented (phase 2)")
}

// AppendCancelRequest encodes a cancellation packet onto dst.
//
// TODO(phase-5): implement.
func AppendCancelRequest(dst []byte, c *CancelRequest) []byte {
	panic("pgwire: AppendCancelRequest not implemented (phase 5)")
}
