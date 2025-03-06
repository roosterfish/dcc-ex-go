package station

import (
	"context"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"golang.org/x/sync/errgroup"
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
	channel *channel.Channel
}

func NewStation(channel *channel.Channel) *CommandStation {
	return &CommandStation{
		channel: channel,
	}
}

func (s PowerState) OpCode() command.OpCode {
	return command.OpCode(s)
}

// Console returns a channel and writer which can be used to retrieve and send
// commands from and to the command station.
// It exposes the underlying protocol utilities directly by breaking out of the channel's session. Use it with care.
// Writing commands might influence concurrent sessions.
func (c *CommandStation) Console() (protocol.CommandC, protocol.WriteF, protocol.CleanupF) {
	var commandC protocol.CommandC
	var cleanupF protocol.CleanupF
	var writeF protocol.WriteF

	_ = c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		commandC, cleanupF = protocol.Read()
		writeF = protocol.Write
		return nil
	})

	return commandC, writeF, cleanupF
}

// Power sets the power to the given state.
func (c *CommandStation) Power(state PowerState) error {
	return c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return protocol.Write(command.NewCommand(state.OpCode(), ""))
	})
}

// PowerTrack sets the tracks power to the given state.
func (c *CommandStation) PowerTrack(state PowerState, track Track) error {
	return c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return protocol.Write(command.NewCommand(state.OpCode(), "%s", track))
	})
}

// Ready waits for the <@ 0 3 "Ready"> broadcast message which indicates the station is ready the receive commands.
func (c *CommandStation) Ready(ctx context.Context) error {
	return c.channel.RSession(func(protocol protocol.Reader) error {
		readyCommand := command.NewCommand(command.OpCodeInfo, "%d %d %q", 0, 3, "Ready")
		return protocol.ReadCommand(ctx, readyCommand)
	})
}

// Status returns DCC-EX version and hardware info, along with defined turnouts.
func (c *CommandStation) Status(ctx context.Context) (*Status, error) {
	var responseCommand *command.Command
	err := c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		g, ctx := errgroup.WithContext(ctx)

		g.Go(func() error {
			var err error
			responseCommand, err = protocol.ReadOpCode(ctx, command.OpCodeStatusResponse)
			return err
		})

		g.Go(func() error {
			return protocol.Write(command.NewCommand(command.OpCodeStatus, ""))
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

	status := &Status{
		Version:             parameters[1],
		MicroprocessorType:  parameters[3],
		MotorcontrollerType: parameters[5],
		BuildNumber:         parameters[6],
	}

	return status, nil
}

// SupportedCabs returns the number of supported cabs.
func (c *CommandStation) SupportedCabs(ctx context.Context) (int, error) {
	var responseCommand *command.Command
	err := c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		g, ctx := errgroup.WithContext(ctx)

		g.Go(func() error {
			var err error
			responseCommand, err = protocol.ReadOpCode(ctx, command.OpCodeStationSupportedCabs)
			return err
		})

		g.Go(func() error {
			return protocol.Write(command.NewCommand(command.OpCodeStationSupportedCabs, ""))
		})

		return g.Wait()
	})
	if err != nil {
		return 0, err
	}

	parameters, err := responseCommand.ParametersStrings()
	if err != nil {
		return 0, err
	}

	if len(parameters) != 1 {
		return 0, fmt.Errorf("Invalid command: %q", responseCommand.String())
	}

	supportedCabs, err := strconv.Atoi(parameters[0])
	if err != nil {
		return 0, fmt.Errorf("Failed to convert supported cabs to int: %w", err)
	}

	return supportedCabs, nil
}
