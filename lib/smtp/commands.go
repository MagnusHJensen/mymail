package smtp

type Command string

const (
	ExtendedHelloCmd Command = "EHLO"
	HelloCmd         Command = "HELO"

	MailCmd      Command = "MAIL"
	RecipientCmd Command = "RCPT"
	DataCmd      Command = "DATA"

	QuitCmd Command = "QUIT"
)
