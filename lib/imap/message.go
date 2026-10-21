package imap

import "strings"

type MessageFlag string

const (
	SeenFlag MessageFlag = "\\Seen"
)

// FlagListFromMaildir takes in the raw string of flags represented in Maildir by individual characters
func FlagListFromMaildir(mailDirFlags string) string {
	flags := []string{}
	for _, flag := range mailDirFlags {
		if flag == 'S' {
			flags = append(flags, string(SeenFlag))
		}
	}

	return strings.Join(flags, " ")
}
