package maildir

import (
	"bufio"
	"fmt"
	"os"
	"path"

	"dk.magnusjensen/mymail/lib/imf"
)

type Mailbox struct {
	// path is the absolute path to the mailbox root (not root of maildir)
	path           string
	uidList        *UIDList
	loadedMessages map[uint32]*MessageMeta //uid -> meta (full filename)
	// TODO: Move above to a slice, that is UID sorted
	// on initial scan/load, all assigned sorted by UID, then as we add each unassigned append them to the end

}

func newMailbox(path string, uidList *UIDList) *Mailbox {

	return &Mailbox{
		path:           path,
		uidList:        uidList,
		loadedMessages: make(map[uint32]*MessageMeta),
	}
}

// migrateToCurrent is spec 7.2
func (m *Mailbox) migrateToCurrent() error {
	// iterate each file in "/new", then rename into "/cur" with added empty info part ":2,"
	newDirEntries, err := os.ReadDir(path.Join(m.path, "new"))
	if err != nil {
		return fmt.Errorf("failed to read new dir: %w", err)
	}

	for _, entry := range newDirEntries {
		err := os.Rename(path.Join(m.path, "new", entry.Name()), path.Join(m.path, "cur", fmt.Sprintf("%s:%d,", entry.Name(), 2))) // hardcoded format version for info meta
		if err != nil {
			return fmt.Errorf("failed to rename %q: %w", entry.Name(), err)
		}
	}

	return nil
}

func (m *Mailbox) getCurrentMessages() ([]*MessageMeta, error) {
	// iterate each file in "/cur"
	curDirEntries, err := os.ReadDir(path.Join(m.path, "cur"))
	if err != nil {
		return nil, fmt.Errorf("failed to read cur dir: %w", err)
	}

	messageMetas := []*MessageMeta{}
	for _, entry := range curDirEntries {
		meta, err := ParseMessageMeta(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to parse message meta for %q: %w", entry.Name(), err)
		}

		messageMetas = append(messageMetas, meta)
	}

	return messageMetas, nil
}

func (m *Mailbox) Count() int {
	return m.uidList.CountExists()
}

func (m *Mailbox) GetUIDValidity() uint32 {
	return m.uidList.Validity
}

func (m *Mailbox) GetUIDNext() uint32 {
	return m.uidList.Next
}

func (m *Mailbox) assign(meta *MessageMeta) {
	assignedUID := m.uidList.addRecord(meta.Unique)

	m.loadedMessages[assignedUID] = meta
}

// loads a message into the memory of the mailbox
func (m *Mailbox) load(uid uint32, meta *MessageMeta) {
	m.loadedMessages[uid] = meta
}

// Fetch gets a list of messages by their uids
func (m *Mailbox) Fetch(uids []uint32) ([]*Message, error) {
	fetchedMessages := []*Message{}
	for _, uid := range uids {
		messageMeta, ok := m.loadedMessages[uid]
		if !ok {
			// TODO: Log? Fail?
			continue
		}

		messageFile, err := os.Open(path.Join(m.path, "cur", messageMeta.ToFileName()))
		if err != nil {
			return nil, fmt.Errorf("failed to open message file %q: %w", messageMeta.ToFileName(), err)
		}

		format, err := imf.NewMessageFormat(bufio.NewReader(messageFile))
		if err != nil {
			return nil, fmt.Errorf("failed to read message %q: %w", messageMeta.ToFileName(), err)
		}

		fetchedMessages = append(fetchedMessages, &Message{
			Meta:   messageMeta,
			Format: format,
			UID:    uid,
		})
	}

	return fetchedMessages, nil
}
