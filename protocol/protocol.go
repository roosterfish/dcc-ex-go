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

type Protocol struct {
	config                   *Config
	port                     io.ReadWriteCloser
	subscriptions            map[string]CommandC
	commandSubscriptions     map[string]map[string]ObservationsC
	opCodeSubscriptions      map[string]map[string]CommandC
	firstSubscriberF         func()
	listenerExitC            chan bool
	subscriptionLock         sync.Mutex
	commandSubscriptionsLock sync.Mutex
	opCodeSubscriptionsLock  sync.Mutex
	writeLock                sync.Mutex
}

type Reader interface {
	Read() (CommandC, CleanupF)
	ReadCommand(command *command.Command) (ObservationsC, CleanupF)
	ReadOpCode(opCode command.OpCode) (CommandC, CleanupF)
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
		config:               config,
		port:                 port,
		subscriptions:        make(map[string]CommandC),
		commandSubscriptions: make(map[string]map[string]ObservationsC),
		opCodeSubscriptions:  make(map[string]map[string]CommandC),
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

		p.commandSubscriptionsLock.Lock()
		for _, subscriberC := range p.commandSubscriptions[command.String()] {
			subscriberC <- Observation{}
		}

		p.commandSubscriptionsLock.Unlock()

		p.opCodeSubscriptionsLock.Lock()
		for _, subscriberC := range p.opCodeSubscriptions[string(command.OpCode())] {
			subscriberC <- command
		}

		p.opCodeSubscriptionsLock.Unlock()

		p.subscriptionLock.Lock()
		for _, subscriberC := range p.subscriptions {
			subscriberC <- command
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
	subscription := make(CommandC)
	p.subscriptions[uuid] = subscription
	p.subscriptionLock.Unlock()

	// Unlock the listener as at least one subscriber is active.
	p.firstSubscriberF()

	commandC := make(CommandC)

	// Create a new context to allow cancellation of the routine.
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case command := <-subscription:
				// Send the command to the caller
				commandC <- command
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

		// Close the returned observation channel.
		// The routine cannot anymore write to it as it has already returned.
		close(commandC)

		p.subscriptionLock.Lock()
		// Now the subscription channel can also be closed.
		// This can only be done after obtaining the lock to protect the listener
		// from writing to a closed subscription channel.
		close(subscription)

		delete(p.subscriptions, uuid)
		p.subscriptionLock.Unlock()
	}

	return commandC, cleanup
}

func (p *Protocol) ReadCommand(command *command.Command) (ObservationsC, CleanupF) {
	commandStr := command.String()

	// Create a map for subscriptions specific to the given command in case it doesn't yet exist.
	p.commandSubscriptionsLock.Lock()
	_, ok := p.commandSubscriptions[commandStr]
	if !ok {
		p.commandSubscriptions[commandStr] = make(map[string]ObservationsC)
	}

	// Unlock the listener as at least one subscriber is active.
	p.firstSubscriberF()

	// In order to easily identify the caller in the subscription map create an UUID.
	uuid := uuid.NewString()

	// Create the caller's subscription channel and insert it into the pool.
	subscription := make(ObservationsC)
	commandSubscriptions := p.commandSubscriptions[commandStr]
	commandSubscriptions[uuid] = subscription
	p.commandSubscriptions[commandStr] = commandSubscriptions
	p.commandSubscriptionsLock.Unlock()

	// Create the channel returned to the caller which receives messages
	// in case the subscribed command got observed.
	observedC := make(ObservationsC)

	// Create a new context to allow cancellation of the routine.
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-subscription:
				// Notify the caller that the command was observed.
				observedC <- Observation{}
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

		// Close the returned observation channel.
		// The routine cannot anymore write to it as it has already returned.
		close(observedC)

		p.commandSubscriptionsLock.Lock()
		// Now the subscription channel can also be closed.
		// This can only be done after obtaining the lock to protect the listener
		// from writing to a closed subscription channel.
		close(subscription)

		delete(p.commandSubscriptions[commandStr], uuid)
		p.commandSubscriptionsLock.Unlock()
	}

	return observedC, cleanup
}

func (p *Protocol) ReadOpCode(opCode command.OpCode) (CommandC, CleanupF) {
	opCodeStr := string(opCode)

	// Create a map for subscriptions specific to the given command in case it doesn't yet exist.
	p.opCodeSubscriptionsLock.Lock()
	_, ok := p.opCodeSubscriptions[opCodeStr]
	if !ok {
		p.opCodeSubscriptions[opCodeStr] = make(map[string]CommandC)
	}

	// Unlock the listener as at least one subscriber is active.
	p.firstSubscriberF()

	// In order to easily identify the caller in the subscription map create an UUID.
	uuid := uuid.NewString()

	// Create the caller's subscription channel and insert it into the pool.
	subscription := make(CommandC)
	opCodeSubscriptions := p.opCodeSubscriptions[opCodeStr]
	opCodeSubscriptions[uuid] = subscription
	p.opCodeSubscriptions[opCodeStr] = opCodeSubscriptions
	p.opCodeSubscriptionsLock.Unlock()

	// Create the channel returned to the caller which receives messages
	// in case the subscribed command got observed.
	commandC := make(CommandC)

	// Create a new context to allow cancellation of the routine.
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case command := <-subscription:
				// Notify the caller that the command was observed.
				commandC <- command
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

		// Close the returned observation channel.
		// The routine cannot anymore write to it as it has already returned.
		close(commandC)

		p.opCodeSubscriptionsLock.Lock()
		// Now the subscription channel can also be closed.
		// This can only be done after obtaining the lock to protect the listener
		// from writing to a closed subscription channel.
		close(subscription)

		delete(p.opCodeSubscriptions[opCodeStr], uuid)
		p.opCodeSubscriptionsLock.Unlock()
	}

	return commandC, cleanup
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
