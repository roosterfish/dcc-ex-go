package connection

import (
	"fmt"
	"io"

	"github.com/roosterfish/dcc-ex-go/cab"
	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/output"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"github.com/roosterfish/dcc-ex-go/sensor"
	"github.com/roosterfish/dcc-ex-go/station"
	"github.com/roosterfish/dcc-ex-go/turnout"
	"go.bug.st/serial"
)

type Mode *serial.Mode

type Config struct {
	Device string
	Mode   Mode
	// RequireSubscriber sets whether or not the connections protocol listener starts to consume
	// messages before there is a single subscriber reading commands.
	// The default is true which allows waiting until the command station is ready.
	RequireSubscriber bool
}

type Connection struct {
	config  *Config
	channel *channel.Channel
}

var DefaultMode Mode = &serial.Mode{
	BaudRate: 115200,
}

func NewDefaultConfig(device string) *Config {
	return &Config{
		Device:            device,
		Mode:              DefaultMode,
		RequireSubscriber: true,
	}
}

func NewConnection(config *Config) (*Connection, error) {
	conn := &Connection{
		config: config,
	}

	// Open up a new serial connection.
	port, err := conn.open()
	if err != nil {
		return nil, err
	}

	// Wrap the serial connection with the protocol utilities.
	connectionProtocol := protocol.NewProtocol(port, &protocol.Config{
		RequireSubscriber: config.RequireSubscriber,
	})

	// Expose the protocol utilities using a channel.
	// The channel offers various entities to interact with the underlying serial connection.
	conn.channel = channel.NewChannel(connectionProtocol)
	return conn, nil
}

// open tries to open up a new serial connection using the given device.
func (c *Connection) open() (io.ReadWriteCloser, error) {
	port, err := serial.Open(c.config.Device, c.config.Mode)
	if err != nil {
		return nil, fmt.Errorf("Failed to open %q: %w", c.config.Device, err)
	}

	return port, nil
}

func (c *Connection) Cab(address cab.Address) *cab.Cab {
	return cab.NewCab(address, c.channel)
}

func (c *Connection) Sensor(id sensor.ID) *sensor.Sensor {
	return sensor.NewSensor(id, c.channel)
}

func (c *Connection) TurnoutServo(id turnout.ID) *turnout.TurnoutServo {
	return turnout.NewTurnoutServo(id, c.channel)
}

func (c *Connection) OutputHeadless() *output.OutputHeadless {
	return output.NewOutputHeadless(c.channel)
}

func (c *Connection) CommandStation() *station.CommandStation {
	return station.NewStation(c.channel)
}

func (c *Connection) Close() error {
	return c.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		return protocol.Close()
	})
}
