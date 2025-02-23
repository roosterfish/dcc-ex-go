package connection

import (
	"fmt"
	"io"

	"github.com/roosterfish/dcc-ex-go/cab"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"github.com/roosterfish/dcc-ex-go/sensor"
	"github.com/roosterfish/dcc-ex-go/station"
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
	config   *Config
	protocol protocol.ReadWriteCloser
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

	port, err := conn.open()
	if err != nil {
		return nil, err
	}

	conn.protocol = protocol.NewProtocol(port)
	return conn, nil
}

func (c *Connection) open() (io.ReadWriteCloser, error) {
	port, err := serial.Open(c.config.Device, c.config.Mode)
	if err != nil {
		return nil, fmt.Errorf("Failed to open %q: %w", c.config.Device, err)
	}

	return port, nil
}

func (c *Connection) Cab(address cab.Address) *cab.Cab {
	return cab.NewCab(address, c.protocol)
}

func (c *Connection) Sensor(id sensor.ID) *sensor.Sensor {
	return sensor.NewSensor(id, c.protocol)
}

func (c *Connection) CommandStation() *station.CommandStation {
	return station.NewStation(c.protocol)
}

func (c *Connection) Close() error {
	return c.protocol.Close()
}
