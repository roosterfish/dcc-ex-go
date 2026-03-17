package sensor

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"golang.org/x/sync/errgroup"
)

type ID uint16
type State command.OpCode
type VPin uint16
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
	id      ID
	channel *channel.Channel
}

func (s State) OpCode() command.OpCode {
	return command.OpCode(s)
}

func (s State) Opposite() State {
	if s == StateActive {
		return StateInactive
	}

	return StateActive
}

func NewSensor(id ID, channel *channel.Channel) *Sensor {
	return &Sensor{
		id:      id,
		channel: channel,
	}
}

func (s *Sensor) Wait(ctx context.Context, state State) error {
	return s.channel.RSession(func(protocol protocol.Reader) error {
		return protocol.ReadCommand(ctx, command.NewCommand(state.OpCode(), "%d", s.id))
	})
}

// WaitConsistent waits until the sensor's new state was unchanged for at least the given duration.
// This helps waiting for sensors (e.g. block detection) whose values flicker during the transition period.
// In case the sensor already has the given state, it will start waiting immediately.
// In case the sensor has a different state, it will wait until the expected state is observed for the first time.
func (s *Sensor) WaitConsistent(ctx context.Context, state State, duration time.Duration) error {
	// First read the sensors current state.
	// It might be that the sensor doesn't receive any state change during the wait duration.
	sensorState, err := s.State(ctx)
	if err != nil {
		return err
	}

	// If the current sensor state isn't what we are waiting for,
	// reset the duration so we only start waiting once the expected status is observed for the first time.
	startDuration := duration
	if sensorState != state {
		startDuration = 0
	}

	// Create a new timer without any duration.
	timer := time.NewTimer(startDuration)
	defer timer.Stop()

	// As the timer could be created without duration, in this case it will expire right away.
	// Read the expiry time from the channel so it's clean.
	if startDuration == 0 {
		<-timer.C
	}

	return s.channel.RSession(func(protocol protocol.Reader) error {
		commandC, cleanupF := protocol.Read()
		defer cleanupF()

		stateCommand := command.NewCommand(state.OpCode(), "%d", s.id).String()
		oppositeStateCommand := command.NewCommand(state.Opposite().OpCode(), "%d", s.id).String()

		for {
			select {
			case cmd := <-commandC:
				cmdStr := cmd.String()
				if cmdStr == stateCommand {
					// In case the requested state was observed reset the expired timer.
					_ = timer.Reset(duration)
				} else if cmdStr == oppositeStateCommand {
					// In case the opposite state was observed stop the timer.
					_ = timer.Stop()
				}
			case <-timer.C:
				// In case the timer expired return.
				return nil
			case <-ctx.Done():
				// If the outer context expires return the error.
				return ctx.Err()
			}
		}
	})
}

func (s *Sensor) SetCallback(state State, f func(id ID, state State)) protocol.CleanupF {
	wg := sync.WaitGroup{}

	ctx, cancel := context.WithCancel(context.Background())

	watcher := func() {
		defer wg.Done()

		wgInner := sync.WaitGroup{}

		_ = s.channel.RSession(func(protocol protocol.Reader) error {
			commandC, cleanupF := protocol.Read()
			defer cleanupF()

			stateCommand := command.NewCommand(state.OpCode(), "%d", s.id)

			for {
				select {
				case cmd := <-commandC:
					if cmd.String() == stateCommand.String() {
						// Ensure the callback is always executed in its own routine.
						// This is essential to detach from the protocols read loop.
						wgInner.Add(1)
						go func() {
							defer wgInner.Done()

							f(s.id, state)
						}()
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		})

		wgInner.Wait()
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
	return s.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		err := protocol.Write(command.NewCommand(command.OpCodeSensorCreate, "%d %d %d", s.id, vpin, pullUp))
		if err != nil {
			return fmt.Errorf("failed to create sensor: %w", err)
		}

		g, ctx := errgroup.WithContext(ctx)

		// Ensure there is a reader before writing.
		// Use the errgroup's context as we later wait for the commandC in a routine.
		waiter := protocol.ReadOpCode(ctx, command.OpCodeSuccess)

		g.Go(func() error {
			<-waiter.WaitC
			return nil
		})

		g.Go(func() error {
			return protocol.Write(command.NewCommand(command.OpCodeEEPROM, ""))
		})

		return g.Wait()
	})
}

func (s *Sensor) Active(ctx context.Context) bool {
	sensorState, err := s.State(ctx)
	if err != nil {
		return false
	}

	return sensorState == StateActive
}

func (s *Sensor) State(ctx context.Context) (State, error) {
	sensorState := StateInactive

	err := s.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		commandC, cleanupF := protocol.Read()
		defer cleanupF()

		// Send a control command to allow waiting for the end of the output.
		err := protocol.Write(command.NewControlCommand(StateActive.OpCode(), ""))
		if err != nil {
			return err
		}

		for {
			select {
			case cmd := <-commandC:
				// Loop through the received commands until observing the end of the output.
				// This is indicated by the fail opcode as we have sent an invalid control command.
				if cmd.OpCode() == command.OpCodeFail {
					return nil
				}

				if cmd.OpCode() == StateActive.OpCode() {
					params, err := cmd.ParametersStrings()
					if err == nil && len(params) == 1 && params[0] == strconv.FormatUint(uint64(s.id), 10) {
						sensorState = StateActive

						// Don't yet return as there might be more commands.
						// Only return once the failure op code is observed.
						// Otherwise we might leak the failure op code into the following session.
						continue
					}
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
	if err != nil {
		return sensorState, err
	}

	return sensorState, nil
}
