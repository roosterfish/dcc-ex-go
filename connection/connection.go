package connection

import (
	"fmt"

	"github.com/roosterfish/dcc-ex-go/cab"
	"github.com/roosterfish/dcc-ex-go/protocol"
	"github.com/roosterfish/dcc-ex-go/sensor"
	"github.com/roosterfish/dcc-ex-go/station"
	"go.bug.st/serial"
)

type Mode *serial.Mode

type Connection struct {
	device   string
	mode     Mode
	protocol protocol.ReadWriteCloser
}

var DefaultMode Mode = &serial.Mode{
	BaudRate: 115200,
}

func NewConnection(device string, mode Mode) (*Connection, error) {
	conn := &Connection{
		device: device,
		mode:   mode,
	}

	port, err := conn.open()
	if err != nil {
		return nil, err
	}

	conn.protocol = protocol.NewProtocol(port)
	return conn, nil
}

func (c *Connection) open() (serial.Port, error) {
	port, err := serial.Open(c.device, c.mode)
	if err != nil {
		return nil, fmt.Errorf("Failed to open %q: %w", c.device, err)
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
