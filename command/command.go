package command

import (
	"fmt"
	"strings"
)

type OpCode rune

const (
	OpCodeCabSpeed    OpCode = 't'
	OpCodeCabFunction OpCode = 'F'
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

func ParseRaw(command string) (string, error) {
	oneLine := strings.Trim(command, "\n")
	if len(oneLine) < 3 {
		return "", fmt.Errorf("Invalid command: %q", command)
	}

	return oneLine, nil
}

func (c *Command) StringRaw() string {
	// If no format is provided just print the op code.
	if c.format == "" {
		return fmt.Sprintf("<%c>", c.opCode)
	}

	return fmt.Sprintf(fmt.Sprintf("<%c %s>", c.opCode, c.format), c.parameters...)
}

func (c *Command) Bytes() []byte {
	return []byte(fmt.Sprintf("%s\n", c.StringRaw()))
}
