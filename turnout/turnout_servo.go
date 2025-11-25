package turnout

import (
	"context"
	"fmt"

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

func (t *TurnoutServo) changeState(s state) error {
	return t.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return protocol.Write(command.NewCommand(command.OpCodeTurnout, "%d %c", t.id, s))
	})
}

func (t *TurnoutServo) Throw() error {
	return t.changeState(stateThrow)
}

func (t *TurnoutServo) Close() error {
	return t.changeState(stateClose)
}
