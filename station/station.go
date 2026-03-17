package station

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/channel"
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
func (c *CommandStation) Power(ctx context.Context, state PowerState) error {
	return c.channel.WriteAndReadOpCode(ctx, command.NewCommand(state.OpCode(), ""), command.OpCodePower, func(cmd *command.Command) error {
		params, err := cmd.ParametersStrings()
		if err != nil {
			return fmt.Errorf("failed getting command station command parameters: %w", err)
		}

		powerMatch := len(params) == 1 && params[0] == string(state)
		if !powerMatch {
			return fmt.Errorf("invalid response for power command: %q", cmd.String())
		}

		return nil
	})
}

// PowerTrack sets the tracks power to the given state.
func (c *CommandStation) PowerTrack(ctx context.Context, state PowerState, track Track) error {
	powerChanged := false

	err := c.channel.WriteAndReadOpCode(ctx, command.NewCommand(state.OpCode(), "%s", track), command.OpCodeInfo, func(cmd *command.Command) error {
		params, err := cmd.ParametersStrings()
		if err != nil {
			return fmt.Errorf("failed getting command station command parameters: %w", err)
		}

		// Powering on/off returns various broadcasts:
		// <1 MAIN> <@ 0 2 "PWR Ab">
		// <0 MAIN> <@ 0 2 "PWR Off">
		// <1 PROG> <@ 0 2 "PWR aB">
		// <0 PROG> <@ 0 2 "PWR Off">
		// <1 JOIN> <@ 0 2 "PWR On JOIN">
		// <0 MAIN> <@ 0 2 "PWR aB">
		// <0 PROG> <@ 0 2 "PWR Ab">
		// Use the least common denominator <@ 0 2.
		if len(params) == 3 && params[0] == "0" && params[1] == "2" {
			powerChanged = true
		}

		return nil
	})
	if err != nil {
		return err
	}

	if !powerChanged {
		return fmt.Errorf("failed to set power %q on track %q", state, track)
	}

	return nil
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
		waiter := protocol.ReadOpCode(ctx, command.OpCodeStatusResponse)

		err := protocol.Write(command.NewCommand(command.OpCodeStatus, ""))
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
		waiter := protocol.ReadOpCode(ctx, command.OpCodeStationSupportedCabs)

		err := protocol.Write(command.NewCommand(command.OpCodeStationSupportedCabs, ""))
		if err != nil {
			return err
		}

		<-waiter.WaitC
		responseCommand = waiter.Command()
		if responseCommand == nil {
			return errors.New("supported cabs response is missing")
		}

		return nil
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
