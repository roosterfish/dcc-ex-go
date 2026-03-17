package channel

import (
	"context"
	"fmt"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

// WriteAndReadOpCode abstracts an underlying read/write session by writing the given command and waiting for a response with the given op code.
// Once the op code is observed, the given function f is called with the observed command(s).
// It will continue to read commands until the function f returns an error, the context is cancelled or the control command is observed.
// In case the function f returns an error, those are accumulated and only returned once
func (c *Channel) WriteAndReadOpCode(ctx context.Context, cmd *command.Command, o command.OpCode, f func(cmd *command.Command) error) error {
	return c.Session(func(protocol protocol.ReadWriteCloser) error {
		commandC, cleanupF := protocol.Read()
		defer cleanupF()

		// Derive a new control command.
		controlCommand := command.NewControlCommand(cmd.OpCode(), cmd.Format(), cmd.Parameters()...)
		err := protocol.Write(controlCommand)
		if err != nil {
			return err
		}

		// When sending <X>, the command stations replies with <* Opcode=X params=0 *><X>.
		describeCommandStr := command.NewCommand(command.OpCodeDescribe, "%s %s %s", "Opcode=X", "params=0", "*").String()
		describeCommandObserved := false

		for {
			select {
			case cmd := <-commandC:
				if cmd.OpCode() == o {
					err := f(cmd)
					if err != nil {
						return fmt.Errorf("failed to run function: %w", err)
					}
				} else if cmd.String() == describeCommandStr {
					// About to be done, waiting for <X>.
					describeCommandObserved = true
				} else if cmd.OpCode() == command.OpCodeFail && describeCommandObserved {
					// <X> observed, return the session cleanly.
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
}
