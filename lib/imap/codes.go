package imap

type ResponseCode string

const (
	// Generic status response codes
	OkCode      ResponseCode = "OK"
	NoCode      ResponseCode = "NO"
	BadCode     ResponseCode = "BAD"
	ByeCode     ResponseCode = "BYE"
	PreAuthCode ResponseCode = "PREAUTH"

	CapabilityCode ResponseCode = "CAPABILITY"
)
