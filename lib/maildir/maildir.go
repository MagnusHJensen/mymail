// Package maildir provides functionality for interacting with Maildir mailboxes.
// It allows reading, writing, and managing emails stored in the Maildir format.
// Specifically the Maildir++ format, which is an extension of the original Maildir format
// with additional features such as subfolders and unique filename conventions.
package maildir

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

type MaildirStore struct {
	rootPath string
}

func NewMaildirStore(rootPath string) *MaildirStore {
	store := &MaildirStore{
		rootPath: rootPath,
	}

	if err := store.ensureMaildir(); err != nil {
		fmt.Printf("failed to ensure maildir: %q", rootPath)
	}
	return store
}

func (s *MaildirStore) RootPath() string {
	return s.rootPath
}

func (s *MaildirStore) StoreMail(mail []byte) error {
	uniqueMeta := GenerateUniqueMeta()
	// mail always lands in root/INBOX
	tmpPath := fmt.Sprintf("%s/tmp/%s", s.rootPath, uniqueMeta.ToFileName())
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	sizeWritten, err := tmpFile.Write(mail)
	if err != nil {
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	uniqueMeta.Size = uint(sizeWritten)
	if err := os.Rename(tmpPath, fmt.Sprintf("%s/new/%s", s.rootPath, uniqueMeta.ToFileName())); err != nil {
		return err
	}

	newDir, err := os.Open(filepath.Join(s.rootPath, "new"))
	if err != nil {
		return err
	}
	if err := newDir.Sync(); err != nil {
		return err
	}
	if err := newDir.Close(); err != nil {
		return err
	}

	return nil
}

func (s *MaildirStore) ensureMaildir() error {
	for _, sub := range []string{"tmp", "new", "cur"} {
		if err := os.MkdirAll(filepath.Join(s.rootPath, sub), 0700); err != nil {
			return fmt.Errorf("ensure maildir %s: %w", sub, err)
		}
	}
	return nil
}

// Getting the Mailbox, executes the maildir sync with the UIDList and move of "new" -> "cur"
func (s *MaildirStore) GetMailbox(mailboxToGet string) (*Mailbox, error) {
	mailboxDir := ""
	if mailboxToGet != "INBOX" {
		return nil, errors.New("non supported mail box")
	}

	// TODO: Something about a timeout here for the lock
	// aquire the lock for the full sync
	absMailboxDir := path.Join(s.rootPath, mailboxDir)
	lockFile, err := os.OpenFile(path.Join(absMailboxDir, "uidlist.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}
	// closing the lockfile also releases the lock safely
	defer lockFile.Close()

	// hold an exclusive lock on this file while we read the actual uidlist file and perform a sync
	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	if err != nil {
		return nil, fmt.Errorf("failed to lock file: %w", err)
	}

	uidList, err := readUIDList(absMailboxDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read uidlist: %w", err)
	}

	mailbox := newMailbox(absMailboxDir, uidList)

	// TODO: skip for STATUS
	if err := mailbox.migrateToCurrent(); err != nil {
		return nil, fmt.Errorf("failed to migrate to current: %w", err)
	}

	// collect all entries in "cur"

	currentMessages, err := mailbox.getCurrentMessages()
	if err != nil {
		return nil, fmt.Errorf("failed to collect current messages: %w", err)
	}

	assigned := []*MailboxMessage{}
	unassigned := []*MessageMeta{}

	// what is left in recordsByUniqueCopy is vanished from the maildir
	recordsByUniqueCopy := map[string]*UIDListRecord{}
	maps.Copy(recordsByUniqueCopy, uidList.recordsByUnique)
	for _, message := range currentMessages {

		// assigned
		if record := uidList.recordsByUnique[message.Unique.ToFileName()]; record != nil {
			assigned = append(assigned, &MailboxMessage{
				UID:  record.UID,
				Meta: message,
			})
			// delete from copy map
			delete(recordsByUniqueCopy, message.Unique.ToFileName())
			continue
		}

		// in current messages, but not in uidlist
		unassigned = append(unassigned, message)
	}

	mailbox.loadAll(assigned)

	// sort unassigned to be deterministic
	slices.SortFunc(unassigned, func(messageA, messageB *MessageMeta) int {
		// 1. by secs part in unique name
		secondsSort := messageA.Unique.UnixSeconds - messageB.Unique.UnixSeconds

		if secondsSort == 0 {
			// 2. the full unique part, bytewise ascending
			return strings.Compare(messageA.Unique.ToFileName(), messageB.Unique.ToFileName())
		}

		return int(secondsSort)
	})

	if len(unassigned) == 0 && len(recordsByUniqueCopy) == 0 {
		// nothing changed, no new unassigned files and no vanished files
		// TODO: most likely need to embed some data in here
		return mailbox, nil
	}

	// add unassigned records and assign them a UID (happens inside)
	for _, unassignedMeta := range unassigned {
		mailbox.assign(unassignedMeta)
	}

	// drop vanished records
	for _, value := range recordsByUniqueCopy {
		uidList.removeRecord(value)
	}

	// rewrite uidlist atomically (SPEC 10.8)
	// atomically by holding the lock
	if err := uidList.rewrite(absMailboxDir); err != nil {
		return nil, fmt.Errorf("failed to rewrite the uidlist: %w", err)
	}

	// release lock by closing the file via defer
	// TODO: Do we need more data here? like counts etc.
	return mailbox, nil
}
