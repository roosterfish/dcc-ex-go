# dcc-ex-go

This module contains Go bindings for the [DCC-EX](https://dcc-ex.com) native command protocol.
It implements the commands outlined in the [summary](https://dcc-ex.com/reference/software/command-summary-consolidated.html) and
uses Go's language features to easily interact with the various entities available in the [DCC-EX CommandStation](https://dcc-ex.com/ex-commandstation/index.html).

Not all of the native commands are implemented yet.

## Get started

Start by plugging your [DCC-EX CommandStation](https://dcc-ex.com/ex-commandstation/index.html) into a USB port.
You can now create a new connection using the right device path:

```go
conn, err := connection.NewConnection("/dev/ttyACM0", connection.DefaultMode)
if err != nil {
    log.Fatalln(err)
}

defer conn.Close()
```

Derive a new instance of the command station to power on the main track and join with the programming track:

```go
commandStation := conn.CommandStation()
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
