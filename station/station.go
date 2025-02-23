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

type Status struct {
	Version             string
	MicroprocessorType  string
	MotorcontrollerType string
	BuildNumber         string
}

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

// Console returns a channel and writer which can be used to retrieve and send
// commands from and to the command station.
func (c *CommandStation) Console() (protocol.CommandC, protocol.WriteF, protocol.CleanupF) {
	commandC, cleanupF := c.protocol.Read()
	return commandC, c.protocol.Write, cleanupF
}

// Power sets the power to the given state.
func (c *CommandStation) Power(state PowerState) error {
	return c.protocol.Write(command.NewCommand(state.OpCode(), ""))
}

// PowerTrack sets the tracks power to the given state.
func (c *CommandStation) PowerTrack(state PowerState, track Track) error {
	return c.protocol.Write(command.NewCommand(state.OpCode(), "%s", track))
}

// Ready waits for the <@ 0 3 "Ready"> broadcast message which indicates the station is ready the receive commands.
func (c *CommandStation) Ready(ctx context.Context) error {
	readyCommand := command.NewCommand(command.OpCodeInfo, "%d %d %q", 0, 3, "Ready")
	observationC, cleanupF := c.protocol.ReadCommand(readyCommand)
	defer cleanupF()

	select {
	case <-observationC:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status returns DCC-EX version and hardware info, along with defined turnouts.
func (c *CommandStation) Status(ctx context.Context) (*Status, error) {
	commandC, cleanupF := c.protocol.ReadOpCode(command.OpCodeStatusResponse)
	defer cleanupF()

	go func() {
		_ = c.protocol.Write(command.NewCommand(command.OpCodeStatus, ""))
	}()

	select {
	case statusCommand := <-commandC:
		parameters, err := statusCommand.ParametersStrings()
		if err != nil {
			return nil, err
		}

		if len(parameters) != 7 {
			return nil, fmt.Errorf("invalid command: %q", statusCommand.String())
		}

		status := &Status{
			Version:             parameters[1],
			MicroprocessorType:  parameters[3],
			MotorcontrollerType: parameters[5],
			BuildNumber:         parameters[6],
		}

		return status, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SupportedCabs returns the number of supported cabs.
func (c *CommandStation) SupportedCabs(ctx context.Context) (int, error) {
	commandC, cleanupF := c.protocol.ReadOpCode(command.OpCodeStationSupportedCabs)
	defer cleanupF()

	go func() {
		_ = c.protocol.Write(command.NewCommand(command.OpCodeStationSupportedCabs, ""))
	}()

	select {
	case cmd := <-commandC:
		parameters, err := cmd.ParametersStrings()
		if err != nil {
			return 0, err
		}

		if len(parameters) != 1 {
			return 0, fmt.Errorf("Invalid command: %q", cmd.String())
		}

		supportedCabs, err := strconv.Atoi(parameters[0])
		if err != nil {
			return 0, fmt.Errorf("Failed to convert supported cabs to int: %w", err)
		}

		return supportedCabs, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
