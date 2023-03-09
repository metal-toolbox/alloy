

The current Alloy code layout was based on the idea of a pipeline model

 [asset-getter] -> [collector] -> [publisher]

 While this model currently functions, it is not going to be maintainable over time, the reasons for this is outlined below.

## inter dependence

 Each of the components - the `asset getter`, `collector`, `publisher` while separate, heavily depend on each other to function.

 The publisher depends on the collector and will exit if the collector closes its channel.

 The collector depends on the asset-getter and will exit if the asset-getter closes its channel.

## code duplication

The asset getter and the publisher speak to serverservice and they don't share
the logic thats similar between them.

## Go routine management

Since each of these are separate components, they spawn their own Go routines
and have to communicate a stall through channels, while this works, its also
non-intuitive to understand and maintain.


## Channels

Channels are shared between the components to pass assets through,
and since each component has to be spawned on its own, the spawn and execution
of each component is ambiguious, and harder to control in a central manner.