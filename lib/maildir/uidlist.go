package maildir

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type UIDListRecord struct {
	Meta *UniqueMeta
	UID  uint32
}

type UIDList struct {
	FormatVersion   string
	Validity        uint32 //uidvalidity
	Next            uint32 // uidnext
	recordsByUID    map[uint32]*UIDListRecord
	recordsByUnique map[string]*UIDListRecord
}

func (l *UIDList) CountExists() int {
	return len(l.recordsByUID)
}

func readUIDList(dirPath string) (*UIDList, error) {
	uidListFile, err := os.Open(path.Join(dirPath, "uidlist"))
	defer uidListFile.Close()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// if it doesn't exist we create it
			return createUIDList()
		}
		return nil, fmt.Errorf("failed to open uidlist file: %w", err)
	}

	reader := bufio.NewReader(uidListFile)
	// uidlist is LF delimited

	lines := []string{}
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read line in uidlist: %w", err)
		}

		lines = append(lines, strings.TrimRight(string(line), "\n"))
	}

	if len(lines) < 1 {
		// Should not happen, means a corrupted file, but let's create it again
		return createUIDList()
	}

	// first line is always metadata
	metadata := strings.Split(lines[0], " ")
	if len(metadata) < 3 {
		// we need 3 pieces, version, validity, next
		return nil, errors.New("metadata line looks corrupted with less than 3 items")
	}
	version := metadata[0]
	validity := uint32(time.Now().Unix())
	if time, found := strings.CutPrefix(metadata[1], "V"); found {
		parsed, err := strconv.ParseUint(time, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse validity: %w", err)
		}
		validity = uint32(parsed)
	}
	next := uint32(1)
	if time, found := strings.CutPrefix(metadata[2], "N"); found {
		parsed, err := strconv.ParseUint(time, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse next: %w", err)
		}
		next = uint32(parsed)
	}

	uidList := &UIDList{
		FormatVersion:   version,
		Validity:        validity,
		Next:            next,
		recordsByUID:    make(map[uint32]*UIDListRecord),
		recordsByUnique: make(map[string]*UIDListRecord),
	}

	// parse each row as a record
	for idx, line := range lines[1:] {
		parts := strings.SplitN(line, " :", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("line %d did not have two parts", idx)
		}

		uidStr := parts[0]
		uidParsed, err := strconv.ParseUint(uidStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse uid on line %d: %w", idx, err)
		}

		uniqueMeta, err := ParseUniqueMeta(parts[1])
		if err != nil {
			return nil, fmt.Errorf("could not parse unique meta string: %w", err)
		}

		uidList.insertRecord(&UIDListRecord{
			Meta: uniqueMeta,
			UID:  uint32(uidParsed),
		})
	}

	return uidList, nil
}

func (l *UIDList) rewrite(dirPath string) error {
	tmpFileName := fmt.Sprintf("uidlist.tmp.%d", os.Getpid())
	tmpFile, err := os.Create(path.Join(dirPath, tmpFileName))
	if err != nil {
		return fmt.Errorf("failed to create %q: %w", tmpFileName, err)
	}

	writer := bufio.NewWriter(tmpFile)
	if _, err := writer.WriteString(fmt.Sprintf("%s V%d N%d\n", l.FormatVersion, l.Validity, l.Next)); err != nil {
		return fmt.Errorf("failed to write header line in %s: %w", tmpFileName, err)
	}

	for _, record := range l.recordsByUID {
		if _, err := writer.WriteString(fmt.Sprintf("%d :%s\n", record.UID, record.Meta.ToFileName())); err != nil {
			return fmt.Errorf("failed to write record line: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync %q: %w", tmpFileName, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close %q: %w", tmpFileName, err)
	}

	if err := os.Rename(path.Join(dirPath, tmpFileName), path.Join(dirPath, "uidlist")); err != nil {
		return fmt.Errorf("failed to rename tmp to actual: %w", err)
	}

	// open the dir with sync flags to sync it
	mailBoxDir, err := os.OpenFile(dirPath, os.O_SYNC, os.ModeDir)
	if err != nil {
		return fmt.Errorf("failed to open the mailbox root dir: %w", err)
	}
	defer mailBoxDir.Close()

	if err := mailBoxDir.Sync(); err != nil {
		return fmt.Errorf("failed syncing mailbox root dir: %w", err)
	}

	return nil
}

// insertRecord inserts an already parsed record
func (l *UIDList) insertRecord(record *UIDListRecord) {
	l.recordsByUID[record.UID] = record
	l.recordsByUnique[record.Meta.ToFileName()] = record
}

func (l *UIDList) addRecord(unique *UniqueMeta) uint32 {
	record := &UIDListRecord{
		Meta: unique,
		UID:  l.Next,
	}

	// increment next
	// TODO: Make this a safe operating by checking bounds + updating validity signal
	l.Next++

	l.recordsByUID[record.UID] = record
	l.recordsByUnique[unique.ToFileName()] = record
	return record.UID
}

func (l *UIDList) removeRecord(record *UIDListRecord) {
	delete(l.recordsByUID, record.UID)
	delete(l.recordsByUnique, record.Meta.ToFileName())
}

func createUIDList() (*UIDList, error) {
	return &UIDList{
		FormatVersion:   "1",
		Validity:        uint32(time.Now().Unix()),
		Next:            1,
		recordsByUID:    make(map[uint32]*UIDListRecord),
		recordsByUnique: make(map[string]*UIDListRecord),
	}, nil
}
