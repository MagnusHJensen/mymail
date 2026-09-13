package smtp

import "slices"

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

	// Temporary negative replies
	CodeServiceNotAvailable Code = 421

	// Permanent negative replies
	CodeSyntaxError       Code = 500
	CodeNotImplemented    Code = 502
	CodeBadSequence       Code = 503
	CodeAuthRequired      Code = 530 // 5.7.0
	CodeAuthFailed        Code = 535 // TODO: Maybe only with enhanced status codes
	CodeActionNotTaken    Code = 550
	CodeStorageExceeded   Code = 552
	CodeTLSRequired       Code = 538
	CodeTransactionFailed Code = 554
)

func IsFailureCode(code Code) bool {
	failureCodes := []Code{
		CodeSyntaxError,
		CodeNotImplemented,
		CodeBadSequence,
		CodeAuthRequired,
		CodeAuthFailed,
		CodeActionNotTaken,
		CodeTLSRequired,
		CodeTransactionFailed,
	}
	return slices.Contains(failureCodes, code)
}
