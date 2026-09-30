// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

func instanceCommand(client *dune.Client, argv []string) error {
	if len(argv) == 0 {
		return usageFailure{"an instance command is required"}
	}
	switch argv[0] {
	case "list":
		return listOnly(argv, func() (any, error) {
			return client.Instance.List(client.Instance.NewListParams())
		})
	case "deploy":
		positional, values, err := take(argv[1:], "offering", "template", "zone", "name", "keypair", "root-size")
		if err != nil {
			return err
		}
		if err := noExtra(positional); err != nil {
			return err
		}
		offering, err := required(values, "offering")
		if err != nil {
			return err
		}
		template, err := required(values, "template")
		if err != nil {
			return err
		}
		zone, err := required(values, "zone")
		if err != nil {
			return err
		}
		name, err := optional(values, "name")
		if err != nil {
			return err
		}
		rootSizeText, err := optional(values, "root-size")
		if err != nil {
			return err
		}
		var rootSize int64
		if rootSizeText != "" {
			rootSize, err = positive(rootSizeText, "root-size")
			if err != nil {
				return err
			}
		}
		offering, err = resolveOffering(client, offering)
		if err != nil {
			return err
		}
		template, err = resolveTemplate(client, template)
		if err != nil {
			return err
		}
		zone, err = resolveZone(client, zone)
		if err != nil {
			return err
		}
		params := client.Instance.NewDeployParams(offering, template, zone)
		if name != "" {
			params.SetName(name)
		}
		if keypairs := values["keypair"]; len(keypairs) > 0 {
			params.SetKeypairs(keypairs)
		}
		if rootSizeText != "" {
			params.SetRootdisksize(rootSize)
		}
		created, err := client.Instance.Deploy(params)
		if err != nil {
			return err
		}
		return finish(client, created, action{doing: "Deploying instance", done: "Deployed instance", target: name})
	case "destroy":
		return withInstance(client, argv[1:], "Destroying instance", "Destroyed instance", func(id string) (any, error) {
			return client.Instance.Destroy(client.Instance.NewDestroyParams(id))
		})
	case "expunge":
		return withInstance(client, argv[1:], "Expunging instance", "Expunged instance", func(id string) (any, error) {
			return client.Instance.Expunge(client.Instance.NewExpungeParams(id))
		})
	case "start":
		return withInstance(client, argv[1:], "Starting instance", "Started instance", func(id string) (any, error) {
			return client.Instance.Start(client.Instance.NewStartParams(id))
		})
	case "stop":
		return withInstance(client, argv[1:], "Stopping instance", "Stopped instance", func(id string) (any, error) {
			return client.Instance.Stop(client.Instance.NewStopParams(id))
		})
	case "update":
		positional, values, err := take(argv[1:], "name")
		if err != nil {
			return err
		}
		token, err := oneArg(positional, "instance id")
		if err != nil {
			return err
		}
		name, err := required(values, "name")
		if err != nil {
			return err
		}
		id, err := resolveInstance(client, token)
		if err != nil {
			return err
		}
		params := client.Instance.NewUpdateParams(id)
		params.SetName(name)
		updated, err := client.Instance.Update(params)
		if err != nil {
			return err
		}
		return finish(client, updated, action{doing: "Updating instance", done: "Updated instance", target: token})
	case "change-service":
		positional, values, err := take(argv[1:], "offering")
		if err != nil {
			return err
		}
		token, err := oneArg(positional, "instance id")
		if err != nil {
			return err
		}
		offering, err := required(values, "offering")
		if err != nil {
			return err
		}
		id, err := resolveInstance(client, token)
		if err != nil {
			return err
		}
		offering, err = resolveOffering(client, offering)
		if err != nil {
			return err
		}
		params := client.Instance.NewChangeServiceParams(id, offering)
		changed, err := client.Instance.ChangeService(params)
		if err != nil {
			return err
		}
		return finish(client, changed, action{doing: "Changing service for instance", done: "Changed service for instance", target: token})
	default:
		return usageFailure{fmt.Sprintf("unknown instance command %q", argv[0])}
	}
}

func diskCommand(client *dune.Client, argv []string) error {
	if len(argv) == 0 {
		return usageFailure{"a disk command is required"}
	}
	switch argv[0] {
	case "list":
		return listOnly(argv, func() (any, error) {
			return client.Disk.List(client.Disk.NewListParams())
		})
	case "create":
		positional, values, err := take(argv[1:], "name", "zone", "offering", "size")
		if err != nil {
			return err
		}
		if err := noExtra(positional); err != nil {
			return err
		}
		name, err := required(values, "name")
		if err != nil {
			return err
		}
		zone, err := required(values, "zone")
		if err != nil {
			return err
		}
		offering, err := required(values, "offering")
		if err != nil {
			return err
		}
		sizeText, err := optional(values, "size")
		if err != nil {
			return err
		}
		zone, err = resolveZone(client, zone)
		if err != nil {
			return err
		}
		offering, err = resolveDiskOffering(client, offering)
		if err != nil {
			return err
		}
		params := client.Disk.NewCreateParams()
		params.SetName(name)
		params.SetZoneid(zone)
		params.SetDiskofferingid(offering)
		if sizeText != "" {
			size, err := positive(sizeText, "size")
			if err != nil {
				return err
			}
			params.SetSize(size)
		}
		created, err := client.Disk.Create(params)
		if err != nil {
			return err
		}
		return finish(client, created, action{doing: "Creating disk", done: "Created disk", target: name})
	case "delete":
		return withDisk(client, argv[1:], "Deleting disk", "Deleted disk", func(id string) (any, error) {
			return client.Disk.Delete(client.Disk.NewDeleteParams(id))
		})
	case "attach":
		positional, _, err := take(argv[1:])
		if err != nil {
			return err
		}
		diskToken, instanceToken, err := twoArgs(positional, "disk id", "instance id")
		if err != nil {
			return err
		}
		diskID, err := resolveDisk(client, diskToken)
		if err != nil {
			return err
		}
		instanceID, err := resolveInstance(client, instanceToken)
		if err != nil {
			return err
		}
		attached, err := client.Disk.Attach(client.Disk.NewAttachParams(diskID, instanceID))
		if err != nil {
			return err
		}
		return finish(client, attached, action{doing: "Attaching disk", done: "Attached disk", target: diskToken + " to " + instanceToken})
	case "detach":
		return withDisk(client, argv[1:], "Detaching disk", "Detached disk", func(id string) (any, error) {
			params := client.Disk.NewDetachParams()
			params.SetId(id)
			return client.Disk.Detach(params)
		})
	case "resize":
		positional, values, err := take(argv[1:], "size")
		if err != nil {
			return err
		}
		token, err := oneArg(positional, "disk id")
		if err != nil {
			return err
		}
		text, err := required(values, "size")
		if err != nil {
			return err
		}
		size, err := positive(text, "size")
		if err != nil {
			return err
		}
		id, err := resolveDisk(client, token)
		if err != nil {
			return err
		}
		params := client.Disk.NewResizeParams(id)
		params.SetSize(size)
		resized, err := client.Disk.Resize(params)
		if err != nil {
			return err
		}
		return finish(client, resized, action{doing: "Resizing disk", done: "Resized disk", target: token})
	case "update":
		positional, values, err := take(argv[1:], "name")
		if err != nil {
			return err
		}
		token, err := oneArg(positional, "disk id")
		if err != nil {
			return err
		}
		name, err := required(values, "name")
		if err != nil {
			return err
		}
		id, err := resolveDisk(client, token)
		if err != nil {
			return err
		}
		params := client.Disk.NewUpdateParams()
		params.SetId(id)
		params.SetName(name)
		updated, err := client.Disk.Update(params)
		if err != nil {
			return err
		}
		return finish(client, updated, action{doing: "Updating disk", done: "Updated disk", target: token})
	default:
		return usageFailure{fmt.Sprintf("unknown disk command %q", argv[0])}
	}
}

func templateCommand(client *dune.Client, argv []string) error {
	if len(argv) == 0 {
		return usageFailure{"a template command is required"}
	}
	switch argv[0] {
	case "list":
		positional, values, err := take(argv[1:], "filter")
		if err != nil {
			return err
		}
		if err := noExtra(positional); err != nil {
			return err
		}
		filter, err := required(values, "filter")
		if err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(filter), "all") {
			return usageFailure{`template filter "all" is not supported`}
		}
		listed, err := client.Template.List(client.Template.NewListParams(filter))
		if err != nil {
			return err
		}
		return present(listed, func() error { return presentList(listed) })
	case "register":
		positional, values, err := take(argv[1:], "name", "display-text", "format", "url")
		if err != nil {
			return err
		}
		if err := noExtra(positional); err != nil {
			return err
		}
		name, err := required(values, "name")
		if err != nil {
			return err
		}
		display, err := required(values, "display-text")
		if err != nil {
			return err
		}
		format, err := required(values, "format")
		if err != nil {
			return err
		}
		rawURL, err := required(values, "url")
		if err != nil {
			return err
		}
		params := client.Template.NewRegisterParams(display, format, name, rawURL)
		registered, err := client.Template.Register(params)
		if err != nil {
			return err
		}
		return finish(client, registered, action{doing: "Registering template", done: "Registered template", target: name})
	case "update":
		positional, values, err := take(argv[1:], "name")
		if err != nil {
			return err
		}
		token, err := oneArg(positional, "template id")
		if err != nil {
			return err
		}
		name, err := required(values, "name")
		if err != nil {
			return err
		}
		id, err := resolveTemplate(client, token)
		if err != nil {
			return err
		}
		params := client.Template.NewUpdateParams(id)
		params.SetName(name)
		updated, err := client.Template.Update(params)
		if err != nil {
			return err
		}
		return finish(client, updated, action{doing: "Updating template", done: "Updated template", target: token})
	case "delete":
		return withTemplate(client, argv[1:], "Deleting template", "Deleted template", func(id string) (any, error) {
			return client.Template.Delete(client.Template.NewDeleteParams(id))
		})
	case "link-user-data":
		positional, values, err := take(argv[1:], "template", "user-data")
		if err != nil {
			return err
		}
		if err := noExtra(positional); err != nil {
			return err
		}
		templateToken, err := required(values, "template")
		if err != nil {
			return err
		}
		userDataID, err := required(values, "user-data")
		if err != nil {
			return err
		}
		templateID, err := resolveTemplate(client, templateToken)
		if err != nil {
			return err
		}
		userDataID, err = resolveUserData(client, userDataID)
		if err != nil {
			return err
		}
		params := client.Template.NewLinkUserDataParams()
		params.SetTemplateid(templateID)
		params.SetUserdataid(userDataID)
		linked, err := client.Template.LinkUserData(params)
		if err != nil {
			return err
		}
		return finish(client, linked, action{doing: "Linking user data to template", done: "Linked user data to template", target: templateToken})
	default:
		return usageFailure{fmt.Sprintf("unknown template command %q", argv[0])}
	}
}

// publicKeyMaterial reads value when it is a file. Any other value is key text.
func publicKeyMaterial(value string) (string, error) {
	info, err := os.Stat(value)
	if err != nil {
		return value, nil
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a file", value)
	}
	body, err := os.ReadFile(value)
	if err != nil {
		return "", err
	}
	text := strings.TrimRight(string(body), "\r\n")
	if text == "" {
		return "", fmt.Errorf("%s is empty", value)
	}
	return text, nil
}

func sshKeyCommand(client *dune.Client, argv []string) error {
	if len(argv) == 0 {
		return usageFailure{"an ssh-key command is required"}
	}
	switch argv[0] {
	case "list":
		return listOnly(argv, func() (any, error) {
			return client.SSHKeyPair.List(client.SSHKeyPair.NewListParams())
		})
	case "create":
		positional, _, err := take(argv[1:])
		if err != nil {
			return err
		}
		name, err := oneArg(positional, "key name")
		if err != nil {
			return err
		}
		created, err := client.SSHKeyPair.Create(client.SSHKeyPair.NewCreateParams(name))
		if err != nil {
			return err
		}
		return finish(client, created, action{doing: "Creating SSH key", done: "Created SSH key", target: name})
	case "register":
		positional, values, err := take(argv[1:], "public-key")
		if err != nil {
			return err
		}
		name, err := oneArg(positional, "key name")
		if err != nil {
			return err
		}
		publicKey, err := required(values, "public-key")
		if err != nil {
			return err
		}
		publicKey, err = publicKeyMaterial(publicKey)
		if err != nil {
			return err
		}
		registered, err := client.SSHKeyPair.Register(client.SSHKeyPair.NewRegisterParams(name, publicKey))
		if err != nil {
			return err
		}
		return finish(client, registered, action{doing: "Registering SSH key", done: "Registered SSH key", target: name})
	case "delete":
		positional, _, err := take(argv[1:])
		if err != nil {
			return err
		}
		name, err := oneArg(positional, "key name")
		if err != nil {
			return err
		}
		deleted, err := client.SSHKeyPair.Delete(client.SSHKeyPair.NewDeleteParams(name))
		if err != nil {
			return err
		}
		return finish(client, deleted, action{doing: "Deleting SSH key", done: "Deleted SSH key", target: name})
	default:
		return usageFailure{fmt.Sprintf("unknown ssh-key command %q", argv[0])}
	}
}

func jobCommand(client *dune.Client, argv []string) error {
	if len(argv) == 0 || argv[0] != "query" {
		return usageFailure{"job query is required"}
	}
	positional, _, err := take(argv[1:])
	if err != nil {
		return err
	}
	id, err := oneArg(positional, "job id")
	if err != nil {
		return err
	}
	result, err := client.Job.Query(client.Job.NewQueryParams(id))
	if err != nil {
		return err
	}
	return present(result, func() error { return writeJob(result) })
}

func withInstance(client *dune.Client, argv []string, doing, done string, call func(string) (any, error)) error {
	return withResolved(client, argv, "instance id", doing, done, resolveInstance, call)
}

func withDisk(client *dune.Client, argv []string, doing, done string, call func(string) (any, error)) error {
	return withResolved(client, argv, "disk id", doing, done, resolveDisk, call)
}

func withTemplate(client *dune.Client, argv []string, doing, done string, call func(string) (any, error)) error {
	return withResolved(client, argv, "template id", doing, done, resolveTemplate, call)
}

func withResolved(client *dune.Client, argv []string, what, doing, done string, resolve func(*dune.Client, string) (string, error), call func(string) (any, error)) error {
	positional, _, err := take(argv)
	if err != nil {
		return err
	}
	token, err := oneArg(positional, what)
	if err != nil {
		return err
	}
	id, err := resolve(client, token)
	if err != nil {
		return err
	}
	result, err := call(id)
	if err != nil {
		return err
	}
	return finish(client, result, action{doing: doing, done: done, target: token})
}
