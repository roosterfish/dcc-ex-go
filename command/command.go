package command

import (
	"fmt"
	"slices"
	"strings"
)

type OpCode rune

const (
	OpCodeInfo                 OpCode = '@'
	OpCodeSuccess              OpCode = 'O'
	OpCodeFail                 OpCode = 'X'
	OpCodeStatus               OpCode = 's'
	OpCodeStatusResponse       OpCode = 'i'
	OpCodeEEPROM               OpCode = 'E'
	OpCodeCabSpeed             OpCode = 't'
	OpCodeCabFunction          OpCode = 'F'
	OpCodeStationSupportedCabs OpCode = '#'
	OpCodeSensorCreate         OpCode = 'S'
	OpCodeTurnout              OpCode = 'T'
	OpCodeTurnoutResponse      OpCode = 'H'
	OpCodeOutputControl        OpCode = 'z'
)

type Command struct {
	opCode     OpCode
	format     string
	parameters []any
}

// NewCommand returns a new memory representation of an opcode together with parameters.
func NewCommand(opCode OpCode, format string, parameters ...any) *Command {
	return &Command{
		opCode:     opCode,
		format:     format,
		parameters: parameters,
	}
}

// NewControlCommand returns a command's memory representation including a control command.
// This control command cannot be interpreted by DCC-EX which causes a <X> sent at the end
// of the output of the preceding valid command.
// This allows identifying the end of output caused by any command.
// An example control command would be <Q ><⚡> where the characters "><⚡" are internally represented
// as command parameters.
// DCC-EX lists all of the sensors including their current state followed by a <X> as <⚡>
// is not a valid command.
func NewControlCommand(opCode OpCode) *Command {
	return NewCommand(opCode, "%s", "><⚡")
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

	formatStrings := []string{}
	parameters := []any{}
	readingParameter := []rune{}
	readingQuotedString := false

	storeParameterF := func() {
		// In case the parameter was quoted persist this information by
		// setting its format string to %q.
		if slices.Contains(readingParameter, '"') {
			formatStrings = append(formatStrings, "%q")
		} else {
			formatStrings = append(formatStrings, "%s")
		}

		// Now trim off the quotes if present.
		parameters = append(parameters, strings.Trim(string(readingParameter), `"`))
	}

	for _, commandRune := range commandWithoutOpCode {
		// The end of the parameter is reached.
		// Insert it into the list of command parameters.
		if commandRune == ' ' && !readingQuotedString {
			storeParameterF()

			readingParameter = []rune{}
			continue
		}

		if commandRune == '"' {
			if !readingQuotedString {
				readingQuotedString = true
			} else {
				readingQuotedString = false
			}
		}

		readingParameter = append(readingParameter, commandRune)
	}

	if len(readingParameter) > 0 {
		storeParameterF()
	}

	return &Command{
		opCode:     OpCode(opCode),
		format:     strings.Join(formatStrings, " "),
		parameters: parameters,
	}, nil
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

func (c *Command) ParametersStrings() ([]string, error) {
	parametersStrings := make([]string, 0, len(c.parameters))
	for _, parameter := range c.parameters {
		parameterString, ok := parameter.(string)
		if !ok {
			return nil, fmt.Errorf("failed to cast parameter %q to string", parameter)
		}

		parametersStrings = append(parametersStrings, parameterString)
	}

	return parametersStrings, nil
}
