package smtp

type Command string

const (
	ExtendedHelloCmd Command = "EHLO"
	HelloCmd         Command = "HELO"

	MailCmd      Command = "MAIL"
	RecipientCmd Command = "RCPT"
	DataCmd      Command = "DATA"

	QuitCmd Command = "QUIT"

	// Extension specific commands

	// STARTTLS
	// https://datatracker.ietf.org/doc/html/rfc3207
	StartTLSCmd Command = "STARTTLS"

	// AUTH
	// https://datatracker.ietf.org/doc/html/rfc4954
	AuthCmd Command = "AUTH"
)
