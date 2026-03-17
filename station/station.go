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
// It exposes the underlying protocol and channel utilities directly.
// Writing commands is protected using an exclusive session.
// Reading commands is happening outside of any session.
func (c *CommandStation) Console() (protocol.CommandC, channel.WriteF, protocol.CleanupF) {
	var commandC protocol.CommandC
	var cleanupF protocol.CleanupF
	var writeF channel.WriteF

	_ = c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		commandC, cleanupF = protocol.Read()
		writeF = c.channel.Write
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
	var status *Status

	statusCommand := command.NewCommand(command.OpCodeStatus, "")
	err := c.channel.WriteAndReadOpCode(ctx, statusCommand, command.OpCodeStatusResponse, func(cmd *command.Command) error {
		params, err := cmd.ParametersStrings()
		if err != nil {
			return fmt.Errorf("failed getting command station command parameters: %w", err)
		}

		if len(params) != 7 {
			return fmt.Errorf("invalid command station command parameter length %q", len(params))
		}

		status = &Status{
			Version:             params[1],
			MicroprocessorType:  params[3],
			MotorcontrollerType: params[5],
			BuildNumber:         params[6],
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get command station status: %w", err)
	}

	if status == nil {
		return nil, errors.New("failed to find status for command station")
	}

	return status, nil
}

// SupportedCabs returns the number of supported cabs.
func (c *CommandStation) SupportedCabs(ctx context.Context) (int, error) {
	var supportedCabs *int

	supportedCabsCommand := command.NewCommand(command.OpCodeStationSupportedCabs, "")
	err := c.channel.WriteAndReadOpCode(ctx, supportedCabsCommand, command.OpCodeStationSupportedCabs, func(cmd *command.Command) error {
		params, err := cmd.ParametersStrings()
		if err != nil {
			return fmt.Errorf("failed getting supported cabs command parameters: %w", err)
		}

		if len(params) != 1 {
			return fmt.Errorf("invalid supported cabs command parameter length %q", len(params))
		}

		supportedCabsResponse, err := strconv.Atoi(params[0])
		if err != nil {
			return fmt.Errorf("failed to convert supported cabs to int: %w", err)
		}

		supportedCabs = &supportedCabsResponse

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get supported cabs: %w", err)
	}

	if supportedCabs == nil {
		return 0, errors.New("failed to find supported cabs")
	}

	return *supportedCabs, nil
}
