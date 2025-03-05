package sensor

import (
	"context"
	"fmt"
	"sync"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

type ID uint16
type State command.OpCode
type VPin int
type PullUp uint8

const (
	StateActive   State = 'Q'
	StateInactive State = 'q'
)

const (
	PullUpOff PullUp = iota
	PullUpOn
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
	observed, cleanup := s.protocol.ReadCommand(command.NewCommand(state.OpCode(), "%d", s.id))
	defer cleanup()

	select {
	case <-observed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Sensor) Watch(state State) (protocol.ObservationsC, protocol.CleanupF) {
	return s.protocol.ReadCommand(command.NewCommand(state.OpCode(), "%d", s.id))
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

// Persist creates the sensor and persists its definition in the EEPROM.
func (s *Sensor) Persist(ctx context.Context, vpin VPin, pullUp PullUp) error {
	err := s.protocol.Write(command.NewCommand(command.OpCodeSensorCreate, "%d %d %d", s.id, vpin, pullUp))
	if err != nil {
		return fmt.Errorf("failed to persist sensor: %w", err)
	}

	commandC, cleanupF := s.protocol.ReadOpCode(command.OpCodeSuccess)
	defer cleanupF()

	go func() {
		_ = s.protocol.Write(command.NewCommand(command.OpCodeEEPROM, ""))
	}()

	select {
	case <-commandC:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
