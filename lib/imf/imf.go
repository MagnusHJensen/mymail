// IMF is a packaged containing shared definitions and helpers for interacting with Internet Message Format (IMF)
//
// RFC - https://www.rfc-editor.org/info/rfc5322
package imf

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// simple struct layout for now

type MessageFormat struct {
	Headers map[string]string // Headers is a map of key=value, order does not matter in IMF - not raw, so needs \r\n appended
	Body    []string          // body is a list of lines as raw text
}

func NewMessageFormat(reader *bufio.Reader) (*MessageFormat, error) {
	headerSeparatorHit := false

	message := &MessageFormat{
		Headers: map[string]string{},
		Body:    []string{},
	}

outer:
	for {
		if !headerSeparatorHit {
			name, value, err := readHeader(reader)
			if err != nil {
				return nil, fmt.Errorf("failed to read header: %w", err)
			}

			if name == "" && value == "" {
				// we hit "\r\n" but no data, we hit the header/body separator
				headerSeparatorHit = true
				continue
			}

			message.Headers[name] = value
		} else {
			line := ""
			for {
				// read all bytes until \r\n then add them on
				if peek, err := reader.Peek(2); err != nil {
					if errors.Is(err, io.EOF) {
						break outer
					}
					return nil, fmt.Errorf("failed to peek two bytes for body: %w", err)
				} else if peek[0] == '\r' && peek[1] == '\n' {
					reader.Discard(2)
					line += "\r\n"
					break
				}

				read, err := reader.ReadByte()
				if err != nil {
					return nil, fmt.Errorf("failed to read byte for body: %w", err)
				}
				line += string(read)
			}
			message.Body = append(message.Body, line)
		}
	}

	return message, nil
}

func readHeader(reader *bufio.Reader) (string, string, error) {
	headerName := ""
	headerValue := ""
	finishedName := false // switches when we read the first ':' // RFC5322 2.2

	for {
		if peek, err := reader.Peek(2); err != nil {
			return "", "", fmt.Errorf("failed to peek 2 bytes when reading header: %w", err)
		} else if peek[0] == '\r' && peek[1] == '\n' {
			// we peeked into the CRLF, discard the two bytes and exit
			reader.Discard(2)
			return strings.TrimSpace(headerName), strings.TrimSpace(headerValue), nil
		}

		readByte, err := reader.ReadByte()
		if err != nil {
			return "", "", fmt.Errorf("failed to read byte for header: %w", err)
		}
		if readByte == ':' && !finishedName {
			finishedName = true
			continue // we consumed it, so start reading the next
		}

		if !finishedName {
			if readByte == ':' {
				finishedName = true
				continue // we consumed it so start reading the value
			}

			headerName += string(readByte)
			continue
		}

		headerValue += string(readByte)
	}
}

func (f *MessageFormat) RawHeaders() string {
	headers := ""
	for key, value := range f.Headers {
		headers += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	headers += "\r\n" // header separator needs to be added

	return headers
}

func (f *MessageFormat) RawFile() string {
	// First add all headers
	rawFile := ""
	rawFile += f.RawHeaders()
	rawFile += f.RawBody()

	return rawFile
}

func (f *MessageFormat) RawBody() string {
	rawBody := ""
	for _, line := range f.Body {
		rawBody += line
	}

	return rawBody
}
