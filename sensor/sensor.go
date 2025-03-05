package sensor

import (
	"context"
	"fmt"
	"sync"
	"time"

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

func (s *Sensor) Watch(state State) (protocol.ObservationsC, protocol.CleanupF) {
	return s.protocol.ReadCommand(command.NewCommand(state.OpCode(), "%d", s.id))
}

func (s *Sensor) Wait(ctx context.Context, state State) error {
	observed, cleanup := s.Watch(state)
	defer cleanup()

	select {
	case <-observed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitConsistent waits until the sensor's new state was unchanged for at least the given duration.
// This helps waiting for sensors (e.g. block detection) whose values flicker during the transition period.
func (s *Sensor) WaitConsistent(ctx context.Context, state State, duration time.Duration) error {
	observedC, cleanupF := s.Watch(state)
	defer cleanupF()

	observedOppositeC, cleanupF := s.Watch(state.Opposite())
	defer cleanupF()

	// Create a new timer without any duration.
	timer := time.NewTimer(0)
	defer timer.Stop()

	// As the timer was created without duration it will expire right away.
	// Read the expiry time from the channel so it's clean.
	<-timer.C

	for {
		select {
		case <-observedC:
			// In case the requested state was observed reset the expired timer.
			_ = timer.Reset(duration)
		case <-observedOppositeC:
			// In case the opposite state was observed stop the timer.
			_ = timer.Stop()
		case <-timer.C:
			// In case the timer expired return.
			return nil
		case <-ctx.Done():
			// If the outer context expires return the error.
			return ctx.Err()
		}
	}
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
