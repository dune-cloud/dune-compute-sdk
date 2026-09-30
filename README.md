# Dune Compute SDK

Go client for the Dune API. It is a thin wrapper over [cloudstack-go](https://github.com/apache/cloudstack-go). It manages instances, disks, templates, and SSH key pairs, looks up networks, zones, and offerings, and opens an SSH session to an instance.

## Quick start

Go 1.23 or later.

```bash
go get github.com/dune-cloud/dune-compute-sdk
```

```go
package main

import (
	"fmt"
	"log"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

func main() {
	client, err := dune.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	listed, err := client.Instance.List(client.Instance.NewListParams())
	if err != nil {
		log.Fatal(err)
	}
	for _, instance := range listed.Instances {
		fmt.Println(instance.Id, instance.Name, instance.State)
	}
}
```

`FromEnv` reads `DUNE_COMPUTE_API_URL`, `DUNE_COMPUTE_API_KEY`, and `DUNE_COMPUTE_SECRET_KEY`. `DUNE_COMPUTE_API_URL` is the full API URL. The same values can be passed to `New`:

```go
client, err := dune.New(apiURL, apiKey, secretKey)
```

`New` returns when the API accepts the call. `NewAsyncClient` waits until an asynchronous job, such as deploy or destroy, finishes or times out. `Timeout` sets that wait in seconds. `Insecure` skips TLS certificate verification.

Deploy an instance:

```go
params := client.Instance.NewDeployParams(serviceOfferingID, templateID, zoneID)
params.SetName("web")
created, err := client.Instance.Deploy(params)
```

Open an SSH session:

```go
endpoint, err := client.SSH.CreateEndpoint(instanceID)
if err != nil {
	log.Fatal(err)
}
```

Dial `endpoint.WebSocketURL` and send `endpoint.Token` in the `endpoint.TokenHeader` request header. An `https` API URL becomes a `wss` WebSocket URL. The token is single-use, lasts about a minute, and is not part of the URL. The caller opens the WebSocket; the instance private key stays with the caller.

## CLI

```bash
go install github.com/dune-cloud/dune-compute-sdk/cmd/dune-compute@latest
```

The CLI uses the same three environment variables. Flags override them for one command. Use either name or ID to specify a resource. 

```bash
dune-compute instance list
dune-compute --json instance list
dune-compute instance deploy --offering OFFERING --template TEMPLATE --zone ZONE --name web --keypair KEY --root-size 40
dune-compute ssh INSTANCE --identity ~/.ssh/id_ed25519
```

`dune-compute --help` lists the commands.

## Tests

```bash
go test ./...
```

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
