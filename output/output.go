package output

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
	return o.channel.SessionSuccess(ctx, func(ctx context.Context, protocol protocol.ReadWriteCloser) error {
		err := protocol.Write(command.NewCommand(command.OpCodeOutput, "%d %d %d", o.id, vpin, iFlag))
		if err != nil {
			return fmt.Errorf("failed to create output: %w", err)
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

func (o *Output) set(protocol protocol.ReadWriteCloser, value DigitalValue) error {
	return protocol.Write(command.NewCommand(command.OpCodeOutput, "%d %c", o.id, value))
}

func (o *Output) High() error {
	return o.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return o.set(protocol, High)
	})
}

func (o *Output) Low() error {
	return o.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return o.set(protocol, Low)
	})
}

func (o *Output) Status(ctx context.Context) (*Status, error) {
	var outputStatus *Status
	err := o.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		commandC, cleanupF := protocol.Read()
		defer cleanupF()

		g := &errgroup.Group{}
		g.Go(func() error {
			// Send a control command to allow waiting for the end of the output.
			return protocol.Write(command.NewControlCommand(command.OpCodeOutput))
		})

		err := g.Wait()
		if err != nil {
			return err
		}

		for {
			select {
			case cmd := <-commandC:
				// Loop through the received commands until observing the end of the output.
				// This is indicated by the fail opcode as we have sent an invalid control command.
				if cmd.OpCode() == command.OpCodeFail {
					return nil
				}

				if cmd.OpCode() == command.OpCodeOutputResponse {
					params, err := cmd.ParametersStrings()
					if err != nil {
						continue
					}

					if len(params) != 4 {
						continue
					}

					if params[0] != strconv.FormatUint(uint64(o.id), 10) {
						continue
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
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get output status: %w", err)
	}

	if outputStatus == nil {
		return nil, fmt.Errorf("failed to find status for output %d", o.id)
	}

	return outputStatus, nil
}
