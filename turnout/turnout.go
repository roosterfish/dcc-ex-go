package turnout

import "github.com/roosterfish/dcc-ex-go/command"

type State command.OpCode

const (
	StateThrown  State = 'T'
	StateClosed  State = 'C'
	StateExamine State = 'X'
)
