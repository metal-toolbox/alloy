## Notes on running Alloy with serverservice as the inventory store

### Serverservice server component types data

Server service has to be populated with the component types which Alloy depends on,
for this, check out the following snippet.

```go
package main

import (
    "log"
    "strings"
    "context"

    "github.com/bmc-toolbox/common"
    "github.com/hashicorp/go-retryablehttp"
    serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

func main {
	client, err := serverservice.NewClientWithToken(
		"foobar",
		"http://localhost:5000", // assuming the service is available on this port
		retryablehttp.NewClient().StandardClient(),
	)

	if err != nil {
		log.Fatal(err)
	}

	componentSlugs := []string{
		common.SlugBackplaneExpander,
		common.SlugChassis,
		common.SlugTPM,
		common.SlugGPU,
		common.SlugCPU,
		common.SlugPhysicalMem,
		common.SlugStorageController,
		common.SlugBMC,
		common.SlugBIOS,
		common.SlugDrive,
		common.SlugDriveTypePCIeNVMEeSSD,
		common.SlugDriveTypeSATASSD,
		common.SlugDriveTypeSATAHDD,
		common.SlugNIC,
		common.SlugPSU,
		common.SlugCPLD,
		common.SlugEnclosure,
		common.SlugUnknown,
		common.SlugMainboard,
	}

	for _, slug := range componentSlugs {
		sct := serverservice.ServerComponentType{
			Name: slug,
			Slug: strings.ToLower(slug),
		}

		hr, err := client.CreateServerComponentType(context.TODO(), sct)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(hr)
	}
}
```

### Alloy with a local development instance of serverservice

To have Alloy submit data to serverservice, various configuration env variables are expected to be exported,
among them are the OAUTH related env variables. 


To auth with a development instance of serverservice, include the env var `SERVERSERVICE_SKIP_OAUTH=true` .

```
SERVERSERVICE_SKIP_OAUTH=true
SERVERSERVICE_FACILITY_CODE="ld7"
SERVERSERVICE_AUTH_TOKEN="asd"
SERVERSERVICE_ENDPOINT="http://localhost:8000"

alloy outofband -asset-source serverService \
                -publish-target serverService \
                -asset-ids 023bd72d-f032-41fc-b7ca-3ef044cd33d5 \
                -collect-interval 1h -trace
```

TODO:(joel) link to server service development env helm charts.