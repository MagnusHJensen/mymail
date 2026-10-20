package session

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"dk.magnusjensen/mymail/lib/imap"
	"dk.magnusjensen/mymail/lib/maildir"
	"dk.magnusjensen/mymail/lib/sasl"
)

type imapServerSession struct {
	conn           *Connection
	tlsCertificate tls.Certificate
	authHandler    authHandler

	state sessionState

	// mail address of authed user ex. "bob@example.com"
	authedUser *string
	// The currently selected mailbox if any
	selectedMailbox *maildir.Mailbox

	logger *slog.Logger
}

type sessionState int

const (
	notAuthedSessionState sessionState = iota
	authedSessionState
	selectedSessionState
)

type authHandler interface {
	AuthenticateUser(username, password string) error
}

func NewIMAPServerSession(logger *slog.Logger, conn net.Conn, authHandler authHandler) *imapServerSession {
	return &imapServerSession{
		conn:        NewConnection(conn, logger),
		authHandler: authHandler,
		state:       notAuthedSessionState,
		logger:      logger,
	}
}

func (s *imapServerSession) SetTLSCertificate(cert tls.Certificate) {
	s.tlsCertificate = cert
}

func (s *imapServerSession) ExtendDeadline(duration time.Duration) {
	s.conn.net.SetDeadline(time.Now().Add(duration))
}

func (s *imapServerSession) Start() {
	s.logger.Info("Started server session")
	go s.serve()
}

func (s *imapServerSession) serve() {
	defer func() {
		if r := recover(); r != nil {
			stackTrace := debug.Stack()
			s.logger.Error("encountered panic", "error", r, "stack_trace", stackTrace)
		}
		s.conn.net.Close()
	}()

	if err := s.greet(); err != nil {
		s.logger.Error("failed to send ready, dropping connection", "err", err)
		return
	}

	for {
		line, err := s.conn.Read(512) // TODO: How many max bytes per line
		if err != nil {
			s.logger.Error("failed to read line", "err", err)
			return
		}

		// Allow 10 seconds to read each line and process
		// TODO: handle it per command based on RFC.
		s.ExtendDeadline(time.Second * 10)

		if failed := s.handleClientCommand(line); failed {
			return
		}
	}
}

func (s *imapServerSession) greet() error {
	return s.Send(imap.OkCode, "IMAP4rev2 server ready")
}

func (s *imapServerSession) handleClientCommand(line string) (terminate bool) {
	// client commands are always three part (for now)
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		s.Send(imap.BadCode, "Request must have tag and command")
		return false
	}
	tag, cmd := parts[0], strings.ToUpper(parts[1])

	switch cmd {
	case "CAPABILITY":
		return s.handleCapabilities(tag)
	case "LOGOUT":
		if err := s.Send(imap.ByeCode, "IMAP4rev2 Server logging out"); err != nil {
			fmt.Printf("Failed writing data: %v\n", err)
			return true
		}

		if err := s.Reply(tag, imap.OkCode, ""); err != nil {
			s.logger.Error("failed to reply", "err", err)
			return true
		}

		// "failed" to indicate connection closure
		return true
	case "AUTHENTICATE":
		if len(parts) < 3 {
			s.Reply(tag, imap.BadCode, "invalid arguments to AUTHENTICATE")
			return true
		}

		return s.handleAuthenticate(tag, parts[2])
	case "STARTTLS":
		if s.state != notAuthedSessionState {
			s.Reply(tag, imap.BadCode, "STARTTLS is only valid in Not Authenticated state")
			return false
		}

		if s.conn.isTLSSessionActive {
			s.Reply(tag, imap.BadCode, "TLS is already active")
			return false
		}

		// reply with OK and start TLS negotiation right after
		s.Reply(tag, imap.OkCode, "")
		s.UpgradeToTLS()
		return false
	case "SELECT":
		if len(parts) < 3 {
			s.Reply(tag, imap.BadCode, "invalid arguments to SELECT")
			return true
		}

		return s.handleSelect(tag, parts[2])
	case "CLOSE":
		return s.handleClose(tag)
	case "FETCH":
		return s.handleFetch(tag, parts[2])
	default:
		if err := s.Reply(tag, imap.NoCode, "not supported"); err != nil {
			s.logger.Error("failed to reply", "err", err)
			return true
		}
		return false
	}

}

func (s *imapServerSession) handleCapabilities(tag string) (terminate bool) {
	capabilities := []string{"IMAP4rev2", "IMAP4rev1", "LOGINDISABLED"}

	if !s.conn.isTLSSessionActive {
		// Advertise STARTTLS
		capabilities = append(capabilities, "STARTTLS")
	} else if s.conn.isTLSSessionActive && s.state == notAuthedSessionState {
		capabilities = append(capabilities, fmt.Sprintf("AUTH=%s", sasl.PlainMechanism))
	}

	if err := s.Send(imap.CapabilityCode, strings.Join(capabilities, " ")); err != nil {
		fmt.Printf("Failed writing data: %v\n", err)
		return true
	}

	if err := s.Reply(tag, imap.OkCode, ""); err != nil {
		s.logger.Error("failed to reply", "err", err)
		return true
	}
	return false
}

func (s *imapServerSession) handleAuthenticate(tag, args string) (terminate bool) {
	if s.state != notAuthedSessionState {
		s.Reply(tag, imap.BadCode, "AUTHENTICATE is only valid in Not Authenticated state")
		return false
	}

	if !s.conn.isTLSSessionActive {
		// do not allow auth over non-tls
		s.Reply(tag, imap.NoCode, "AUTHENTICATE not allowed over non-tls connection")
		return true
	}

	authArgs := strings.Split(args, " ")

	if len(authArgs) < 1 {
		s.Reply(tag, imap.BadCode, "invalid arguments to AUTHENTICATE")
		return true
	}

	mechanism := authArgs[0]

	supportedMechanisms := []string{sasl.PlainMechanism}

	if !slices.Contains(supportedMechanisms, mechanism) {
		s.logger.Warn("auth mechanism not supported", "requested_auth", mechanism)
		s.Reply(tag, imap.NoCode, "mechanism not supported")
		return true
	}

	// continue command to get user/password
	if err := s.ContinueCommand(""); err != nil {
		s.logger.Error("failed to continue AUTHENTICATE command", "err", err)
		return true
	}

	authEncoded, err := s.conn.Read(512)
	if err != nil {
		s.logger.Error("failed to read SASL AUTH sequence", "err", err)
		return true
	}

	_, username, password, err := sasl.ParsePlainAuthMessage(authEncoded)
	if err != nil {
		s.Reply(tag, imap.BadCode, "failed to parse PLAIN auth")
		return true
	}

	if err := s.authHandler.AuthenticateUser(username, password); err != nil {
		s.Reply(tag, imap.BadCode, "invalid username or password")
		return true
	}

	s.state = authedSessionState
	s.authedUser = &username

	s.Reply(tag, imap.OkCode, "authed")
	return false
}

func (s *imapServerSession) handleSelect(tag, args string) (terminate bool) {
	if s.state != authedSessionState {
		s.Reply(tag, imap.BadCode, "SELECT is not possible when not in authed state")
		return false
	}

	parts := strings.Split(args, " ")
	selectedMailbox := parts[0]

	logger := s.logger.With("mailbox", selectedMailbox)

	splitMail := strings.Split(*s.authedUser, "@")
	local, hostname := splitMail[0], splitMail[1]
	mailDirPath := filepath.Join("maildir", hostname, local)
	s.logger.Debug("opening store with path", "path", mailDirPath, "authed_user", *s.authedUser)
	maildirStore := maildir.NewMaildirStore(mailDirPath)

	mailbox, err := maildirStore.GetMailbox(selectedMailbox)
	if err != nil {
		logger.Error("failed to SELECT mailbox", "err", err)
		s.Reply(tag, imap.NoCode, "failed to SELECT mailbox")
		return false
	}

	if mailbox == nil {
		logger.Error("mailbox was returned nil, but no error")
		return true
	}

	logger.Debug("successfully selected mailbox")
	s.selectedMailbox = mailbox
	s.state = selectedSessionState

	// EXISTS
	if err := s.SendNoCode(fmt.Sprintf("%d EXISTS", mailbox.Count())); err != nil {
		s.logger.Error("failed to send EXISTS reply", "err", err)
		return true
	}

	// LIST
	// TODO: ensure this is correct and serves the right state etc.
	if err := s.SendNoCode(`LIST () "/" INBOX`); err != nil {
		s.logger.Error("failed to send LIST reply", "err", err)
		return true
	}

	// FLAGS - default to general-system flags
	// TODO: Add correct support for sending this from somewhere
	if err := s.SendNoCode("FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)"); err != nil {
		s.logger.Error("failed to send FLAGS reply")
		return true
	}

	// now OK-untagged responses
	if err := s.Send(imap.OkCode, fmt.Sprintf("[UIDVALIDITY %d] UIDs valid", mailbox.GetUIDValidity())); err != nil {
		s.logger.Error("failed to send UIDVALIDITY reply", "err", err)
		return true
	}
	if err := s.Send(imap.OkCode, fmt.Sprintf("[UIDNEXT %d] Predicted next UID", mailbox.GetUIDNext())); err != nil {
		s.logger.Error("failed to send UIDNEXT reply", "err", err)
		return true
	}
	if err := s.Send(imap.OkCode, "[PERMANENTFLAGS (\\Deleted \\Seen)] Limited"); err != nil {
		s.logger.Error("failed to send PERMANENTFLAGS reply", "err", err)
		return true
	}

	// lastly we reply to the tag with read-only or read-write
	if err := s.Reply(tag, imap.OkCode, "[READ-WRITE] SELECT completed"); err != nil {
		s.logger.Error("failed to send tagged reply for SELECT", "err", err)
		return true
	}
	return false
}

func (s *imapServerSession) handleClose(tag string) (terminate bool) {
	if s.state != selectedSessionState {
		s.Reply(tag, imap.BadCode, "CLOSE is only valid in selected state")
		return false
	}

	// TODO: delete all messages with \Delete flag permanently from the currently selected mailbox

	// move back to authed state and thereby also unselect the current mailbox
	s.selectedMailbox = nil
	s.state = authedSessionState

	if err := s.Reply(tag, imap.OkCode, "CLOSE completed"); err != nil {
		s.logger.Error("failed to reply to CLOSE command", "err", err)
		return true
	}

	return false
}

func (s *imapServerSession) handleFetch(tag, args string) (terminate bool) {
	if s.state != selectedSessionState {
		s.Reply(tag, imap.BadCode, "FETCH is not allowed in a non selected state")
		return false
	}

	parts := strings.Split(args, " ")
	if len(parts) < 1 {
		s.Reply(tag, imap.BadCode, "invalid number of arguments to FETCH")
		return false
	}

	sequenceSet := parts[0]
	//TODO: for now we only support a single number
	parsedSequenceSet, err := strconv.ParseUint(sequenceSet, 10, 32)
	if err != nil {
		s.Reply(tag, imap.BadCode, "could not parse as a single number, this server only supports a single number for now")
		return false
	}

	messages, err := s.selectedMailbox.Fetch([]uint32{uint32(parsedSequenceSet)})
	if err != nil {
		s.logger.Error("failed to fetch mails", "err", err)
		return true
	}

	for _, message := range messages {
		internalDate := time.Unix(message.Meta.Unique.UnixSeconds, 0).Format("_2-Jan-2006 15:04:05 -0700")
		size := message.Meta.Unique.Size
		headerPeek := ""
		for key, value := range message.Format.Headers {
			headerPeek += fmt.Sprintf("%s: %s\r\n", key, value)
		}
		headerPeek += "\r\n" // header separator needs to be added
		// TODO: We hardcode the response based on Apple mail, but read the fetch args to what we should return
		// (FLAGS INTERNALDATE UID RFC822.SIZE BODY.PEEK[HEADER])
		innerFetch := fmt.Sprintf("UID %d FLAGS () INTERNALDATE %q RFC822.SIZE %d BODY[HEADER] {%d}\r\n%s", message.UID, internalDate, size, len(headerPeek), headerPeek)
		if err := s.SendNoCode(fmt.Sprintf("%d FETCH (%s)", message.UID, innerFetch)); err != nil {
			s.logger.Error("failed to send untagged FETCH response", "err", err, "message_uid", message.UID)
			return true
		}
	}

	s.Reply(tag, imap.OkCode, "FETCH Completed")
	return false
}

// Send sends a line as a untagged response
func (s *imapServerSession) Send(code imap.ResponseCode, args string) error {
	reply := fmt.Sprintf("* %s", code)
	if args != "" {
		reply += fmt.Sprintf(" %s", args)
	}
	s.ExtendDeadline(time.Second * 10)
	return s.conn.sendRaw(reply)
}

func (s *imapServerSession) SendNoCode(args string) error {
	if args == "" {
		return errors.New("args can not be empty for SendNoCode")
	}

	s.ExtendDeadline(time.Second * 10)
	return s.conn.sendRaw(fmt.Sprintf("* %s", args))
}

// Reply replies to the client with a tagged response
func (s *imapServerSession) Reply(tag string, code imap.ResponseCode, args string) error {
	reply := fmt.Sprintf("%s %s", tag, code)
	if args != "" {
		reply += fmt.Sprintf(" %s", args)
	}
	s.ExtendDeadline(time.Second * 10)
	return s.conn.sendRaw(reply)
}

func (s *imapServerSession) ContinueCommand(args string) error {
	reply := fmt.Sprintf("+")
	if args != "" {
		reply += fmt.Sprintf(" %s", args)
	}
	s.ExtendDeadline(time.Second * 10)
	return s.conn.sendRaw(reply)
}

func (s *imapServerSession) UpgradeToTLS() {
	// Wrap the current connection with a TLS server
	tlsServer := tls.Server(s.conn.net, &tls.Config{
		Certificates: []tls.Certificate{
			s.tlsCertificate,
		},
	})
	if err := tlsServer.Handshake(); err != nil {
		panic(err)
	}
	s.conn.net = tlsServer
	s.conn.reader = bufio.NewReader(s.conn.net)
	s.conn.isTLSSessionActive = true
}
