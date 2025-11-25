package turnout

import (
	"context"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"golang.org/x/sync/errgroup"
)

type ID uint16
type VPin int
type Pos uint16
type Profile uint8

type TurnoutServo struct {
	id      ID
	channel *channel.Channel
}

type TurnoutServoStatus struct {
	VPin           int
	ThrownPosition Pos
	ClosedPosition Pos
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
func (t *TurnoutServo) Persist(ctx context.Context, vpin VPin, thrownPos Pos, closedPos Pos, profile Profile) error {
	return t.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		err := protocol.Write(command.NewCommand(command.OpCodeTurnout, "%d SERVO %d %d %d %d", t.id, vpin, thrownPos, closedPos, profile))
		if err != nil {
			return fmt.Errorf("failed to create sensor: %w", err)
		}

		g, ctx := errgroup.WithContext(ctx)

		g.Go(func() error {
			_, err := protocol.ReadOpCode(ctx, command.OpCodeSuccess)
			return err
		})

		g.Go(func() error {
			return protocol.Write(command.NewCommand(command.OpCodeEEPROM, ""))
		})

		return g.Wait()
	})
}

func (t *TurnoutServo) setState(protocol protocol.ReadWriteCloser, state State) error {
	return protocol.Write(command.NewCommand(command.OpCodeTurnout, "%d %c", t.id, state))
}

func (t *TurnoutServo) Throw() error {
	return t.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return t.setState(protocol, StateThrown)
	})
}

func (t *TurnoutServo) Close() error {
	return t.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return t.setState(protocol, StateClosed)
	})
}

// Examine returns the status of the servo.
func (t *TurnoutServo) Examine(ctx context.Context) (*TurnoutServoStatus, error) {
	var responseCommand *command.Command
	err := t.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		g, ctx := errgroup.WithContext(ctx)

		g.Go(func() error {
			var err error
			responseCommand, err = protocol.ReadOpCode(ctx, command.OpCodeTurnoutResponse)
			return err
		})

		g.Go(func() error {
			return t.setState(protocol, StateExamine)
		})

		return g.Wait()
	})
	if err != nil {
		return nil, err
	}

	parameters, err := responseCommand.ParametersStrings()
	if err != nil {
		return nil, err
	}

	if len(parameters) != 7 {
		return nil, fmt.Errorf("invalid command: %q", responseCommand.String())
	}

	vPin, err := strconv.Atoi(parameters[2])
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

	state := []rune(parameters[6])
	if len(state) != 1 {
		return nil, fmt.Errorf("invalid state %q", parameters[6])
	}

	status := &TurnoutServoStatus{
		VPin:           vPin,
		ThrownPosition: Pos(thrownPosition),
		ClosedPosition: Pos(closedPosition),
		Profile:        Profile(profile),
		State:          State(state[0]),
	}

	return status, nil
}
