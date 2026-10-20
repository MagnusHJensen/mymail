package maildir

import (
	"bufio"
	"cmp"
	"fmt"
	"os"
	"path"
	"slices"
	"sync"

	"dk.magnusjensen/mymail/lib/imf"
)

type Mailbox struct {
	// path is the absolute path to the mailbox root (not root of maildir)
	path    string
	uidList *UIDList

	mutex sync.RWMutex

	messages []*MailboxMessage
}

func newMailbox(path string, uidList *UIDList) *Mailbox {

	return &Mailbox{
		path:     path,
		uidList:  uidList,
		messages: make([]*MailboxMessage, 0),
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
	return len(m.messages)
}

func (m *Mailbox) GetUIDValidity() uint32 {
	return m.uidList.Validity
}

func (m *Mailbox) GetUIDNext() uint32 {
	return m.uidList.Next
}

func (m *Mailbox) assign(meta *MessageMeta) {
	assignedUID := m.uidList.addRecord(meta.Unique)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// append to the end as UID always goes up
	// ? If issues arise once we get to counter wrapping, we can tackle it then.
	m.messages = append(m.messages, &MailboxMessage{
		UID:  assignedUID,
		Meta: meta,
	})
}

// loads a message into the memory of the mailbox
func (m *Mailbox) loadAll(metas []*MailboxMessage) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, meta := range metas {
		m.messages = append(m.messages, meta)
	}

	slices.SortFunc(m.messages, func(a, b *MailboxMessage) int {
		return cmp.Compare(a.UID, b.UID)
	})
}

func (m *Mailbox) BySeq(seq uint32) (message *Message, sequence uint32, found bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if seq == 0 || int(seq) > len(m.messages) {
		return nil, 0, false
	}

	foundMessage := m.messages[seq-1]

	messageFile, err := os.Open(path.Join(m.path, "cur", foundMessage.Meta.ToFileName()))
	if err != nil {
		return nil, 0, false
	}

	format, err := imf.NewMessageFormat(bufio.NewReader(messageFile))
	if err != nil {
		return nil, 0, false
	}

	return &Message{
		MailboxMessage: foundMessage,
		Format:         format,
	}, seq, true
}

func (m *Mailbox) ByUID(uid uint32) (message *Message, seq uint32, found bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	i, found := slices.BinarySearchFunc(m.messages, uid, func(msg *MailboxMessage, uid uint32) int {
		return cmp.Compare(msg.UID, uid)
	})
	if !found {
		return nil, 0, false
	}

	foundMessage := m.messages[i]

	messageFile, err := os.Open(path.Join(m.path, "cur", foundMessage.Meta.ToFileName()))
	if err != nil {
		return nil, 0, false
	}

	format, err := imf.NewMessageFormat(bufio.NewReader(messageFile))
	if err != nil {
		return nil, 0, false
	}

	return &Message{
		MailboxMessage: foundMessage,
		Format:         format,
	}, uint32(i + 1), true
}
