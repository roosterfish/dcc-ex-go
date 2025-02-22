package station

import (
	"context"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

type PowerState command.OpCode
type Track string

const (
	PowerOff PowerState = '0'
	PowerOn  PowerState = '1'
)

const (
	TrackMain Track = "MAIN"
	TrackProg Track = "PROG"
	TrackJoin Track = "JOIN"
)

type CommandStation struct {
	protocol protocol.ReadWriteCloser
}

func NewStation(protocol protocol.ReadWriteCloser) *CommandStation {
	return &CommandStation{
		protocol: protocol,
	}
}

func (s PowerState) OpCode() command.OpCode {
	return command.OpCode(s)
}

func (c *CommandStation) Console() (protocol.CommandC, protocol.WriteF, protocol.CleanupF) {
	commandC, cleanupF := c.protocol.Read()
	return commandC, c.protocol.Write, cleanupF
}

func (c *CommandStation) Power(state PowerState) error {
	return c.protocol.Write(command.NewCommand(state.OpCode(), ""))
}

func (c *CommandStation) PowerTrack(state PowerState, track Track) error {
	return c.protocol.Write(command.NewCommand(state.OpCode(), "%s", track))
}

func (c *CommandStation) SupportedCabs(ctx context.Context) (int, error) {
	commandC, cleanupF := c.protocol.ReadOpCode(command.OpCodeStationSupportedCabs)
	defer cleanupF()

	go func() {
		_ = c.protocol.Write(command.NewCommand(command.OpCodeStationSupportedCabs, ""))
	}()

	select {
	case cmd := <-commandC:
		parameters := cmd.Parameters()
		if len(parameters) != 1 {
			return 0, fmt.Errorf("Invalid command: %q", cmd.String())
		}

		supportedCabs, err := strconv.Atoi(parameters[0].(string))
		if err != nil {
			return 0, fmt.Errorf("Failed to cast supported cabs to int: %w", err)
		}

		return supportedCabs, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
