package channel

import (
	"context"
	"fmt"
	"sync"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"golang.org/x/sync/errgroup"
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

// SessionSuccess is equal to Session including an additional reader that checks for the failure op code.
// If any invalid command is sent throughout the session this likely indicates a wrong command was sent.
// An example is examining the status of an invalid turnout.
func (c *Channel) SessionSuccess(ctx context.Context, sessionF func(ctx context.Context, protocol protocol.ReadWriteCloser) error) error {
	c.sessionLock.Lock()
	defer c.sessionLock.Unlock()

	// Allows to early cancel all other routines in the errgroup.
	// This is required in case the sessionF returned with success so we can also cancel the failure op code reader.
	earlyCancelCtx, earlyCancel := context.WithCancel(ctx)

	g, ctx := errgroup.WithContext(earlyCancelCtx)
	g.Go(func() error {
		_, err := c.protocol.ReadOpCode(ctx, command.OpCodeFail)
		if err != nil {
			return err
		}

		return fmt.Errorf("observed session failure after last command %q", c.protocol.lastCommand().String())
	})

	g.Go(func() error {
		err := sessionF(ctx, c.protocol)
		if err == nil {
			// Cancel the failure op code reader.
			earlyCancel()
		}

		return err
	})

	err := g.Wait()

	// Only return an error if it isn't caused by an early context cancellation.
	if err != nil && earlyCancelCtx.Err() == nil {
		return err
	}

	return nil
}

// RSession allows having a short-term read-only session on the connection's channel to interact
// with the underlying protocol.
// Unlike Session it only allows reading.
// It allows multiple concurrent reader sessions independent whether or not there is an active rw session.
func (c *Channel) RSession(sessionF func(protocol protocol.Reader) error) error {
	return sessionF(c.protocol)
}
