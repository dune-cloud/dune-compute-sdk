// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"fmt"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

type named struct {
	id   string
	name string
}

// resolveID looks up input as a name. A single exact match returns that
// resource's ID. No match returns input unchanged, so a raw ID still works.
// The name wins when the same text is both a name and an ID.
func resolveID(input, kind string, list func() ([]named, error)) (string, error) {
	items, err := list()
	if err != nil {
		return "", err
	}
	id, ok, err := pickNamed(input, kind, items)
	if err != nil || !ok {
		if err != nil {
			return "", err
		}
		return input, nil
	}
	return id, nil
}

func pickNamed(input, kind string, items []named) (string, bool, error) {
	seen := map[string]bool{}
	matches := []named{}
	for _, item := range items {
		if item.name != input || item.id == "" || seen[item.id] {
			continue
		}
		seen[item.id] = true
		matches = append(matches, item)
	}
	switch len(matches) {
	case 0:
		return "", false, nil
	case 1:
		return matches[0].id, true, nil
	default:
		return "", false, fmt.Errorf("%d %s are named %q", len(matches), kind, input)
	}
}

func resolveOffering(client *dune.Client, input string) (string, error) {
	return resolveID(input, "service offerings", func() ([]named, error) {
		params := client.ServiceOffering.NewListParams()
		params.SetName(input)
		listed, err := client.ServiceOffering.List(params)
		if err != nil {
			return nil, err
		}
		return namesOf(listed.ServiceOfferings, func(item *dune.ServiceOffering) (string, string) {
			return item.Id, item.Name
		}), nil
	})
}

func resolveDiskOffering(client *dune.Client, input string) (string, error) {
	return resolveID(input, "disk offerings", func() ([]named, error) {
		params := client.DiskOffering.NewListParams()
		params.SetName(input)
		listed, err := client.DiskOffering.List(params)
		if err != nil {
			return nil, err
		}
		return namesOf(listed.DiskOfferings, func(item *dune.DiskOffering) (string, string) {
			return item.Id, item.Name
		}), nil
	})
}

func resolveZone(client *dune.Client, input string) (string, error) {
	return resolveID(input, "zones", func() ([]named, error) {
		params := client.Zone.NewListParams()
		params.SetName(input)
		listed, err := client.Zone.List(params)
		if err != nil {
			return nil, err
		}
		return namesOf(listed.Zones, func(item *dune.Zone) (string, string) {
			return item.Id, item.Name
		}), nil
	})
}

func resolveInstance(client *dune.Client, input string) (string, error) {
	return resolveID(input, "instances", func() ([]named, error) {
		params := client.Instance.NewListParams()
		params.SetName(input)
		listed, err := client.Instance.List(params)
		if err != nil {
			return nil, err
		}
		return namesOf(listed.Instances, func(item *dune.Instance) (string, string) {
			return item.Id, item.Name
		}), nil
	})
}

func resolveDisk(client *dune.Client, input string) (string, error) {
	return resolveID(input, "disks", func() ([]named, error) {
		params := client.Disk.NewListParams()
		params.SetName(input)
		listed, err := client.Disk.List(params)
		if err != nil {
			return nil, err
		}
		return namesOf(listed.Disks, func(item *dune.Disk) (string, string) {
			return item.Id, item.Name
		}), nil
	})
}

func resolveNetwork(client *dune.Client, input string) (string, error) {
	return resolveID(input, "networks", func() ([]named, error) {
		params := client.Network.NewListParams()
		params.SetName(input)
		listed, err := client.Network.List(params)
		if err != nil {
			return nil, err
		}
		return namesOf(listed.Networks, func(item *dune.Network) (string, string) {
			return item.Id, item.Name
		}), nil
	})
}

func resolveTemplate(client *dune.Client, input string) (string, error) {
	return resolveID(input, "templates", func() ([]named, error) {
		return templatesNamed(client, input)
	})
}

func templatesNamed(client *dune.Client, input string) ([]named, error) {
	var found []named
	var lastErr error
	succeeded := 0
	for _, filter := range []string{"executable", "self", "featured", "community"} {
		params := client.Template.NewListParams(filter)
		params.SetName(input)
		listed, err := client.Template.List(params)
		if err != nil {
			lastErr = err
			continue
		}
		succeeded++
		batch := namesOf(listed.Templates, func(item *dune.Template) (string, string) {
			return item.Id, item.Name
		})
		exact := 0
		for _, item := range batch {
			if item.name == input {
				exact++
			}
		}
		found = append(found, batch...)
		if exact > 0 {
			break
		}
	}
	if succeeded == 0 && lastErr != nil {
		return nil, lastErr
	}
	return found, nil
}

func resolveUserData(client *dune.Client, input string) (string, error) {
	return resolveID(input, "user data", func() ([]named, error) {
		params := client.UserData.NewListParams()
		params.SetName(input)
		listed, err := client.UserData.List(params)
		if err != nil {
			return nil, err
		}
		return namesOf(listed.UserData, func(item *dune.UserData) (string, string) {
			return item.Id, item.Name
		}), nil
	})
}

func namesOf[T any](items []*T, idName func(*T) (string, string)) []named {
	out := make([]named, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		id, name := idName(item)
		out = append(out, named{id: id, name: name})
	}
	return out
}
