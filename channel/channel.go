package channel

import (
	"sync"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

// readWriteCloserCache wraps the protocols readWriteCloser and allows caching the written commands.
type readWriteCloserCache struct {
	protocol.ReadWriteCloser
	commandCache command.Command
}

// Write caches the written command and calls the wrapped readWriteCloser's Write.
func (r *readWriteCloserCache) Write(command *command.Command) error {
	// Don't copy the pointer to ensure we don't influence the underlying protocol.
	r.commandCache = *command
	return r.ReadWriteCloser.Write(command)
}

// LastCommand returns the last written command from the cache.
func (r *readWriteCloserCache) lastCommand() *command.Command {
	return &r.commandCache
}

type Channel struct {
	protocol    *readWriteCloserCache
	sessionLock sync.Mutex
}

// NewChannel returns a new channel using the given protocol.
func NewChannel(protocol protocol.ReadWriteCloser) *Channel {
	return &Channel{
		protocol: &readWriteCloserCache{ReadWriteCloser: protocol},
	}
}

// Session allows having a short-term session on the connection's channel to interact
// with the underlying protocol.
// There can only be a single session at a time.
// Session is thread safe and allows exclusive read and write from and to the channel.
func (c *Channel) Session(sessionF func(protocol protocol.ReadWriteCloser) error) error {
	c.sessionLock.Lock()
	defer c.sessionLock.Unlock()

	return sessionF(c.protocol)
}

// RSession allows having a short-term read-only session on the connection's channel to interact
// with the underlying protocol.
// Unlike Session it only allows reading.
// It allows multiple concurrent reader sessions independent whether or not there is an active rw session.
func (c *Channel) RSession(sessionF func(protocol protocol.Reader) error) error {
	return sessionF(c.protocol)
}
