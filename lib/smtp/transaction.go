package smtp

import "strings"

type MailTransaction struct {
	From string   `json:"from"`
	To   *string  `json:"to"`
	Data []string `json:"data"`
}

func (t *MailTransaction) GetRemoteAddress() string {
	if t.To == nil {
		return ""
	}

	parts := strings.SplitN(*t.To, "@", 2)
	_, hostName := parts[0], strings.TrimSuffix(parts[1], ">")

	return hostName
}
