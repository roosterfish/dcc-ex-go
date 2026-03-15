package channel

import (
	"context"
	"fmt"

	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

// WriteAndReadOpCode abstracts an underlying read/write session by writing the given command and waiting for a response with the given op code.
// Once the op code is observed, the given function f is called with the commands's parameters.
// It will continue to read commands until the function f returns true or the context is done.
func (c *Channel) WriteAndReadOpCode(ctx context.Context, cmd *command.Command, o command.OpCode, f func(cmd []string) bool) error {
	return c.Session(func(protocol protocol.ReadWriteCloser) error {
		commandC, cleanupF := protocol.Read()
		defer cleanupF()

		err := protocol.Write(cmd)
		if err != nil {
			return err
		}

		for {
			select {
			case cmd := <-commandC:
				if cmd.OpCode() == o {
					params, err := cmd.ParametersStrings()
					if err != nil {
						return fmt.Errorf("failed to parse command %q: %w", cmd.String(), err)
					}

					if f(params) {
						return nil
					}
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
}
