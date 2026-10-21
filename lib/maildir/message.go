package maildir

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"dk.magnusjensen/mymail/lib/imf"
)

// MessageMeta describes a single message's metadata from it's given filename. The uniqueness and it's flags
type UniqueMeta struct {
	UnixSeconds int64
	UnixMicro   int64
	ProcessID   int
	Counter     uint
	Hostname    string
	Size        uint // we have size here as part of the unique, but it is not set while the server writes the entry to "tmp" only after moved to "new"
}

func GenerateUniqueMeta() *UniqueMeta {
	return &UniqueMeta{
		UnixSeconds: time.Now().Unix(),
		UnixMicro:   time.Now().UnixMicro(),
		ProcessID:   os.Getpid(),
		Counter:     1,   // For now hardcoded to 1
		Hostname:    "@", // Hardcoded to @ to represent root
		Size:        0,
	}
}

func (m *UniqueMeta) ToFileName() string {
	unique := fmt.Sprintf("%d.M%dP%dQ%d.%s", m.UnixSeconds, m.UnixMicro, m.ProcessID, m.Counter, m.Hostname)
	if m.Size > 0 {
		unique += fmt.Sprintf(",S=%d", m.Size)
	}

	return unique
}

func ParseUniqueMeta(metadata string) (*UniqueMeta, error) {
	parts := strings.SplitN(metadata, ",", 2)
	if len(parts) != 2 {
		return nil, errors.New("unique metadata does not have two parts with ',' delim")
	}

	uniqueParts := strings.SplitN(parts[0], ".", 3)
	if len(uniqueParts) != 3 {
		return nil, errors.New("unique parts does not have 3 parts with '.' delim")
	}

	seconds := uniqueParts[0]
	hostName := uniqueParts[2]

	// first we cut P away, then we have Mxxx (in "micro") and xxxQxxx in "rest"
	// we then strip M away from "micro", and we cut 'Q' on "rest" to get process ID and counter
	micro, rest, found := strings.Cut(uniqueParts[1], "P")
	if !found {
		return nil, errors.New("could not find 'P' to cut against")
	}

	micro = strings.TrimPrefix(micro, "M")
	pid, counter, found := strings.Cut(rest, "Q")
	if !found {
		return nil, errors.New("could not find 'Q' to cut against")
	}

	parsedSeconds, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse seconds: %w", err)
	}
	parsedMicro, err := strconv.ParseInt(micro, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse micro seconds: %w", err)
	}
	parsedPID, err := strconv.ParseInt(pid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pid: %w", err)
	}
	parsedCounter, err := strconv.ParseUint(counter, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to parse counter: %w", err)
	}

	parsedSize, err := strconv.ParseUint(strings.TrimPrefix(parts[1], "S="), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to parse size: %w", err)
	}

	return &UniqueMeta{
		UnixSeconds: parsedSeconds,
		UnixMicro:   parsedMicro,
		ProcessID:   int(parsedPID),
		Counter:     uint(parsedCounter),
		Hostname:    hostName,
		Size:        uint(parsedSize),
	}, nil
}

type InfoMeta struct {
	FormatVersion string

	// Flags
	Seen bool // Maps to \Seen
}

func (m *InfoMeta) FlagString() string {
	flagList := ""
	if m.Seen {
		flagList += "S"
	}
	return flagList
}

func (m *InfoMeta) ToFileName() string {
	flagList := m.FlagString()

	return fmt.Sprintf("%s,%s", m.FormatVersion, flagList)
}

func ParseInfoMeta(infoMetaRaw string) (*InfoMeta, error) {
	// we expect to not have ':' delim here
	parts := strings.Split(infoMetaRaw, ",")
	if len(parts) < 1 {
		// update once we support more info meta
		return nil, errors.New("info meta does not carry minimum required fields")
	}

	// first part is format version
	version := parts[0]
	infoMeta := &InfoMeta{
		FormatVersion: version,
	}

	if len(parts) > 1 {
		// we have flags present
		// split each character as it is its own flag
		for _, flagChar := range parts[1] {
			if flagChar == 'S' {
				infoMeta.Seen = true
			}
		}
	}
	return infoMeta, nil
}

type MessageMeta struct {
	Unique *UniqueMeta
	Info   *InfoMeta
}

func (m *MessageMeta) ToFileName() string {
	return fmt.Sprintf("%s:%s", m.Unique.ToFileName(), m.Info.ToFileName())
}

func ParseMessageMeta(meta string) (*MessageMeta, error) {
	parts := strings.SplitN(meta, ":", 2)
	if len(parts) < 2 {
		return nil, errors.New("message meta does not carry two parts split by ':'")
	}

	unique, err := ParseUniqueMeta(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse unique meta for message: %w", err)
	}
	info, err := ParseInfoMeta(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse info meta for message: %w", err)
	}

	return &MessageMeta{
		Unique: unique,
		Info:   info,
	}, nil
}

type Message struct {
	*MailboxMessage
	Format *imf.MessageFormat
}

type MailboxMessage struct {
	UID  uint32
	Meta *MessageMeta // current full filename + flags
}
