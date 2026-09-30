// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

type sshArgs struct {
	vmID     string
	user     string
	identity string
	remote   []string
}

func parseSSHArgs(argv []string) (sshArgs, error) {
	remote := []string{}
	if split := indexOf(argv, "--"); split >= 0 {
		remote = append(remote, argv[split+1:]...)
		argv = argv[:split]
	}
	parsed := sshArgs{user: "ubuntu"}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "--user":
			if i+1 >= len(argv) {
				return sshArgs{}, usageFailure{"--user needs a value"}
			}
			parsed.user = argv[i+1]
			i++
		case strings.HasPrefix(arg, "--user="):
			parsed.user = strings.TrimPrefix(arg, "--user=")
			if parsed.user == "" {
				return sshArgs{}, usageFailure{"--user needs a value"}
			}
		case arg == "--identity":
			if i+1 >= len(argv) {
				return sshArgs{}, usageFailure{"--identity needs a file"}
			}
			parsed.identity = argv[i+1]
			i++
		case strings.HasPrefix(arg, "--identity="):
			parsed.identity = strings.TrimPrefix(arg, "--identity=")
			if parsed.identity == "" {
				return sshArgs{}, usageFailure{"--identity needs a file"}
			}
		case strings.HasPrefix(arg, "-"):
			return sshArgs{}, usageFailure{fmt.Sprintf("unknown option %s", arg)}
		default:
			if parsed.vmID != "" {
				return sshArgs{}, usageFailure{"only one instance id is accepted"}
			}
			parsed.vmID = arg
		}
	}
	if parsed.vmID == "" {
		return sshArgs{}, usageFailure{"an instance id is required"}
	}
	if parsed.user == "" {
		return sshArgs{}, usageFailure{"--user needs a value"}
	}
	parsed.remote = remote
	return parsed, nil
}

func sshCommand(executable string, args sshArgs) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	known := filepath.Join(home, ".dune-compute", "known_hosts")
	if err := os.MkdirAll(filepath.Dir(known), 0o700); err != nil {
		return nil, err
	}
	proxy := shellQuote(executable) + " ssh-proxy"
	command := []string{
		"ssh",
		"-o", "ProxyCommand=" + proxy,
		"-o", "UserKnownHostsFile=" + known,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "PreferredAuthentications=publickey",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-l", args.user,
		args.vmID,
	}
	if args.identity != "" {
		command = append([]string{command[0], "-i", args.identity}, command[1:]...)
	}
	if stdinIsTTY() && len(args.remote) == 0 {
		command = append(command, "-t")
	}
	if len(args.remote) > 0 {
		command = append(command, "--")
		command = append(command, args.remote...)
	}
	return command, nil
}

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func prepareSSH(client *dune.Client, instanceID, apiKey string, insecure bool) error {
	if apiKey == "" || strings.ContainsAny(apiKey, "\r\n") {
		return fmt.Errorf("ssh endpoint: api key is unusable")
	}
	endpoint, err := client.SSH.CreateEndpoint(instanceID)
	if err != nil {
		return fmt.Errorf("ssh endpoint: %w", err)
	}
	if err := os.Setenv("DUNE_COMPUTE_SSH_WS_URL", endpoint.WebSocketURL); err != nil {
		return err
	}
	if err := os.Setenv("DUNE_COMPUTE_SSH_TOKEN", endpoint.Token); err != nil {
		return err
	}
	if err := os.Setenv("DUNE_COMPUTE_SSH_TOKEN_HEADER", endpoint.TokenHeader); err != nil {
		return err
	}
	if err := os.Setenv("DUNE_COMPUTE_API_KEY", apiKey); err != nil {
		return err
	}
	if insecure {
		return os.Setenv("DUNE_COMPUTE_SSH_INSECURE", "1")
	}
	return os.Unsetenv("DUNE_COMPUTE_SSH_INSECURE")
}

func runSSH(argv []string, client *dune.Client, apiKey string, insecure bool) error {
	args, err := parseSSHArgs(argv)
	if err != nil {
		return err
	}
	instanceID, err := resolveInstance(client, args.vmID)
	if err != nil {
		return err
	}
	if err := prepareSSH(client, instanceID, apiKey, insecure); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command, err := sshCommand(executable, args)
	if err != nil {
		return err
	}
	bin, err := exec.LookPath("ssh")
	if err != nil {
		return err
	}
	command[0] = bin
	return syscall.Exec(bin, command, os.Environ())
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
