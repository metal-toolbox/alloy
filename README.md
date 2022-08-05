### Alloy - hardware inventory collector.

Alloy collects and publishes server hardware inventory.

Hardware inventory includes information on the hardware components present on a server,
the firmware versions installed and the component health status.

Inventory collection with Alloy can be executed in two modes,
 - `In band` - the alloy command is executed on the target host OS.
 - `Out of band` - the alloy command is executed on a remote system that can reach the target BMC.

The `outofband` command will cause Alloy to collect inventory from the server BMC.

Assets are fetched from an `asset source`, this is defined by the by the `-asset-source` flag,
see [examples](examples/assets.csv). Accepted `-asset-source` parameters are `csv` and `serverService`.

Collected inventory is published to a `publish target`, this is defined by the `-publish-target` flag. Accepted `-publish-target` parameters are `stdout` or `serverService`.

For Alloy internals see [README-development.md](docs/README-development.md)

##### build Alloy

a. build linux executable
`make build-linux`

b. build osx executable
`make build-osx`

##### sample commands

1. CSV file asset source with inventory published to `stdout`
```
./alloy outofband  -asset-source csv \
                   -csv-file examples/assets.csv \
                   -publish-target stdout
```

2. CSV file asset source with inventory published to `serverService`
```
export SERVERSERVICE_AUTH_TOKEN="hunter2"
export SERVERSERVICE_ENDPOINT="http://127.0.0.1:8000"

./alloy outofband  -asset-source csv \
                   -csv-file examples/assets.csv \
                   -publish-target serverService
```


3. ServerService as an asset source with inventory published to `stdout`.

In this case the asset id is passed to the `-asset-ids` flag.
```
export SERVERSERVICE_FACILITY_CODE="ld7"
export SERVERSERVICE_AUTH_TOKEN="asd"
export SERVERSERVICE_ENDPOINT="http://localhost:8000"

alloy outofband -asset-source serverService \
                -asset-ids fc167440-18d3-4455-b5ee-1c8e347b3f36
                -publish-target stdout
```

3. ServerService as an asset source and target.


```
export SERVERSERVICE_FACILITY_CODE="ld7"
export SERVERSERVICE_AUTH_TOKEN="asd"
export SERVERSERVICE_ENDPOINT="http://localhost:8000"

alloy outofband -asset-source serverService \
                -publish-target serverService
```

4. ServerService as an asset source and target, collect inventory at the given interval.

```
SERVERSERVICE_FACILITY_CODE="ld7"
SERVERSERVICE_AUTH_TOKEN="asd"
SERVERSERVICE_ENDPOINT="http://localhost:8000"

alloy outofband -asset-source serverService \
                -publish-target serverService \
                -asset-ids 023bd72d-f032-41fc-b7ca-3ef044cd33d5 \
                --collect-interval 1h --trace
```

### Metrics and traces

Go runtime and Alloy metrics are exposed on `localhost:9090/metrics`.

Telementry can be collected by setting env variables to point to the
opentelemetry collector like Jaeger.

```
export OTEL_EXPORTER_OTLP_ENDPOINT=jaeger:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```

### Alloy commands

```
❯ ./alloy
USAGE
  alloy [inband|outofband] [flags]

SUBCOMMANDS
  outofband  outofband command collects asset inventory out of band
  inband     inband command runs on target hardware to collect inventory inband

FLAGS
  -config-file ...     Alloy config file
  -debug=false         Set logging to debug level.
  -profile=false       Enable performance profile endpoint.
  -publish-target ...  Publish collected inventory to [serverService|stdout]
  -trace=false         Set logging to trace level.

flag: help requested
```
