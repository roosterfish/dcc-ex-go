package cab

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

// CabDirection can be either 0 or 1.
type Direction uint8

// Speed can be -1 (emergency stop) or 0-127.
type Speed int8
type Address uint16
type Function uint8

// CabFunctionState can be either 0 or 1.
type FunctionState uint8

type Cab struct {
	address Address
	channel *channel.Channel
}

type CabStatus struct {
	SpeedByte uint8
	FunctMap  uint8
}

const (
	DirectionBackward Direction = iota
	DirectionForward
)

const (
	FunctionOff FunctionState = iota
	FunctionOn
)

const (
	CabCommand rune = 't'
)

func (d Direction) Opposite() Direction {
	if d == DirectionForward {
		return DirectionBackward
	}

	return DirectionForward
}

func NewCab(address Address, channel *channel.Channel) *Cab {
	return &Cab{
		address: address,
		channel: channel,
	}
}

func (c *Cab) equalsCommandParams(cmd *command.Command) (bool, error) {
	params, err := cmd.ParametersStrings()
	if err != nil {
		return false, fmt.Errorf("failed getting cab command parameters: %w", err)
	}

	return len(params) == 4 && params[0] == strconv.FormatUint(uint64(c.address), 10), nil
}

func (c *Cab) speedUnchanged(status *CabStatus, newSpeed Speed, newDirection Direction) bool {
	// 1: Backward emergency stop
	// 129: Forward emergency stop
	if status.SpeedByte == 1 && newSpeed == -1 && newDirection == DirectionBackward {
		return true
	} else if status.SpeedByte == 129 && newSpeed == -1 && newDirection == DirectionForward {
		return true
	}

	// 2-127: Backward 1-126
	// 130-255: Forward 1-126
	if status.SpeedByte >= 2 && status.SpeedByte <= 127 {
		// Check if already at the same speed and going backward.
		if status.SpeedByte-1 == uint8(newSpeed) && newDirection == DirectionBackward {
			return true
		}
	} else if status.SpeedByte >= 130 {
		// Check if already at the same speed and going forward.
		if status.SpeedByte-129 == uint8(newSpeed) && newDirection == DirectionForward {
			return true
		}
	} else if status.SpeedByte == 0 && uint8(newSpeed) == 0 && newDirection == DirectionBackward {
		// Stopped backward.
		return true
	} else if status.SpeedByte == 128 && uint8(newSpeed) == 0 && newDirection == DirectionForward {
		// Stopped forward.
		return true
	}

	return false
}

// Speed sets the cabs speed and direction.
// It first checks whether or not the speed and direction is already set.
// Keep in mind that the check and change are not inside the same session.
func (c *Cab) Speed(ctx context.Context, speed Speed, direction Direction) error {
	// Check if already at the requested speed.
	// There isn't a broadcast sent if the cab is already at the requested speed and direction.
	status, err := c.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status of cab %q: %w", c.address, err)
	}

	if c.speedUnchanged(status, speed, direction) {
		return nil
	}

	speedCommand := command.NewCommand(command.OpCodeCabSpeed, "%d %d %d", c.address, speed, direction)
	return c.channel.WriteAndReadOpCode(ctx, speedCommand, command.OpCodeCabResponse, c.equalsCommandParams)
}

func (c *Cab) Function(ctx context.Context, funct Function, state FunctionState) error {
	functionCommand := command.NewCommand(command.OpCodeCabFunction, "%d %d %d", c.address, funct, state)
	return c.channel.WriteAndReadOpCode(ctx, functionCommand, command.OpCodeCabResponse, c.equalsCommandParams)
}

func (c *Cab) Status(ctx context.Context) (*CabStatus, error) {
	var responseCommand *command.Command
	err := c.channel.SessionSuccess(ctx, func(ctx context.Context, protocol protocol.ReadWriteCloser) error {
		waiter := protocol.ReadOpCode(ctx, command.OpCodeCabResponse)

		err := protocol.Write(command.NewCommand(command.OpCodeCabSpeed, "%d", c.address))
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
		return nil, fmt.Errorf("failed to get status of cab %d: %w", c.address, err)
	}

	parameters, err := responseCommand.ParametersStrings()
	if err != nil {
		return nil, err
	}

	if len(parameters) != 4 {
		return nil, fmt.Errorf("invalid command: %q", responseCommand.String())
	}

	speedByte, err := strconv.ParseUint(parameters[2], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid speed byte %q: %w", parameters[2], err)
	}

	functMap, err := strconv.ParseUint(parameters[3], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid funct map %q: %w", parameters[3], err)
	}

	status := &CabStatus{
		SpeedByte: uint8(speedByte),
		FunctMap:  uint8(functMap),
	}

	return status, nil
}
