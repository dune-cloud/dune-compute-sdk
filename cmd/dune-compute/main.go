// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

var (
	stdout        io.Writer = os.Stdout
	stderr        io.Writer = os.Stderr
	asJSON        bool
	jobTimeout    = 300 * time.Second
	jobPollDelay  = time.Second
	progressWidth int
	progressShown string
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	asJSON = false
	jobTimeout = 300 * time.Second
	progressWidth = 0
	progressShown = ""
	if len(argv) > 0 && argv[0] == "ssh-proxy" {
		if err := runProxyFromEnv(); err != nil {
			fmt.Fprintln(stderr, errorText(err))
			return 1
		}
		return 0
	}
	parsed, err := splitGlobals(argv)
	if err != nil {
		return usageStatus(err)
	}
	if parsed.help {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	if len(parsed.rest) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	asJSON = parsed.json
	client, apiKey, err := clientFrom(parsed)
	if err != nil {
		return usageStatus(err)
	}
	if err := dispatch(client, apiKey, parsed.insecure, parsed.rest); err != nil {
		if _, ok := err.(usageFailure); ok {
			return usageStatus(err)
		}
		fmt.Fprintln(stderr, errorText(err))
		return 1
	}
	return 0
}

// errorText returns plain text from a non-JSON response.
func errorText(err error) string {
	var plain *dune.PlainError
	if errors.As(err, &plain) {
		return plain.Error()
	}
	return err.Error()
}

func usageStatus(err error) int {
	fmt.Fprintln(stderr, err.Error())
	fmt.Fprintln(stderr, `See "dune-compute --help".`)
	return 2
}

type globalFlags struct {
	apiURL   string
	apiKey   string
	secret   string
	timeout  string
	insecure bool
	json     bool
	help     bool
	rest     []string
}

func splitGlobals(argv []string) (globalFlags, error) {
	parsed := globalFlags{}
	seen := map[string]bool{}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			parsed.rest = append(parsed.rest, argv[i:]...)
			break
		}
		if arg == "-h" || arg == "--help" || (arg == "help" && len(parsed.rest) == 0) {
			parsed.help = true
			continue
		}
		name, value, ok := flagParts(arg)
		if !ok {
			parsed.rest = append(parsed.rest, arg)
			continue
		}
		switch name {
		case "api-url", "api-key", "secret-key", "timeout":
			if seen[name] {
				return globalFlags{}, usageFailure{fmt.Sprintf("--%s was given more than once", name)}
			}
			seen[name] = true
			if value == "" {
				if i+1 >= len(argv) || argv[i+1] == "--" || isFlag(argv[i+1]) {
					return globalFlags{}, usageFailure{fmt.Sprintf("--%s needs a value", name)}
				}
				i++
				value = argv[i]
			}
			if value == "" {
				return globalFlags{}, usageFailure{fmt.Sprintf("--%s needs a value", name)}
			}
			switch name {
			case "api-url":
				parsed.apiURL = value
			case "api-key":
				parsed.apiKey = value
			case "secret-key":
				parsed.secret = value
			case "timeout":
				parsed.timeout = value
			}
		case "insecure":
			if value != "" {
				return globalFlags{}, usageFailure{"--insecure does not take a value"}
			}
			parsed.insecure = true
		case "json":
			if value != "" {
				return globalFlags{}, usageFailure{"--json does not take a value"}
			}
			parsed.json = true
		case "help":
			parsed.help = true
		default:
			parsed.rest = append(parsed.rest, arg)
		}
	}
	return parsed, nil
}

func clientFrom(parsed globalFlags) (*dune.Client, string, error) {
	apiURL := parsed.apiURL
	if apiURL == "" {
		apiURL = os.Getenv("DUNE_COMPUTE_API_URL")
	}
	apiKey := parsed.apiKey
	if apiKey == "" {
		apiKey = os.Getenv("DUNE_COMPUTE_API_KEY")
	}
	secret := parsed.secret
	if secret == "" {
		secret = os.Getenv("DUNE_COMPUTE_SECRET_KEY")
	}
	var opts []dune.Option
	if parsed.insecure {
		opts = append(opts, dune.Insecure())
	}
	if parsed.timeout != "" {
		seconds, err := strconv.ParseInt(parsed.timeout, 10, 64)
		if err != nil || seconds <= 0 {
			return nil, "", usageFailure{"--timeout must be a positive number of seconds"}
		}
		jobTimeout = time.Duration(seconds) * time.Second
		opts = append(opts, dune.Timeout(seconds))
	}
	client, err := dune.New(apiURL, apiKey, secret, opts...)
	if err != nil {
		return nil, "", usageFailure{"DUNE_COMPUTE_API_URL, DUNE_COMPUTE_API_KEY, and DUNE_COMPUTE_SECRET_KEY are required"}
	}
	return client, apiKey, nil
}

func dispatch(client *dune.Client, apiKey string, insecure bool, argv []string) error {
	if len(argv) == 0 {
		return usageFailure{"a command is required"}
	}
	switch argv[0] {
	case "instance":
		return instanceCommand(client, argv[1:])
	case "disk":
		return diskCommand(client, argv[1:])
	case "template":
		return templateCommand(client, argv[1:])
	case "ssh-key":
		return sshKeyCommand(client, argv[1:])
	case "network":
		return listOnly(argv[1:], func() (any, error) {
			return client.Network.List(client.Network.NewListParams())
		})
	case "zone":
		return listOnly(argv[1:], func() (any, error) {
			return client.Zone.List(client.Zone.NewListParams())
		})
	case "service-offering":
		return listOnly(argv[1:], func() (any, error) {
			return client.ServiceOffering.List(client.ServiceOffering.NewListParams())
		})
	case "disk-offering":
		return listOnly(argv[1:], func() (any, error) {
			return client.DiskOffering.List(client.DiskOffering.NewListParams())
		})
	case "job":
		return jobCommand(client, argv[1:])
	case "api":
		return listOnly(argv[1:], func() (any, error) {
			return client.API.List(client.API.NewListParams())
		})
	case "ssh":
		return runSSH(argv[1:], client, apiKey, insecure)
	default:
		return usageFailure{fmt.Sprintf("unknown command %q", argv[0])}
	}
}

const usageText = `Dune Compute CLI

usage: dune-compute <command> [arguments]

Credentials come from DUNE_COMPUTE_API_URL, DUNE_COMPUTE_API_KEY, and
DUNE_COMPUTE_SECRET_KEY. These flags override them for one command:

  --api-url URL
  --api-key KEY
  --secret-key SECRET
  --insecure           skip TLS certificate verification
  --timeout SECONDS    how long to wait for a job
  --json               print the raw JSON response for a query

A name or an ID is accepted wherever an ID is shown below.

  instance list
  instance deploy --offering OFFERING --template TEMPLATE --zone ZONE [--name NAME] [--keypair NAME [--keypair NAME]] [--root-size GB]
  instance destroy ID
  instance expunge ID
  instance start ID
  instance stop ID
  instance update ID --name NAME
  instance change-service ID --offering OFFERING
  disk list
  disk create --name NAME --zone ZONE --offering OFFERING [--size GB]
  disk delete ID
  disk attach ID INSTANCE
  disk detach ID
  disk resize ID --size GB
  disk update ID --name NAME
  template list --filter FILTER
  template register --name NAME --display-text TEXT --format FORMAT --url URL
  template update ID --name NAME
  template delete ID
  template link-user-data --template ID --user-data ID
  ssh-key list
  ssh-key create NAME
  ssh-key register NAME --public-key KEY|PATH
  ssh-key delete NAME
  network list
  zone list
  service-offering list
  disk-offering list
  job query ID
  api list
  ssh INSTANCE [--user USER] [--identity FILE] [-- remote-command]
`
