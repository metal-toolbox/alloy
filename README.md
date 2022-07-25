### Alloy - hardware inventory collector.

Alloy collects and publishes server hardware inventory.

Hardware inventory includes information on the hardware components present on a server,
the firmware versions installed and the component health status.

Inventory collection with Alloy can be executed in two modes,
 - `In band` - the alloy command is executed on the target host OS.
 - `Out of band` - the alloy command is executed on a remote system that can reach the target BMC.

The `outofband` command will cause Alloy to collect inventory from the server BMC.

The command requires BMC credential information provided by the `-asset-source` flag,
see [examples](examples/assets.csv).

The command also requires the `-publish-target`, which must be either `stdout` or `serverService`.

For Alloy internals see [README-development.md](docs/README-development.md)

##### sample commands

CSV file asset source with inventory published to stdout
```
./alloy outofband  -asset-source csv \
                   -csv-file examples/assets.csv \
                   -publish-target stdout
```

CSV file asset source with inventory published to serverService
```
export SERVERSERVICE_AUTH_TOKEN="hunter2"
export SERVERSERVICE_ENDPOINT="http://127.0.0.1:8000"

./alloy outofband  -asset-source csv \
                   -csv-file examples/assets.csv \
                   -publish-target serverService
```


EMAPI as an asset source with inventory published to stdout.

In this case the asset id is passed to the `--list` flag, and the `-config-file` parameter is required.
```
alloy outofband  -asset-source emapi \
                 -publish-target stdout \
                 -config-file examples/alloy.yaml \
                 --list fc167440-18d3-4455-b5ee-1c8e347b3f36
```

### Alloy commands

```
❯ ./alloy --help
USAGE
  alloy [inband|outofband] [flags]

SUBCOMMANDS
  outofband  outofband command collects asset inventory out of band
  inband     inband command runs on target hardware to collect inventory inband

FLAGS
  -config-file ...     Alloy config file
  -debug=false         Set logging to debug level.
  -publish-target ...  Publish collected inventory to [serverService|stdout]
  -trace=false         Set logging to trace level.
```
