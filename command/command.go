package command

import (
	"fmt"
	"strings"
)

type OpCode string

const (
	OpCodeCabSpeed    OpCode = "t"
	OpCodeCabFunction OpCode = "F"
)

type Command struct {
	opCode     OpCode
	format     string
	parameters []any
}

func NewCommand(opCode OpCode, format string, parameters ...any) *Command {
	return &Command{
		opCode:     opCode,
		format:     format,
		parameters: parameters,
	}
}

func NewCommandFromString(command string) (*Command, error) {
	if len(command) < 3 {
		return nil, fmt.Errorf("Invalid command: %q", command)
	}

	commandTrimmed := strings.Trim(command, "<>")
	commandSplit := strings.Split(commandTrimmed, " ")
	if len(commandSplit) < 1 {
		return nil, fmt.Errorf("Op code missing in command: %q", command)
	}

	formatStr := make([]string, 0, len(commandSplit)-1)
	parameters := make([]any, 0, len(commandSplit)-1)
	for _, parameter := range commandSplit[1:] {
		formatStr = append(formatStr, "%s")
		parameters = append(parameters, parameter)
	}

	return &Command{
		opCode:     OpCode(commandSplit[0]),
		format:     strings.Join(formatStr, " "),
		parameters: parameters,
	}, nil
}

func (c *Command) String() string {
	// If no format is provided just print the op code.
	if c.format == "" {
		return fmt.Sprintf("<%s>", c.opCode)
	}

	return fmt.Sprintf(fmt.Sprintf("<%s %s>", c.opCode, c.format), c.parameters...)
}

func (c *Command) Bytes() []byte {
	return []byte(fmt.Sprintf("%s\n", c.String()))
}

func (c *Command) OpCode() OpCode {
	return c.opCode
}

func (c *Command) Parameters() []any {
	return c.parameters
}
