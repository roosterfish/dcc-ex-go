package output

import (
	"context"
	"fmt"
	"strconv"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
)

type ID uint16
type IFlag uint8

type Status struct {
	VPin  VPin
	IFlag IFlag
	State DigitalValue
}

type Output struct {
	id      ID
	channel *channel.Channel
}

func NewOutput(id ID, channel *channel.Channel) *Output {
	return &Output{
		id:      id,
		channel: channel,
	}
}

// Persist creates the output and persists its definition in the EEPROM.
func (o *Output) Persist(ctx context.Context, vpin VPin, iFlag IFlag) error {
	outputCommand := command.NewCommand(command.OpCodeOutput, "%d %d %d", o.id, vpin, iFlag)
	persistCommand := command.NewCommand(command.OpCodeEEPROM, "")

	persisted := false
	err := o.channel.WriteAndReadOpCode(ctx, outputCommand.Append(persistCommand), command.OpCodeSuccess, func(cmd *command.Command) error {
		persisted = true
		return nil
	})
	if err != nil {
		return err
	}

	if !persisted {
		return fmt.Errorf("failed to persist output %d: %w", o.id, err)
	}

	return nil
}

func (o *Output) setCommand(value DigitalValue) *command.Command {
	return command.NewCommand(command.OpCodeOutput, "%d %c", o.id, value)
}

func (o *Output) equalsCommandParams(cmd *command.Command) error {
	params, err := cmd.ParametersStrings()
	if err != nil {
		return fmt.Errorf("failed getting output command parameters: %w", err)
	}

	outputMatch := len(params) == 2 && params[0] == strconv.FormatUint(uint64(o.id), 10)
	if !outputMatch {
		return fmt.Errorf("invalid response for output %d: %q", o.id, cmd.String())
	}

	return nil
}

func (o *Output) High(ctx context.Context) error {
	return o.channel.WriteAndReadOpCode(ctx, o.setCommand(High), command.OpCodeOutputResponse, o.equalsCommandParams)
}

func (o *Output) Low(ctx context.Context) error {
	return o.channel.WriteAndReadOpCode(ctx, o.setCommand(Low), command.OpCodeOutputResponse, o.equalsCommandParams)
}

func (o *Output) Status(ctx context.Context) (*Status, error) {
	var outputStatus *Status

	statusCommand := command.NewCommand(command.OpCodeOutput, "")
	err := o.channel.WriteAndReadOpCode(ctx, statusCommand, command.OpCodeOutputResponse, func(cmd *command.Command) error {
		params, err := cmd.ParametersStrings()
		if err != nil {
			return fmt.Errorf("failed getting output command parameters: %w", err)
		}

		if len(params) != 4 {
			return fmt.Errorf("invalid output command parameter length %q", len(params))
		}

		if params[0] != strconv.FormatUint(uint64(o.id), 10) {
			// Not the right output, return early.
			return nil
		}

		vPin, err := strconv.ParseUint(params[1], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid vpin %q: %w", params[1], err)
		}

		iFlag, err := strconv.ParseUint(params[2], 10, 8)
		if err != nil {
			return fmt.Errorf("invalid iflag %q: %w", params[2], err)
		}

		state := []rune(params[3])
		if len(state) != 1 {
			return fmt.Errorf("invalid state %q", params[3])
		}

		outputStatus = &Status{
			VPin:  VPin(vPin),
			IFlag: IFlag(iFlag),
			State: DigitalValue(state[0]),
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get output %d status: %w", o.id, err)
	}

	if outputStatus == nil {
		return nil, fmt.Errorf("failed to find status for output %d", o.id)
	}

	return outputStatus, nil
}
