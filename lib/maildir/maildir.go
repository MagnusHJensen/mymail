// Package maildir provides functionality for interacting with Maildir mailboxes.
// It allows reading, writing, and managing emails stored in the Maildir format.
// Specifically the Maildir++ format, which is an extension of the original Maildir format
// with additional features such as subfolders and unique filename conventions.
package maildir

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type MaildirStore struct {
	rootPath string
}

func NewMaildirStore(rootPath string) *MaildirStore {
	return &MaildirStore{
		rootPath: rootPath,
	}
}

func (s *MaildirStore) RootPath() string {
	return s.rootPath
}

func (s *MaildirStore) StoreMail(mail []byte) error {
	if err := s.ensureMaildir(); err != nil {
		return err
	}
	uniqueName := s.generateUniqueName()
	tmpPath := fmt.Sprintf("%s/tmp/%s", s.rootPath, uniqueName)
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

	final := fmt.Sprintf("%s,S=%d", uniqueName, sizeWritten)
	if err := os.Rename(tmpPath, fmt.Sprintf("%s/new/%s", s.rootPath, final)); err != nil {
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

func (s *MaildirStore) generateUniqueName() string {
	// Q1 should be a counter, but for now hardcoded to 1, "." <host> should be the server hostname it serves, but for now hardcoded to smtp.
	return fmt.Sprintf("%d.M%dP%dQ1.smtp", time.Now().Unix(), time.Now().UnixMicro(), os.Getpid())
}
