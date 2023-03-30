### Alloy software components

Alloy is internally composed of two main components the `Collector` and the `Controller`,
the store is a backend repository of inventory assets.

![Alloy software components](alloy_components.png)

#### Inventory collector

The `Inventory collector` listens for devices on the asset channel (from the asset getter),
and proceeds to collect inventory for the devices received - through `In band` or `Out of band`.

The collector then sends the collected inventory as an `model.AssetDevice` on the
`collector channel`, for the `Inventory publisher` component.

Inventory collectors implement the `Collector` interface.

###### In band collection

In band inventory is collected when alloy is invoked with the `inband` command,
this calls into the [ironlib](https://github.com/metal-toolbox/ironlib) library
which abstracts the hardware/vendor specific data collection through a host OS.

![Alloy software components](alloy_inband.png)


###### Out of band collection

Out of band inventory is collected when Alloy is invoked with the `out of band`
command, which calls into the [bmclib](https://github.com/bmc-toolbox/bmclib/)
library, which abstracts hardware/vendor specific data collection remotely through the
BMC.


![Alloy software components](alloy_oob.png)



#### Debugging/fixture dump environment variables

 Set `DEBUG_DUMP_FIXTURES=true` to have fixture data for `fixtures/device.go`, `fixtures/serverservice_components.*` dumped,
 the objects are dumped to files in the current directory,
 ```
fc167440-18d3-4455-b5ee-1c8e347b3f36.device.fixture             # the device object returned from ironlib/bmclib
fc167440-18d3-4455-b5ee-1c8e347b3f36.current.components.fixture # the current component data from server service
fc167440-18d3-4455-b5ee-1c8e347b3f36.new.components.fixture     # the newer component data based on the device object from ironlib/bmclib
 ```

 Set `DEBUG_DUMP_DIFFERS=true` to have object differ changelogs from the `publish.CreateUpdateServerComponents()` method dumped.