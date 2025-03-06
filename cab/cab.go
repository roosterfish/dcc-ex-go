package cab

import (
	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

// CabDirection can be either 0 or 1.
type Direction uint8

// Speed can be -1 or 0-127.
type Speed int8
type Address uint16
type Function uint8

// CabFunctionState can be either 0 or 1.
type FunctionState uint8

type Cab struct {
	address Address
	channel *channel.Channel
}

const (
	DirectionForward Direction = iota
	DirectionBackward
)

const (
	FunctionOff FunctionState = iota
	FunctionOn
)

const (
	CabCommand rune = 't'
)

func NewCab(address Address, channel *channel.Channel) *Cab {
	return &Cab{
		address: address,
		channel: channel,
	}
}

func (c *Cab) Speed(speed Speed, direction Direction) error {
	return c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return protocol.Write(command.NewCommand(command.OpCodeCabSpeed, "%d %d %d", c.address, speed, direction))
	})
}

func (c *Cab) Function(funct Function, state FunctionState) error {
	return c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return protocol.Write(command.NewCommand(command.OpCodeCabFunction, "%d %d %d", c.address, funct, state))
	})
}
