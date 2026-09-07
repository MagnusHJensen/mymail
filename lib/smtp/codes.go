package smtp

type Code uint

const (
	// 220 is unique that it is only served on establishing connection
	CodeReady Code = 220
	// Positive completion replies
	CodeShuttingDown Code = 221
	CodeOK           Code = 250

	// Positive intermediate replies
	CodeStartData Code = 354

	// Permanent negative replies
	CodeSyntaxError       Code = 500
	CodeNotImplemented    Code = 502
	CodeBadSequence       Code = 503
	CodeActionNotTaken    Code = 550
	CodeTransactionFailed Code = 554
)
