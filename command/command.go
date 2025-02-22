package command

import (
	"fmt"
	"strings"
)

type OpCode rune

const (
	OpCodeCabSpeed             OpCode = 't'
	OpCodeCabFunction          OpCode = 'F'
	OpCodeStationSupportedCabs OpCode = '#'
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

// NewCommandFromString creates a new command from the given string.
// The command can still contain the delimiting arrow characters
// as they will get trimmed from the command in any case.
// It follows the parsing recommendations in https://dcc-ex.com/reference/developers/api.html.
// The internal representation of commands whose op code is immediately followed by the
// first paramter is always performed using a separating whitespace.
// A good example is the <JT> command whose internal representation is <J T>.
// Also its response <jT> is represented as <j T> within the Command struct.
func NewCommandFromString(command string) (*Command, error) {
	commandTrimmed := strings.Trim(command, "<>")
	if len(commandTrimmed) == 0 {
		return nil, fmt.Errorf("invalid command length: %q", command)
	}

	opCode := commandTrimmed[0]

	// Trim unwanted whitespaces from left and right.
	commandWithoutOpCode := strings.Trim(commandTrimmed[1:], " ")

	// Split all the parameters.
	commandSplitted := strings.Split(commandWithoutOpCode, " ")

	newCommand := Command{
		opCode: OpCode(opCode),
	}
	formatStrings := []string{}
	for _, parameter := range commandSplitted {
		newCommand.parameters = append(newCommand.parameters, parameter)
		formatStrings = append(formatStrings, "%s")
	}

	newCommand.format = strings.Join(formatStrings, " ")
	return &newCommand, nil
}

func (c *Command) String() string {
	// If no format is provided just print the op code.
	if c.format == "" {
		return fmt.Sprintf("<%c>", c.opCode)
	}

	return fmt.Sprintf(fmt.Sprintf("<%c %s>", c.opCode, c.format), c.parameters...)
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
