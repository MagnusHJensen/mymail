package smtp

import (
	"fmt"
	"slices"
	"strings"
)

type MailTransaction struct {
	From string   `json:"from"`
	To   *string  `json:"to"`
	Data []string `json:"data"` // TODO: Move to MaiLWrapper that contains headers as map[string]string and a body object, for now we have an append header method that manipulates this.
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

func (t *MailTransaction) AppendHeader(header, value string) {
	if t.hasHeader(header) {
		// no-op on existing header
		return
	}

	// first find the first line with \r\n (first empty row splits header and body)
	for i, line := range t.Data {
		if line == "\r\n" {
			t.Data = slices.Insert(t.Data, i, fmt.Sprintf("%s: %s\r\n", header, value))
			return
		}
	}
}

func (t *MailTransaction) hasHeader(header string) bool {
	for _, line := range t.Data {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(header)) {
			return true
		}
	}
	return false
}
