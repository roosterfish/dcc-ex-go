package channel

import (
	"context"
	"sync"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

const sessionProtocolCtxKey = "session-protocol"

type WriteF func(ctx context.Context, command *command.Command) error

type Channel struct {
	protocol    protocol.ReadWriteCloser
	sessionLock sync.Mutex
}

// NewChannel returns a new channel using the given protocol.
func NewChannel(protocol protocol.ReadWriteCloser) *Channel {
	return &Channel{
		protocol: protocol,
	}
}

// Consider using the channel abstraction functions instead as those perform additional control command handling to gate
// the beginning and end of a session and can ensure that no response is leaked into follow-up sessions.
//
// Session allows having a short-term session on the connection's channel to interact with the underlying protocol.
// There can only be a single session at a time.
// Session is thread safe and allows exclusive read and write from and to the channel.
// There can be other read sessions in parallel.
func (c *Channel) Session(sessionF func(protocol protocol.ReadWriteCloser) error) error {
	c.sessionLock.Lock()
	defer c.sessionLock.Unlock()

	return sessionF(c.protocol)
}

// SessionContext allows using a session to run multiple commands exclusively in succession.
// The context passed into function f contains the channels underlying protocol.
// Calling any of the other channel abstraction functions within function f ensures the session is derived
// from the passed context.
// With this, atomic operations can be implemented which first require reading some content and then performing
// an action based on the read values.
func (c *Channel) SessionContext(ctx context.Context, f func(ctx context.Context) error) error {
	c.sessionLock.Lock()
	defer c.sessionLock.Unlock()

	ctx = context.WithValue(ctx, sessionProtocolCtxKey, c.protocol)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return f(ctx)
}

// RSession allows having a short-term read-only session on the connection's channel to interact with the underlying protocol.
// Unlike Session it only allows reading.
// It allows multiple concurrent reader sessions independent whether or not there is an active read and write session.
func (c *Channel) RSession(sessionF func(protocol protocol.Reader) error) error {
	return sessionF(c.protocol)
}
