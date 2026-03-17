package turnout

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"golang.org/x/sync/errgroup"
)

type ID uint16
type VPin uint16
type Position uint16
type Profile uint8

type TurnoutServo struct {
	id      ID
	channel *channel.Channel
}

type TurnoutServoStatus struct {
	VPin           VPin
	ThrownPosition Position
	ClosedPosition Position
	Profile        Profile
	State          State
}

const (
	ProfileInstant Profile = iota
	ProfileFast
	ProfileMedium
	ProfileSlow
	ProfileBounce
)

func NewTurnoutServo(id ID, channel *channel.Channel) *TurnoutServo {
	return &TurnoutServo{
		id:      id,
		channel: channel,
	}
}

// Persist creates the turnout and persists its definition in the EEPROM.
func (t *TurnoutServo) Persist(ctx context.Context, vpin VPin, thrownPos Position, closedPos Position, profile Profile) error {
	return t.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		err := protocol.Write(command.NewCommand(command.OpCodeTurnout, "%d SERVO %d %d %d %d", t.id, vpin, thrownPos, closedPos, profile))
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

func (t *TurnoutServo) setStateCommand(state State) *command.Command {
	return command.NewCommand(command.OpCodeTurnout, "%d %c", t.id, state)
}

func (t *TurnoutServo) equalsCommandParams(cmd *command.Command) (bool, error) {
	params, err := cmd.ParametersStrings()
	if err != nil {
		return false, fmt.Errorf("failed getting turnout servo command parameters: %w", err)
	}

	return len(params) == 2 && params[0] == strconv.FormatUint(uint64(t.id), 10), nil
}

// Throw throws the servo turnout.
// It first checks whether or not the turnout is already thrown.
// Keep in mind that the check and actual throw are not inside the same session.
func (t *TurnoutServo) Throw(ctx context.Context) error {
	// Check if already thrown.
	// There isn't a broadcast sent if the turnout is already thrown.
	status, err := t.Examine(ctx)
	if err != nil {
		return err
	}

	if status.State == StateThrown {
		return nil
	}

	stateCommand := t.setStateCommand(StateThrown)
	return t.channel.WriteAndReadOpCode(ctx, stateCommand, command.OpCodeTurnoutResponse, t.equalsCommandParams)
}

// Close closes the servo turnout.
// It first checks whether or not the turnout is already closed.
// Keep in mind that the check and actual close are not inside the same session.
func (t *TurnoutServo) Close(ctx context.Context) error {
	// Check if already closed.
	// There isn't a broadcast sent if the turnout is already closed.
	status, err := t.Examine(ctx)
	if err != nil {
		return err
	}

	if status.State == StateClosed {
		return nil
	}

	stateCommand := t.setStateCommand(StateClosed)
	return t.channel.WriteAndReadOpCode(ctx, stateCommand, command.OpCodeTurnoutResponse, t.equalsCommandParams)
}

// Examine returns the status of the servo.
func (t *TurnoutServo) Examine(ctx context.Context) (*TurnoutServoStatus, error) {
	var responseCommand *command.Command
	err := t.channel.SessionSuccess(ctx, func(ctx context.Context, protocol protocol.ReadWriteCloser) error {
		waiter := protocol.ReadOpCode(ctx, command.OpCodeTurnoutResponse)

		err := protocol.Write(t.setStateCommand(StateExamine))
		if err != nil {
			return err
		}

		<-waiter.WaitC
		responseCommand = waiter.Command()
		if responseCommand == nil {
			return errors.New("status response is missing")
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to examine turnout %d: %w", t.id, err)
	}

	parameters, err := responseCommand.ParametersStrings()
	if err != nil {
		return nil, err
	}

	if len(parameters) != 7 {
		return nil, fmt.Errorf("invalid command: %q", responseCommand.String())
	}

	vPin, err := strconv.ParseUint(parameters[2], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid vpin %q: %w", parameters[2], err)
	}

	thrownPosition, err := strconv.ParseUint(parameters[3], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid thrown position %q: %w", parameters[3], err)
	}

	closedPosition, err := strconv.ParseUint(parameters[4], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid closed position %q: %w", parameters[4], err)
	}

	profile, err := strconv.ParseUint(parameters[5], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid profile %q: %w", parameters[5], err)
	}

	// State is returned as 0 or 1, not C and T.
	if parameters[6] != "0" && parameters[6] != "1" {
		return nil, fmt.Errorf("invalid state %q", parameters[6])
	}

	state := StateClosed
	if parameters[6] == "1" {
		state = StateThrown
	}

	status := &TurnoutServoStatus{
		VPin:           VPin(vPin),
		ThrownPosition: Position(thrownPosition),
		ClosedPosition: Position(closedPosition),
		Profile:        Profile(profile),
		State:          state,
	}

	return status, nil
}
