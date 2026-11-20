//go:build integration

package station_test

import (
	"context"
	"testing"

	"github.com/roosterfish/dcc-ex-go/connection"
	"github.com/stretchr/testify/require"
)

func TestCommandStation_Status(t *testing.T) {
	conn, err := connection.NewConnection(connection.NewDefaultConfig("/tmp/simavr-uart0"))
	require.NoError(t, err)

	defer conn.Close()

	commandStation := conn.CommandStation()
	status, err := commandStation.Status(context.TODO())

	require.NoError(t, err)
	require.NotNil(t, status)

	t.Logf("DCC-EX version: %s", status.Version)
	t.Logf("Microprocessor: %s", status.MicroprocessorType)
	t.Logf("Motor controller: %s", status.MotorcontrollerType)
	t.Logf("Build number: %s", status.BuildNumber)

	require.NotEmpty(t, status.Version)
	require.NotEmpty(t, status.MicroprocessorType)
	require.NotEmpty(t, status.MotorcontrollerType)
	require.NotEmpty(t, status.BuildNumber)
}
