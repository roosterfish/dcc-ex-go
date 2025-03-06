# dcc-ex-go

This module contains Go bindings for the [DCC-EX](https://dcc-ex.com) native command protocol.
It implements the commands outlined in the [summary](https://dcc-ex.com/reference/software/command-summary-consolidated.html) and
uses Go's language features to easily interact with the various entities available in the [DCC-EX CommandStation](https://dcc-ex.com/ex-commandstation/index.html).

As the underlying serial connection doesn't allow mapping the response(s) to the actual command the module offers the concept of a
channel on which a caller can obtain either a rw (read/write) or ro (read-only) session.
This allows the serialization of commands which expect one or more responses to be sent by DCC-EX.
The logic of this is mostly hidden behind the individual package's functions.

By far not all of the native commands are implemented yet.

## Get started

Start by plugging your [DCC-EX CommandStation](https://dcc-ex.com/ex-commandstation/index.html) into a USB port.
You can now create a new connection using the right device path:

```go
conn, err := connection.NewConnection(connection.NewDefaultConfig("/dev/ttyACM0"))
if err != nil {
    log.Fatalln(err)
}

defer conn.Close()
```

Derive a new instance of the command station to power on the main track and join with the programming track.
But before wait until the station is ready to receive commands:

```go
commandStation := conn.CommandStation()

err = commandStation.Ready(context.Background())
if err != nil {
    log.Fatalln(err)
}

err = commandStation.PowerTrack(station.PowerOn, station.TrackJoin)
if err != nil {
    log.Fatalln(err)
}
```

Set the speed of the locomotive after deriving it from its address:

```go
loc := conn.Cab(3)
err = loc.Speed(70, cab.DirectionForward)
if err != nil {
    log.Fatalln(err)
}
```

And activate function F1:

```go
err = loc.Function(1, cab.FunctionOn)
if err != nil {
    log.Fatalln(err)
}
```

Now wait until it reaches the block with sensor 31:

```go
block := conn.Sensor(31)
err := block.Wait(context.Background(), sensor.SensorStateActive)
if err != nil {
    log.Fatalln(err)
}
```

Or define a callback to be fired every time it leaves the block again:

```go
cleanup := block.SetCallback(sensor.StateInactive, func(id sensor.ID, state sensor.State) {
    log.Println("Sensor went inactive")
})

defer cleanup()
```

## Status information

Retrieve status information from the command station:

```go
status, err := controller.Status(context.Background())
if err != nil {
    log.Fatalln(err)
}

fmt.Printf("Version: %s\n", status.Version)
```

## Direct console access

In case you just want to get access to the console for reading and writing native commands
setup a console:

```go
commandC, writeF, cleanupF := controller.Console()
defer cleanupF()
```

Ingress commands can be consumed from the `commandC` channel.
New commands can be sent using the `writeF` function.
