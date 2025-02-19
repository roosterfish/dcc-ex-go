package sensor

import (
	"context"
	"sync"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

type ID uint16
type State command.OpCode

const (
	StateActive   State = 'Q'
	StateInactive State = 'q'
)

type Sensor struct {
	id       ID
	protocol protocol.ReadWriteCloser
}

func (s State) OpCode() command.OpCode {
	return command.OpCode(s)
}

func NewSensor(id ID, protocol protocol.ReadWriteCloser) *Sensor {
	return &Sensor{
		id:       id,
		protocol: protocol,
	}
}

func (s *Sensor) Wait(ctx context.Context, state State) error {
	observed, cleanup := s.protocol.Read(command.NewCommand(state.OpCode(), "%d", s.id))
	defer cleanup()

	select {
	case <-observed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Sensor) Watch(state State) (protocol.ObservationsC, protocol.CleanupF) {
	return s.protocol.Read(command.NewCommand(state.OpCode(), "%d", s.id))
}

func (s *Sensor) SetCallback(state State, f func(id ID, state State)) protocol.CleanupF {
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}

	ctx, cancel := context.WithCancel(context.Background())

	watcher := func() {
		defer wg.Done()

		for {
			err := s.Wait(ctx, state)
			if err != nil {
				return
			}

			lock.Lock()
			f(s.id, state)
			lock.Unlock()
		}
	}

	wg.Add(1)
	go watcher()

	return func() {
		cancel()
		wg.Wait()
	}
}
