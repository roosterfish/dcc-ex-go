package turnout

import (
	"context"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
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
	turnoutCommand := command.NewCommand(command.OpCodeTurnout, "%d SERVO %d %d %d %d", t.id, vpin, thrownPos, closedPos, profile)
	persistCommand := command.NewCommand(command.OpCodeEEPROM, "")

	persisted := false
	err := t.channel.WriteAndReadOpCode(ctx, turnoutCommand.Append(persistCommand), command.OpCodeSuccess, func(cmd *command.Command) error {
		persisted = true
		return nil
	})
	if err != nil {
		return err
	}

	if !persisted {
		return fmt.Errorf("failed to persist turnout servo %d: %w", t.id, err)
	}

	return nil
}

func (t *TurnoutServo) setStateCommand(state State) *command.Command {
	return command.NewCommand(command.OpCodeTurnout, "%d %c", t.id, state)
}

func (t *TurnoutServo) equalsCommandParams(cmd *command.Command) error {
	params, err := cmd.ParametersStrings()
	if err != nil {
		return fmt.Errorf("failed getting turnout servo command parameters: %w", err)
	}

	turnoutMatch := len(params) == 2 && params[0] == strconv.FormatUint(uint64(t.id), 10)
	if !turnoutMatch {
		return fmt.Errorf("invalid response for turnout servo %d: %q", t.id, cmd.String())
	}

	return nil
}

// Throw throws the servo turnout.
// It first checks whether or not the turnout is already thrown.
func (t *TurnoutServo) Throw(ctx context.Context) error {
	return t.channel.SessionContext(ctx, func(ctx context.Context) error {
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
	})
}

// Close closes the servo turnout.
// It first checks whether or not the turnout is already closed.
func (t *TurnoutServo) Close(ctx context.Context) error {
	return t.channel.SessionContext(ctx, func(ctx context.Context) error {
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
	})
}

// Examine returns the status of the servo.
func (t *TurnoutServo) Examine(ctx context.Context) (*TurnoutServoStatus, error) {
	var status *TurnoutServoStatus

	err := t.channel.WriteAndReadOpCode(ctx, t.setStateCommand(StateExamine), command.OpCodeTurnoutResponse, func(cmd *command.Command) error {
		params, err := cmd.ParametersStrings()
		if err != nil {
			return fmt.Errorf("failed getting turnout servo command parameters: %w", err)
		}

		if len(params) != 7 {
			return fmt.Errorf("invalid turnout servo command parameter length %q", len(params))
		}

		vPin, err := strconv.ParseUint(params[2], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid vpin %q: %w", params[2], err)
		}

		thrownPosition, err := strconv.ParseUint(params[3], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid thrown position %q: %w", params[3], err)
		}

		closedPosition, err := strconv.ParseUint(params[4], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid closed position %q: %w", params[4], err)
		}

		profile, err := strconv.ParseUint(params[5], 10, 8)
		if err != nil {
			return fmt.Errorf("invalid profile %q: %w", params[5], err)
		}

		// State is returned as 0 or 1, not C and T.
		if params[6] != "0" && params[6] != "1" {
			return fmt.Errorf("invalid state %q", params[6])
		}

		state := StateClosed
		if params[6] == "1" {
			state = StateThrown
		}

		status = &TurnoutServoStatus{
			VPin:           VPin(vPin),
			ThrownPosition: Position(thrownPosition),
			ClosedPosition: Position(closedPosition),
			Profile:        Profile(profile),
			State:          state,
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get turnout servo %d status: %w", t.id, err)
	}

	if status == nil {
		return nil, fmt.Errorf("failed to find status for turnout servo %d", t.id)
	}

	return status, nil
}
