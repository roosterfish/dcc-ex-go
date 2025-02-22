package command

import (
	"testing"
)

func TestNewCommandFromString(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		opCode     OpCode
		format     string
		parameters []any
	}{
		{
			name:       "op code only",
			command:    "<a>",
			opCode:     'a',
			format:     "",
			parameters: []any{},
		},
		{
			name:       "op code and one parameter",
			command:    "<a b>",
			opCode:     'a',
			format:     "%s",
			parameters: []any{"b"},
		},
		{
			name:       "op code and two parameters",
			command:    "<a b c>",
			opCode:     'a',
			format:     "%s %s",
			parameters: []any{"b", "c"},
		},
		{
			name:       "op code and three mixed parameters",
			command:    "<a 3 c>",
			opCode:     'a',
			format:     "%s %s",
			parameters: []any{"3", "c"},
		},
		{
			name:       "op code and three mixed parameters with multi character strings",
			command:    "<a 3 hello world>",
			opCode:     'a',
			format:     "%s %s %s",
			parameters: []any{"3", "hello", "world"},
		},
		{
			name:       "op code and three mixed parameters with multi character strings in uppercase",
			command:    "<a 3 HELLO WORLD>",
			opCode:     'a',
			format:     "%s %s %s",
			parameters: []any{"3", "HELLO", "WORLD"},
		},
		{
			name:       "op code and quoted parameter",
			command:    `<a "hello">`,
			opCode:     'a',
			format:     "%q",
			parameters: []any{"hello"},
		},
		{
			name:       "op code and quoted parameter with space",
			command:    `<a "hello world">`,
			opCode:     'a',
			format:     "%q",
			parameters: []any{"hello world"},
		},
		{
			name:       "op code and quoted parameter with multiple spaces",
			command:    `<a "hello world from test">`,
			opCode:     'a',
			format:     "%q",
			parameters: []any{"hello world from test"},
		},
		{
			name:       "op code and multiple quoted parameters",
			command:    `<a "hello" "world">`,
			opCode:     'a',
			format:     "%q %q",
			parameters: []any{"hello", "world"},
		},
		{
			name:       "op code and multiple quoted parameters with multiple spaces",
			command:    `<a "hello world 1" "hello world 2">`,
			opCode:     'a',
			format:     "%q %q",
			parameters: []any{"hello world 1", "hello world 2"},
		},
		{
			name:       "op code with mixed parameters",
			command:    `<a 1 "hello world" abc 42 "hello">`,
			opCode:     'a',
			format:     "%s %q %s %s %q",
			parameters: []any{"1", "hello world", "abc", "42", "hello"},
		},
		{
			name:       "op code with parameter without space separation",
			command:    `<a1>`,
			opCode:     'a',
			format:     "%s",
			parameters: []any{"1"},
		},
		{
			name:       "op code with string parameter without space separation",
			command:    `<ab>`,
			opCode:     'a',
			format:     "%s",
			parameters: []any{"b"},
		},
		{
			name:       "op code with quoted string parameter without space separation",
			command:    `<a"b">`,
			opCode:     'a',
			format:     "%q",
			parameters: []any{"b"},
		},
		// Real examples from DCC-EX.
		{
			name:    "DCC-EX status command",
			command: `<* Track B sensOffset=0 *>`,
			opCode:  '*',
			format:  "%s %s %s %s",
			// Special case: The last * is also parsed as parameter
			parameters: []any{"Track", "B", `sensOffset=0`, "*"},
		},
		{
			name:       "DCC-EX version and hardware info",
			command:    `<iDCC-EX V-5.4.0 / MEGA / EX8874 G-c389fe9>`,
			opCode:     'i',
			format:     "%s %s %s %s %s %s %s",
			parameters: []any{"DCC-EX", "V-5.4.0", "/", "MEGA", "/", "EX8874", "G-c389fe9"},
		},
	}

	for _, test := range tests {
		command, err := NewCommandFromString(test.command)
		if err != nil {
			t.Error(err)
		}

		if command == nil {
			t.Error("Command is nil")
		}

		if test.opCode != command.opCode {
			t.Errorf("Expected op code %c but got %c", test.opCode, command.opCode)
		}

		if test.format != command.format {
			t.Errorf("Expected format %q but got %q", test.format, command.format)
		}

		if len(test.parameters) != len(command.parameters) {
			t.Errorf("Expected %d parameters (%+v) but got %d (%+v)", len(test.parameters), test.parameters, len(command.parameters), command.parameters)
		}

		for i, parameter := range test.parameters {
			if parameter != command.parameters[i] {
				t.Errorf("Expected parameter %q but got %q", parameter, command.parameters[i])
			}
		}
	}
}
