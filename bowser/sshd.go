package bowser

import (
	_ "bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/kr/pty"
	"github.com/pquerna/otp/totp"
	"github.com/satori/go.uuid"
	"github.com/uber-go/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

type AccountKey struct {
	Account *Account
	Key     ssh.PublicKey
	Comment string
	Options []string
}

func NewAccountKey(account *Account, rawKey []byte) (*AccountKey, error) {
	key, comment, options, _, err := ssh.ParseAuthorizedKey(rawKey)
	if err != nil {
		return nil, err
	}

	return &AccountKey{
		Account: account,
		Key:     key,
		Comment: comment,
		Options: options,
	}, nil
}

func (key *AccountKey) ID() string {
	return string(key.Key.Marshal())
}

type SSHSession struct {
	UUID          string
	State         *SSHDState
	Account       *Account
	Conn          *ssh.ServerConn
	RecordingFile *os.File
	log           zap.Logger
}

func NewSSHSession(state *SSHDState, conn *ssh.ServerConn) *SSHSession {
	id := uuid.NewV4()

	strID, _ := id.MarshalText()
	path := state.Config.RecordingPath + string(strID) + ".rec"
	file, err := os.Create(path)

	// This is ok, we null check below
	if err != nil {
		state.log.Warn("Couldn't create recording file", zap.Error(err), zap.String("path", path))
	}

	return &SSHSession{
		UUID:          string(strID),
		State:         state,
		Account:       state.accounts[conn.User()],
		Conn:          conn,
		RecordingFile: file,
		log:           state.log,
	}
}

type SSHDState struct {
	Config *Config

	log      zap.Logger
	accounts map[string]*Account
	keys     map[string]*AccountKey
}

func NewSSHDState() *SSHDState {
	config, err := LoadConfig("config.json")

	if err != nil {
		log.Panicf("Failed to load config: %v", err)
	}

	f, err := os.Create(config.LogFile)

	if err != nil {
		log.Panicf("Failed to open logfile: %v", err)
	}

	state := SSHDState{
		Config: config,
		log: zap.New(
			zap.NewJSONEncoder(),
			zap.Output(f),
		),
	}

	// Ensure the logpath exists
	os.Mkdir(state.Config.RecordingPath, 0770)

	state.reloadAccounts()
	return &state
}

func (s *SSHDState) reloadAccounts() {
	rawAccounts, err := LoadAccounts(s.Config.Accounts)
	if err != nil {
		s.log.Error("Failed to load accounts", zap.Error(err))
		return
	}

	accounts := make(map[string]*Account)
	keys := make(map[string]*AccountKey)

	for _, account := range rawAccounts {
		if _, exists := accounts[account.Username]; exists {
			s.log.Error("Duplicate username", zap.String("username", account.Username))
			return
		}

		accounts[account.Username] = &account

		for _, key := range account.SSHKeysRaw {
			key, err := NewAccountKey(&account, []byte(key))
			if err != nil {
				s.log.Warn(
					"Skipping key for account, couldn't parse",
					zap.Error(err),
					zap.Object("account", account))
				continue
			}

			other, exists := keys[key.ID()]
			if exists {
				s.log.Error("Duplicate key", zap.Object("account a", other.Account), zap.Object("account b", account))
				return
			}

			keys[key.ID()] = key
		}

	}

	s.accounts = accounts
	s.keys = keys
}

var badKeyError = fmt.Errorf("This is not the castle you are looking for...")

func (s *SSHDState) Run() {
	sshConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			accountKey, exists := s.keys[string(key.Marshal())]

			// If we don't have that key, just gtfo
			if !exists {
				return nil, badKeyError
			}

			if conn.User() != accountKey.Account.Username {
				return nil, badKeyError
			}

			return nil, nil
		},
	}

	// You can generate a keypair with 'ssh-keygen -t rsa'
	privateBytes, err := ioutil.ReadFile(s.Config.IDRSA)
	if err != nil {
		log.Fatalf("Failed to load private key (%v)", s.Config.IDRSA)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatal("Failed to parse private key")
	}

	sshConfig.AddHostKey(private)

	// Once a ServerConfig has been configured, connections can be accepted.
	listener, err := net.Listen("tcp", s.Config.Bind)
	if err != nil {
		log.Fatalf("Failed to listen on 2200 (%s)", err)
	}

	// Accept all connections
	log.Printf("Listening on %v", s.Config.Bind)
	for {
		tcpConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept incoming connection (%s)", err)
			continue
		}

		// Before use, a handshake must be performed on the incoming net.Conn.
		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, sshConfig)
		if err != nil {
			s.log.Warn(
				"Failed to handshake",
				zap.String("user", sshConn.User()),
				zap.String("remote", sshConn.RemoteAddr().String()),
				zap.Error(err))
			continue
		}

		session := NewSSHSession(s, sshConn)

		s.log.Info(
			"New SSH connection",
			zap.String("remote", sshConn.RemoteAddr().String()),
			zap.String("version", string(sshConn.ClientVersion())))

		// Discard all global out-of-band Requests
		go ssh.DiscardRequests(reqs)

		// Run the core loop which handles channels
		go session.handleChannels(chans)
	}
}

func (s *SSHSession) handleChannels(chans <-chan ssh.NewChannel) {
	// Service the incoming Channel channel in go routine
	for newChannel := range chans {
		go s.handleChannel(newChannel)
	}
}

func (s *SSHSession) handleChannel(newChannel ssh.NewChannel) {
	if t := newChannel.ChannelType(); t != "session" {
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("unknown channel type: %s", t))
		return
	}

	connection, requests, err := newChannel.Accept()
	if err != nil {
		s.log.Warn("Could not accept channel", zap.Error(err))
		return
	}

	term := terminal.NewTerminal(connection, "")
	term.Write([]byte(fmt.Sprintf("Session %v opened\r\n", s.UUID)))
	term.Write([]byte(s.State.Config.MOTD + "\r\n"))

	// TODO: Timeout here

	// Query and validate MFA
	valid := false

	for i := 0; i < 3; i++ {
		term.Write([]byte("MFA Code: "))
		line, err := term.ReadLine()

		if err != nil {
			break
		}

		if totp.Validate(line, s.Account.MFA.TOTP) {
			valid = true
			break
		}
	}

	// Close connection if its not valid
	if !valid {
		connection.Close()
		return
	}

	// Start shell session
	bash := exec.Command(s.Account.Shell)

	// Prepare teardown function
	close := func() {
		connection.Close()
		_, err := bash.Process.Wait()
		if err != nil {
			s.log.Warn(
				"Failed to exit shell",
				zap.Error(err),
				zap.String("uuid", s.UUID))
		}

		s.log.Info("session and shell closed", zap.String("uuid", s.UUID))
	}

	// Allocate a terminal for this channel
	s.log.Info("shell created", zap.String("uuid", s.UUID), zap.String("shell", s.Account.Shell))
	bashf, err := pty.Start(bash)
	if err != nil {
		s.log.Warn("could not start pty", zap.String("uuid", s.UUID), zap.Error(err))
		close()
		return
	}

	// Handle sending PTY data to the session while also logging to file
	var once sync.Once
	go func() {
		var err error
		var size int
		buffer := make([]byte, 1024)

		for err != io.EOF {
			size, err = connection.Read(buffer)
			bashf.Write(buffer[:size])
		}

		once.Do(close)
	}()

	go func() {
		var err error
		var size int
		buffer := make([]byte, 1024)

		for err != io.EOF {
			size, err = bashf.Read(buffer)
			connection.Write(buffer[:size])

			if s.RecordingFile != nil {
				s.RecordingFile.Write(buffer[:size])
			}
		}

		once.Do(close)
	}()

	// Sessions have out-of-band requests such as "shell", "pty-req" and "env"
	go func() {
		for req := range requests {
			switch req.Type {
			case "shell":
				// We only accept the default shell
				// (i.e. no command in the Payload)
				if len(req.Payload) == 0 {
					req.Reply(true, nil)
				}
			case "pty-req":
				termLen := req.Payload[3]
				w, h := parseDims(req.Payload[termLen+4:])
				SetWinsize(bashf.Fd(), w, h)
				// Responding true (OK) here will let the client
				// know we have a pty ready for input
				req.Reply(true, nil)
			case "window-change":
				w, h := parseDims(req.Payload)
				SetWinsize(bashf.Fd(), w, h)
			}
		}
	}()
}

// =======================

// parseDims extracts terminal dimensions (width x height) from the provided buffer.
func parseDims(b []byte) (uint32, uint32) {
	w := binary.BigEndian.Uint32(b)
	h := binary.BigEndian.Uint32(b[4:])
	return w, h
}

// ======================

// Winsize stores the Height and Width of a terminal.
type Winsize struct {
	Height uint16
	Width  uint16
	x      uint16 // unused
	y      uint16 // unused
}

// SetWinsize sets the size of the given pty.
func SetWinsize(fd uintptr, w, h uint32) {
	ws := &Winsize{Width: uint16(w), Height: uint16(h)}
	syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
}
