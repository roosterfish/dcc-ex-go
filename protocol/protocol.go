package protocol

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/roosterfish/dcc-ex-go/command"
	"go.bug.st/serial"
)

type Observation struct{}
type ObservationsC chan Observation
type CleanupF func()

type Protocol struct {
	port              serial.Port
	reader            func()
	subscriptions     map[string]map[string]chan bool
	listenerExit      chan bool
	subscriptionLock  sync.Mutex
	writeLock         sync.Mutex
	exclusiveReadLock sync.Mutex
}

type Reader interface {
	Read(command *command.Command) (ObservationsC, CleanupF)
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
		port:          port,
		subscriptions: make(map[string]map[string]chan bool),
		listenerExit:  make(chan bool),
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
		if strings.Contains(bufStr, "\n") {
			command, _ := command.ParseRaw(bufStr)

			p.subscriptionLock.Lock()
			for _, subscriber := range p.subscriptions[command] {
				// Try to write non blocking.
				select {
				case subscriber <- true:
				default:
				}
			}

			p.subscriptionLock.Unlock()
			bufStr = ""
		}
	}
}

func (p *Protocol) Read(command *command.Command) (ObservationsC, CleanupF) {
	commandStr := command.StringRaw()

	// Create a map for subscriptions specific to the given command in case it doesn't yet exist.
	p.subscriptionLock.Lock()
	_, ok := p.subscriptions[commandStr]
	if !ok {
		p.subscriptions[commandStr] = make(map[string]chan bool)
	}

	// In order to easily identify the caller in the subscription map create an UUID.
	uuid := uuid.NewString()

	// Create the caller's subscription channel and insert it into the pool.
	subscription := make(chan bool)
	commandSubscriptions := p.subscriptions[commandStr]
	commandSubscriptions[uuid] = subscription
	p.subscriptions[commandStr] = commandSubscriptions
	p.subscriptionLock.Unlock()

	// Create the channel returned to the caller which receives messages
	// in case the subscribed command got observed.
	observed := make(ObservationsC)

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
				observed <- struct{}{}
			case <-ctx.Done():
				// Initiate cleanup as requested by the caller.
				close(observed)
				return
			}
		}
	}()

	// The cleanup function is returned to the caller and ensures the
	// routine has returned, the channels are closed and the subscription is removed.
	cleanup := func() {
		// Cancels the routine which causes the observation channel to be closed too.
		cancel()
		wg.Wait()

		// Now the subscription channel can also be closed.
		close(subscription)

		p.subscriptionLock.Lock()
		delete(p.subscriptions[commandStr], uuid)
		p.subscriptionLock.Unlock()
	}

	return observed, cleanup
}

// TODO: not really exclusive if there are already callers on Read()
func (p *Protocol) ReadExclusive(command *command.Command) (ObservationsC, CleanupF) {
	// Acquire the exclusive read lock
	p.exclusiveReadLock.Lock()
	observationsC, cleanupF := p.Read(command)
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
	return err
}

func (p *Protocol) Close() error {
	err := p.port.Close()
	if err != nil {
		return fmt.Errorf("Failed to close protocol: %w", err)
	}

	<-p.listenerExit
	return nil
}
