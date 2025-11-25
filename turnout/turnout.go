package turnout

import "github.com/roosterfish/dcc-ex-go/command"

type state command.OpCode

const (
	stateThrow state = 'T'
	stateClose state = 'C'
	// stateExamine state = 'X'
)
