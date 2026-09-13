package smtp

type Code uint

const (
	// 220 is unique that it is only served on establishing connection
	CodeReady Code = 220
	// Positive completion replies
	CodeShuttingDown Code = 221
	CodeAuthOK       Code = 235 // TODO: Maybe only with enhanced status codes
	CodeOK           Code = 250

	// Positive intermediate replies
	CodeStartData Code = 354

	// Permanent negative replies
	CodeSyntaxError       Code = 500
	CodeNotImplemented    Code = 502
	CodeBadSequence       Code = 503
	CodeAuthFailed        Code = 535 // TODO: Maybe only with enhanced status codes
	CodeActionNotTaken    Code = 550
	CodeTransactionFailed Code = 554
)
