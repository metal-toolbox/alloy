## Notes on running Alloy with serverservice as the inventory store


### Alloy with a local development instance of serverservice

To have Alloy submit data to serverservice, various configuration env variables are expected to be exported, among them are the OAUTH related env variables.


To auth with a development instance of serverservice, include the env var `SERVERSERVICE_DISABLE_OAUTH=true` .

```
SERVERSERVICE_DISABLE_OAUTH=true
SERVERSERVICE_FACILITY_CODE="ld7"
SERVERSERVICE_AUTH_TOKEN="asd"
SERVERSERVICE_ENDPOINT="http://localhost:8000"

alloy outofband -store serverservice \
                -asset-ids 023bd72d-f032-41fc-b7ca-3ef044cd33d5 \
                -collect-interval 1h -trace
```

TODO:(joel) link to server service development env helm charts.