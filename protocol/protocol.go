package protocol

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/roosterfish/dcc-ex-go/command"
	"go.bug.st/serial"
	"golang.org/x/sys/unix"
)

type Observation struct{}
type ObservationsC chan Observation
type CommandC chan *command.Command
type WriteF func(*command.Command) error
type CleanupF func()

type Protocol struct {
	port                     serial.Port
	subscriptions            map[string]CommandC
	commandSubscriptions     map[string]map[string]ObservationsC
	listenerExit             chan bool
	subscriptionLock         sync.Mutex
	commandSubscriptionsLock sync.Mutex
	writeLock                sync.Mutex
	exclusiveReadLock        sync.Mutex
}

type Reader interface {
	Read() (CommandC, CleanupF)
	ReadCommand(command *command.Command) (ObservationsC, CleanupF)
	ReadExclusive(command *command.Command) (ObservationsC, CleanupF)
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

func NewProtocol(port serial.Port) *Protocol {
	protocol := &Protocol{
		port:                 port,
		subscriptions:        make(map[string]CommandC),
		commandSubscriptions: make(map[string]map[string]ObservationsC),
		listenerExit:         make(chan bool),
	}

	go protocol.listen()
	return protocol
}

func (p *Protocol) listen() {
	// The protocol's Close is waiting for the channel to be closed.
	defer close(p.listenerExit)

	bufStr := ""
	buf := make([]byte, 100)
	for {
		n, err := p.port.Read(buf)
		if err != nil {
			return
		}

		// Not every read operation contains already the full command.
		// Check if the read string contains a newline in which case
		// the command got fully read and we can continue.
		bufStr += string(buf[:n])

		// A command might be built from several read operations.
		// Also multiple commands could be read in a single operation.
		// Iterate over the bufStr as long as it contains a newline.
		for strings.Contains(bufStr, "\n") {
			// Cut the first command.
			// The next loop will potentially cut off the next command.
			// Therefore only split the string once.
			split := strings.SplitN(bufStr, "\n", 2)
			if len(split) != 2 {
				continue
			}

			// Append the remainder of the string to be checked
			// in the next iteration.
			bufStr = split[1]

			command, err := command.NewCommandFromString(split[0])
			if err != nil {
				continue
			}

			p.commandSubscriptionsLock.Lock()
			for _, subscriberC := range p.commandSubscriptions[split[0]] {
				subscriberC <- Observation{}
			}

			p.commandSubscriptionsLock.Unlock()

			p.subscriptionLock.Lock()
			for _, subscriberC := range p.subscriptions {
				subscriberC <- command
			}

			p.subscriptionLock.Unlock()
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

// TODO: not really exclusive if there are already callers on Read()
func (p *Protocol) ReadExclusive(command *command.Command) (ObservationsC, CleanupF) {
	// Acquire the exclusive read lock
	p.exclusiveReadLock.Lock()
	observationsC, cleanupF := p.ReadCommand(command)
	return observationsC, func() {
		cleanupF()

		// Release the exclusive read lock only after the caller cleaned up.
		p.exclusiveReadLock.Unlock()
	}
}

func (p *Protocol) Write(command *command.Command) error {
	p.writeLock.Lock()
	defer p.writeLock.Unlock()

	_, err := p.port.Write(command.Bytes())
	if errors.Is(err, unix.EBADF) {
		return fmt.Errorf("Serial port is closed")
	}

	return err
}

func (p *Protocol) Close() error {
	err := p.port.Close()
	if err != nil {
		return fmt.Errorf("Failed to close serial port: %w", err)
	}

	<-p.listenerExit
	return nil
}
