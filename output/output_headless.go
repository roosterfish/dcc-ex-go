package output

import (
	"fmt"
	"time"

	"github.com/roosterfish/dcc-ex-go/channel"
	"github.com/roosterfish/dcc-ex-go/command"
	"github.com/roosterfish/dcc-ex-go/protocol"
)

type VPin uint16
type AnalogValue uint16
type DigitalValue rune
type Profile uint16
type Duration uint16

const (
	ProfileNoPowerOff Profile = 0x80

	LEDLow  AnalogValue = 0
	LEDHigh AnalogValue = 4095

	Low  DigitalValue = '0'
	High DigitalValue = '1'
)

type OutputHeadless struct {
	channel *channel.Channel
}

// NewOutputHeadless returns an output without ID.
// It allows directly interacting with vPINs.
func NewOutputHeadless(channel *channel.Channel) *OutputHeadless {
	return &OutputHeadless{
		channel: channel,
	}
}

// Set sets the digital value to vPin.
func (o *OutputHeadless) Set(vPin VPin, value DigitalValue) error {
	var prefix string
	if value == Low {
		prefix = "-"
	}

	return o.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		err := protocol.Write(command.NewCommand(command.OpCodeOutputControl, "%s%d", prefix, vPin))
		if err != nil {
			return fmt.Errorf("failed to set digital value on vpin %d: %w", vPin, err)
		}

		return nil
	})
}

// SetAnalog sets the analog value to vPin using profile.
func (o *OutputHeadless) SetAnalog(vPin VPin, value AnalogValue, profile Profile) error {
	return o.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		err := protocol.Write(command.NewCommand(command.OpCodeOutputControl, "%d %d %d", vPin, value, profile))
		if err != nil {
			return fmt.Errorf("failed to set analog value on vpin %d: %w", vPin, err)
		}

		return nil
	})
}

// SetAnalogDuration sets the analog value to vPin using profile and duration.
func (o *OutputHeadless) SetAnalogDuration(vPin VPin, value AnalogValue, profile Profile, duration time.Duration) error {
	return o.channel.Session(func(protocol protocol.ReadWriteCloser) error {
		err := protocol.Write(command.NewCommand(command.OpCodeOutputControl, "%d %d %d %d", vPin, value, profile, duration.Milliseconds()/100))
		if err != nil {
			return fmt.Errorf("failed to set analog value on vpin %d over duration %q: %w", vPin, duration.String(), err)
		}

		return nil
	})
}
