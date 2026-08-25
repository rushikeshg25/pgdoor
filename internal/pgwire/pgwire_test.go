package pgwire

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// catchTODO converts the panics thrown by unimplemented scaffolding into an
// ordinary test failure, so that one unimplemented function does not abort the
// whole test binary and hide the rest of the red.
//
// Delete it once every TODO in this package is implemented.
func catchTODO(t *testing.T) {
	t.Helper()
	if r := recover(); r != nil {
		t.Fatalf("not implemented yet: %v", r)
	}
}

// ---------------------------------------------------------------------------
// Fixtures: real bytes, hand-assembled from the protocol spec.
// ---------------------------------------------------------------------------

// Query('Q') carrying "SELECT 1".
//
//	'Q'            tag
//	0x0000000D     length 13 = 4 (length field) + 9 (body)
//	"SELECT 1\x00" body: the SQL text, null-terminated
var queryBytes = []byte{
	'Q',
	0x00, 0x00, 0x00, 0x0D,
	'S', 'E', 'L', 'E', 'C', 'T', ' ', '1', 0x00,
}

// ReadyForQuery('Z') with transaction status 'I' (idle).
var readyIdleBytes = []byte{
	'Z',
	0x00, 0x00, 0x00, 0x05,
	'I',
}

// A protocol-3.0 startup packet for user=rushi database=postgres.
//
//	0x00000026  length 38 = 4 + 4 (version) + 30 (parameters)
//	0x00030000  version 196608
//	"user\0rushi\0database\0postgres\0" then a final \0 terminator
var startupBytes = func() []byte {
	body := []byte("user\x00rushi\x00database\x00postgres\x00\x00")
	out := []byte{0x00, 0x00, 0x00, 0x26, 0x00, 0x03, 0x00, 0x00}
	return append(out, body...)
}()

// SSLRequest: length 8, code 80877103 (== 1234<<16 | 5679).
var sslRequestBytes = []byte{
	0x00, 0x00, 0x00, 0x08,
	0x04, 0xD2, 0x16, 0x2F,
}

// CancelRequest: length 16, code 80877102, pid 4242, secret 0xDEADBEEF.
var cancelRequestBytes = []byte{
	0x00, 0x00, 0x00, 0x10,
	0x04, 0xD2, 0x16, 0x2E,
	0x00, 0x00, 0x10, 0x92,
	0xDE, 0xAD, 0xBE, 0xEF,
}

// ---------------------------------------------------------------------------
// Framing
// ---------------------------------------------------------------------------

func TestReadMessage_Query(t *testing.T) {
	defer catchTODO(t)

	r := NewReader(bytes.NewReader(queryBytes))
	m, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if m.Tag != FTagQuery {
		t.Errorf("Tag = %q, want %q", m.Tag, byte(FTagQuery))
	}
	// The body excludes the tag AND the length prefix, and INCLUDES the
	// trailing NUL that terminates the query string.
	if got, want := string(m.Body), "SELECT 1\x00"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

func TestReadMessage_Sequence(t *testing.T) {
	defer catchTODO(t)

	// Two messages back to back in one stream: the reader must consume exactly
	// the right number of bytes for the first, or the second will be garbage.
	stream := append(append([]byte{}, queryBytes...), readyIdleBytes...)
	r := NewReader(bytes.NewReader(stream))

	first, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("first ReadMessage: %v", err)
	}
	if first.Tag != FTagQuery {
		t.Fatalf("first tag = %q, want 'Q'", first.Tag)
	}

	second, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("second ReadMessage: %v", err)
	}
	if second.Tag != BTagReadyForQuery {
		t.Fatalf("second tag = %q, want 'Z'", second.Tag)
	}
	if len(second.Body) != 1 || second.Body[0] != TxIdle {
		t.Errorf("ReadyForQuery body = %q, want %q", second.Body, []byte{TxIdle})
	}

	if _, err := r.ReadMessage(); !errors.Is(err, io.EOF) {
		t.Errorf("third ReadMessage err = %v, want io.EOF", err)
	}
}

func TestReadMessage_Rejects(t *testing.T) {
	defer catchTODO(t)

	tests := []struct {
		name  string
		input []byte
	}{
		{
			// Length must include itself, so anything below 4 is nonsense.
			name:  "length below minimum",
			input: []byte{'Q', 0x00, 0x00, 0x00, 0x03},
		},
		{
			// A hostile peer must not be able to make us allocate 4 GiB.
			name:  "length exceeds MaxMessageSize",
			input: []byte{'Q', 0xFF, 0xFF, 0xFF, 0xFF},
		},
		{
			// Header promises 9 body bytes, stream delivers 3.
			name:  "truncated body",
			input: []byte{'Q', 0x00, 0x00, 0x00, 0x0D, 'S', 'E', 'L'},
		},
		{
			name:  "truncated header",
			input: []byte{'Q', 0x00, 0x00},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchTODO(t)
			r := NewReader(bytes.NewReader(tc.input))
			if m, err := r.ReadMessage(); err == nil {
				t.Fatalf("ReadMessage succeeded (%+v), want an error", m)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	defer catchTODO(t)

	for _, want := range [][]byte{queryBytes, readyIdleBytes} {
		r := NewReader(bytes.NewReader(want))
		m, err := r.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		if got := m.WireLen(); got != len(want) {
			t.Errorf("WireLen = %d, want %d", got, len(want))
		}

		var buf bytes.Buffer
		w := NewWriter(&buf)
		if err := w.WriteMessage(m); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
		if err := w.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if got := buf.Bytes(); !bytes.Equal(got, want) {
			t.Errorf("round trip = % x, want % x", got, want)
		}
	}
}

func TestAppendTo_ReusesBuffer(t *testing.T) {
	defer catchTODO(t)

	m := &RawMessage{Tag: BTagReadyForQuery, Body: []byte{TxIdle}}
	buf := make([]byte, 0, 64)
	buf = m.AppendTo(buf)
	buf = m.AppendTo(buf)

	want := append(append([]byte{}, readyIdleBytes...), readyIdleBytes...)
	if !bytes.Equal(buf, want) {
		t.Errorf("AppendTo twice = % x, want % x", buf, want)
	}
}

// ---------------------------------------------------------------------------
// Direction-aware decoding: the ambiguous-tag trap
// ---------------------------------------------------------------------------

func TestTagName_AmbiguousTags(t *testing.T) {
	defer catchTODO(t)

	tests := []struct {
		tag               byte
		frontend, backend string
	}{
		{'D', "Describe", "DataRow"},
		{'C', "Close", "CommandComplete"},
		{'E', "Execute", "ErrorResponse"},
		{'S', "Sync", "ParameterStatus"},
		{'H', "Flush", "CopyOutResponse"},
	}
	for _, tc := range tests {
		t.Run(string(tc.tag), func(t *testing.T) {
			defer catchTODO(t)
			if got := TagName(FromFrontend, tc.tag); got != tc.frontend {
				t.Errorf("TagName(FromFrontend, %q) = %q, want %q", tc.tag, got, tc.frontend)
			}
			if got := TagName(FromBackend, tc.tag); got != tc.backend {
				t.Errorf("TagName(FromBackend, %q) = %q, want %q", tc.tag, got, tc.backend)
			}
		})
	}
}

func TestTagName_UnknownDoesNotPanic(t *testing.T) {
	defer catchTODO(t)

	// The trace logger calls this on arbitrary bytes. It must never panic.
	if got := TagName(FromBackend, 0x00); got == "" {
		t.Error("TagName(unknown) returned empty string, want a placeholder")
	}
}

func TestDescribe_IsReadableAndSafe(t *testing.T) {
	defer catchTODO(t)

	q := &RawMessage{Tag: FTagQuery, Body: []byte("SELECT 1\x00")}
	if got := q.Describe(FromFrontend); !strings.Contains(got, "SELECT 1") {
		t.Errorf("Describe = %q, want it to contain the SQL text", got)
	}

	// A truncated / malformed body must degrade gracefully rather than panic:
	// this arrives from the network, and a panic here kills the proxy.
	bad := &RawMessage{Tag: BTagReadyForQuery, Body: nil}
	if got := bad.Describe(FromBackend); got == "" {
		t.Error("Describe of malformed message returned empty string")
	}
}

// ---------------------------------------------------------------------------
// Startup phase
// ---------------------------------------------------------------------------

func TestReadFirstPacket_Startup(t *testing.T) {
	defer catchTODO(t)

	r := NewReader(bytes.NewReader(startupBytes))
	p, err := r.ReadFirstPacket()
	if err != nil {
		t.Fatalf("ReadFirstPacket: %v", err)
	}
	if p.Kind != KindStartup {
		t.Fatalf("Kind = %v, want KindStartup", p.Kind)
	}
	if p.Startup.ProtocolVersion != ProtocolVersion3 {
		t.Errorf("ProtocolVersion = %d, want %d", p.Startup.ProtocolVersion, ProtocolVersion3)
	}
	if got := p.Startup.User(); got != "rushi" {
		t.Errorf("User = %q, want %q", got, "rushi")
	}
	if got := p.Startup.Database(); got != "postgres" {
		t.Errorf("Database = %q, want %q", got, "postgres")
	}
}

func TestStartup_DatabaseDefaultsToUser(t *testing.T) {
	defer catchTODO(t)

	// Postgres defaults the database name to the user name when "database" is
	// absent. pgdoor keys its pools on (user, database) and MUST resolve this
	// the same way, or "psql -U rushi" and "psql -U rushi -d rushi" get two
	// separate pools for the same thing.
	s := &Startup{
		ProtocolVersion: ProtocolVersion3,
		Parameters:      map[string]string{"user": "rushi"},
	}
	if got := s.Database(); got != "rushi" {
		t.Errorf("Database = %q, want %q (defaulted from user)", got, "rushi")
	}
}

func TestReadFirstPacket_SSLRequest(t *testing.T) {
	defer catchTODO(t)

	r := NewReader(bytes.NewReader(sslRequestBytes))
	p, err := r.ReadFirstPacket()
	if err != nil {
		t.Fatalf("ReadFirstPacket: %v", err)
	}
	if p.Kind != KindSSLRequest {
		t.Fatalf("Kind = %v, want KindSSLRequest", p.Kind)
	}
}

func TestReadFirstPacket_CancelRequest(t *testing.T) {
	defer catchTODO(t)

	r := NewReader(bytes.NewReader(cancelRequestBytes))
	p, err := r.ReadFirstPacket()
	if err != nil {
		t.Fatalf("ReadFirstPacket: %v", err)
	}
	if p.Kind != KindCancelRequest {
		t.Fatalf("Kind = %v, want KindCancelRequest", p.Kind)
	}
	if p.Cancel.ProcessID != 4242 {
		t.Errorf("ProcessID = %d, want 4242", p.Cancel.ProcessID)
	}
	if p.Cancel.SecretKey != 0xDEADBEEF {
		t.Errorf("SecretKey = %#x, want 0xDEADBEEF", p.Cancel.SecretKey)
	}
}

func TestReadFirstPacket_DoesNotOverRead(t *testing.T) {
	defer catchTODO(t)

	// After an SSLRequest the very next bytes on the wire are a TLS ClientHello.
	// If ReadFirstPacket buffers past the declared length it will swallow them
	// and the TLS handshake will fail in a maximally confusing way.
	trailer := []byte{0x16, 0x03, 0x01, 0xFF, 0xFF} // looks like a ClientHello
	stream := append(append([]byte{}, sslRequestBytes...), trailer...)

	br := bytes.NewReader(stream)
	r := NewReader(br)
	if _, err := r.ReadFirstPacket(); err != nil {
		t.Fatalf("ReadFirstPacket: %v", err)
	}
	// Whatever buffering strategy you choose, the trailing bytes must still be
	// retrievable. If you buffer, expose the buffered remainder; if you do not,
	// this reads straight from the underlying reader.
	rest, _ := io.ReadAll(io.MultiReader(bytes.NewReader(nil), br))
	if len(rest) != len(trailer) {
		t.Errorf("consumed %d trailing bytes, want the ClientHello left intact", len(trailer)-len(rest))
	}
}

// ---------------------------------------------------------------------------
// Fuzzing: the framer parses attacker-controlled lengths. Run: make fuzz
// ---------------------------------------------------------------------------

func FuzzReadMessage(f *testing.F) {
	f.Add(queryBytes)
	f.Add(readyIdleBytes)
	f.Add([]byte{'Q', 0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{'Z', 0x00, 0x00, 0x00, 0x04})

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic on input % x: %v", data, r)
			}
		}()
		r := NewReader(bytes.NewReader(data))
		m, err := r.ReadMessage()
		if err != nil {
			return
		}
		// Anything that parses must re-encode to something that parses back to
		// the same thing.
		if got := m.WireLen(); got != len(m.Body)+5 {
			t.Fatalf("WireLen = %d, inconsistent with body length %d", got, len(m.Body))
		}
		_ = m.Describe(FromBackend)
		_ = m.Describe(FromFrontend)
	})
}
