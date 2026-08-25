package pgwire

// Message tags for the PostgreSQL v3 wire protocol.
//
// TRAP: tags are ambiguous across directions. The same byte means different
// things depending on who sent it:
//
//	'D' -> Describe (frontend)  |  DataRow          (backend)
//	'C' -> Close    (frontend)  |  CommandComplete  (backend)
//	'E' -> Execute  (frontend)  |  ErrorResponse    (backend)
//	'S' -> Sync     (frontend)  |  ParameterStatus  (backend)
//	'H' -> Flush    (frontend)  |  CopyOutResponse  (backend)
//
// This is why every decode entry point takes a Direction. It is a constraint on
// the API, not an implementation detail.

// Frontend tags (client -> server).
const (
	FTagQuery           = 'Q'
	FTagParse           = 'P'
	FTagBind            = 'B'
	FTagDescribe        = 'D'
	FTagExecute         = 'E'
	FTagSync            = 'S'
	FTagClose           = 'C'
	FTagFlush           = 'H'
	FTagTerminate       = 'X'
	FTagCopyData        = 'd'
	FTagCopyDone        = 'c'
	FTagCopyFail        = 'f'
	FTagFunctionCall    = 'F'
	FTagPasswordMessage = 'p' // also SASLInitialResponse and SASLResponse
)

// Backend tags (server -> client).
const (
	BTagAuthentication           = 'R'
	BTagParameterStatus          = 'S'
	BTagBackendKeyData           = 'K'
	BTagReadyForQuery            = 'Z'
	BTagRowDescription           = 'T'
	BTagDataRow                  = 'D'
	BTagCommandComplete          = 'C'
	BTagErrorResponse            = 'E'
	BTagNoticeResponse           = 'N'
	BTagParseComplete            = '1'
	BTagBindComplete             = '2'
	BTagCloseComplete            = '3'
	BTagNoData                   = 'n'
	BTagParameterDescription     = 't'
	BTagPortalSuspended          = 's'
	BTagNotificationResponse     = 'A'
	BTagEmptyQueryResponse       = 'I'
	BTagCopyInResponse           = 'G'
	BTagCopyOutResponse          = 'H'
	BTagCopyBothResponse         = 'W'
	BTagNegotiateProtocolVersion = 'v'
)

// Authentication sub-codes carried in the body of a 'R' message.
const (
	AuthOK                = 0
	AuthKerberosV5        = 2
	AuthCleartextPassword = 3
	AuthMD5Password       = 5
	AuthGSS               = 7
	AuthGSSContinue       = 8
	AuthSSPI              = 9
	AuthSASL              = 10
	AuthSASLContinue      = 11
	AuthSASLFinal         = 12
)

// Transaction status bytes carried in the body of a ReadyForQuery ('Z') message.
//
// This is the single most important byte in the project: transaction pooling is
// "release the server connection when you relay a 'Z' carrying TxIdle".
const (
	TxIdle   = 'I' // not in a transaction block
	TxInTx   = 'T' // inside a transaction block
	TxFailed = 'E' // in a failed transaction block, awaiting ROLLBACK
)

// TagName returns a human-readable name for tag as sent in direction d.
//
// TODO(phase-1): implement. Unknown tags should render as something like
// "Unknown(0x41)" rather than panicking -- this function is used by the trace
// logger, which must never take down the proxy.
func TagName(d Direction, tag byte) string {
	panic("pgwire: TagName not implemented (phase 1)")
}
