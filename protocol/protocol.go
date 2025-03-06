package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/roosterfish/dcc-ex-go/command"
	"golang.org/x/sys/unix"
)

type Observation struct{}
type ObservationsC chan Observation
type CommandC chan *command.Command
type WriteF func(*command.Command) error
type CleanupF func()

type Config struct {
	RequireSubscriber bool
}

type Subscription struct {
	ingressC, egressC CommandC
	cancelledC        chan bool
}

type Protocol struct {
	config           *Config
	port             io.ReadWriteCloser
	subscriptions    map[string]*Subscription
	firstSubscriberF func()
	listenerExitC    chan bool
	subscriptionLock sync.Mutex
	writeLock        sync.Mutex
}

type Reader interface {
	Read() (CommandC, CleanupF)
	ReadCommand(ctx context.Context, command *command.Command) error
	ReadOpCode(ctx context.Context, opCode command.OpCode) (*command.Command, error)
}

type Writer interface {
	Write(command *command.Command) error
}

type Closer interface {
	Close() error
}

type ReadWriteCloser interface {
	Reader
	Writer
	Closer
}

func NewProtocol(port io.ReadWriteCloser, config *Config) *Protocol {
	firstSubscriber := make(chan bool)

	protocol := &Protocol{
		config:        config,
		port:          port,
		subscriptions: make(map[string]*Subscription),
		firstSubscriberF: sync.OnceFunc(func() {
			close(firstSubscriber)
		}),
		listenerExitC: make(chan bool),
	}

	go protocol.listen(firstSubscriber)
	return protocol
}

func (p *Protocol) listen(firstSubscriber chan bool) {
	// The protocol's Close is waiting for the channel to be closed.
	defer close(p.listenerExitC)

	notifyF := func(stringCommand string) {
		command, err := command.NewCommandFromString(stringCommand)
		if err != nil {
			return
		}

		p.subscriptionLock.Lock()
		for _, subscription := range p.subscriptions {
			select {
			case subscription.ingressC <- command:
				// Try writing the command to the subscriptions ingress channel.
			case <-subscription.cancelledC:
				// In case the subscription was cancelled, don't block trying to write.
			}
		}

		p.subscriptionLock.Unlock()
	}

	// Wait until the first subscriber is active.
	// This ensures the subscriber can always observe the ready info message.
	// The first subscriber closes the channel which unblocks belows statement.
	if p.config.RequireSubscriber {
		<-firstSubscriber
	}

	commandRunes := []rune{}
	commandReading := false
	buf := make([]byte, 100)
	for {
		n, err := p.port.Read(buf)
		if err != nil {
			return
		}

		for _, receivedByte := range buf[:n] {
			// The parsing of the commands is implemented according to
			// https://dcc-ex.com/reference/developers/api.html#appendix-b-suggested-parameter-parsing-sequence.
			receivedRune := rune(receivedByte)
			if receivedRune == '<' {
				commandReading = true
				continue
			}

			if receivedRune == '>' {
				notifyF(string(commandRunes))

				commandReading = false
				commandRunes = []rune{}
			}

			// Filter out newlines.
			if receivedRune == '\n' {
				continue
			}

			if commandReading {
				commandRunes = append(commandRunes, receivedRune)
			}
		}
	}
}

func (p *Protocol) Read() (CommandC, CleanupF) {
	// In order to easily identify the caller in the subscription map create an UUID.
	uuid := uuid.NewString()

	p.subscriptionLock.Lock()

	// Create the caller's subscription channel and insert it into the map.
	subscription := &Subscription{
		egressC:    make(CommandC),
		ingressC:   make(CommandC),
		cancelledC: make(chan bool),
	}

	p.subscriptions[uuid] = subscription
	p.subscriptionLock.Unlock()

	// Unlock the listener as at least one subscriber is active.
	p.firstSubscriberF()

	// Create a new context to allow cancellation of the routine.
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case cmd := <-subscription.ingressC:
				// Send the command to the caller.
				select {
				case subscription.egressC <- cmd:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				// Initiate cleanup as requested by the caller.
				return
			}
		}
	}()

	// The cleanup function is returned to the caller and ensures the
	// routine has returned, the channels are closed and the subscription is removed.
	cleanup := func() {
		// Cancels the routine.
		cancel()
		wg.Wait()

		// Close the returned command channel.
		// The routine cannot anymore write to it as it has already returned.
		close(subscription.egressC)

		// Cancel the subscription.
		// This ensures the listener is unblocked trying to write to the subscriptions ingress
		// channel because the caller already hang up and doesn't anymore consume from the egress channel.
		close(subscription.cancelledC)

		// Obtain the lock and cleanup the subscription.
		p.subscriptionLock.Lock()
		close(subscription.ingressC)
		delete(p.subscriptions, uuid)
		p.subscriptionLock.Unlock()
	}

	return subscription.egressC, cleanup
}

func (p *Protocol) ReadCommand(ctx context.Context, command *command.Command) error {
	commandC, cleanupF := p.Read()
	defer cleanupF()

	commandStr := command.String()

	for {
		select {
		case cmd := <-commandC:
			if cmd.String() == commandStr {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Protocol) ReadOpCode(ctx context.Context, opCode command.OpCode) (*command.Command, error) {
	commandC, cleanupF := p.Read()
	defer cleanupF()

	for {
		select {
		case cmd := <-commandC:
			if cmd.OpCode() == opCode {
				return cmd, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (p *Protocol) Write(command *command.Command) error {
	p.writeLock.Lock()
	defer p.writeLock.Unlock()

	_, err := p.port.Write(command.Bytes())
	if err != nil {
		if errors.Is(err, unix.EBADF) {
			return fmt.Errorf("serial port is closed")
		} else {
			return fmt.Errorf("failed to write command %q: %w", command.String(), err)
		}
	}

	return nil
}

func (p *Protocol) Close() error {
	err := p.port.Close()
	if err != nil {
		return fmt.Errorf("failed to close serial port: %w", err)
	}

	<-p.listenerExitC
	return nil
}
