package smtp

import (
	"fmt"
	"strings"
)

type MailTransaction struct {
	From string   `json:"from"`
	To   *string  `json:"to"`
	Data []string `json:"data"`
}

func (t *MailTransaction) GetFromHostname() string {
	parts := strings.SplitN(t.From, "@", 2)
	_, hostName := parts[0], strings.TrimSuffix(parts[1], ">")

	return hostName
}

func (t *MailTransaction) GetRemoteAddress() string {
	if t.To == nil {
		return ""
	}

	parts := strings.SplitN(*t.To, "@", 2)
	_, hostName := parts[0], strings.TrimSuffix(parts[1], ">")

	return hostName
}

func (t *MailTransaction) ToLocalUser() string {
	if t.To == nil {
		return ""
	}

	parts := strings.SplitN(*t.To, "@", 2)
	localUser, _ := strings.TrimPrefix(parts[0], "<"), strings.TrimSuffix(parts[1], ">")
	return localUser
}

func (t *MailTransaction) TemporaryTO() string {
	leftTrimmed := strings.TrimLeft(*t.To, "<")
	return strings.TrimRight(
		leftTrimmed, ">",
	)
}

func (t *MailTransaction) DataAsByte() []byte {
	output := []byte{}
	for _, line := range t.Data {
		output = fmt.Appendf(output, "%s", line)
	}

	return output
}
