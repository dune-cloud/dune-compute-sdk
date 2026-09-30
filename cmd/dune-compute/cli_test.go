// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/dune-cloud/dune-compute-sdk/dune"
)

func TestHelpListsTheSurface(t *testing.T) {
	code, out, errOut := capture(t, []string{"--help"})
	if code != 0 || errOut != "" {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	for _, phrase := range []string{
		"dune-compute",
		"DUNE_COMPUTE_API_URL",
		"instance deploy",
		"disk attach",
		"ssh-key register",
		"ssh INSTANCE",
		"--json",
		"name or an ID",
	} {
		if !strings.Contains(out, phrase) {
			t.Fatalf("help missing %q", phrase)
		}
	}
	if strings.Contains(out, "ssh-proxy") || strings.Contains(out, "deployVirtualMachine") {
		t.Fatalf("help exposes an internal name:\n%s", out)
	}
	code, out, errOut = capture(t, nil)
	if code != 2 || !strings.Contains(errOut, "instance deploy") || out != "" {
		t.Fatalf("no-args code %d out %q err %q", code, out, errOut)
	}
}

func TestMissingCredentialsAreUsage(t *testing.T) {
	t.Setenv("DUNE_COMPUTE_API_URL", "")
	t.Setenv("DUNE_COMPUTE_API_KEY", "")
	t.Setenv("DUNE_COMPUTE_SECRET_KEY", "")
	code, _, errOut := capture(t, []string{"instance", "list"})
	if code != 2 || !strings.Contains(errOut, "DUNE_COMPUTE_API_URL") {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
}

func TestFlagsOverrideTheEnvironment(t *testing.T) {
	fake := newCommandAPI(t)
	t.Setenv("DUNE_COMPUTE_API_URL", "https://wrong.example/client/api")
	t.Setenv("DUNE_COMPUTE_API_KEY", "WRONG")
	t.Setenv("DUNE_COMPUTE_SECRET_KEY", "wrong")
	code, _, errOut := capture(t, []string{
		"--api-url", fake.url,
		"--api-key", "KEY",
		"--secret-key", "secret",
		"--insecure",
		"zone", "list",
	})
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	if fake.paramsFor("listZones") == nil {
		t.Fatal("zone list did not reach the flagged endpoint")
	}
}

func TestPlainTextResponseIsRelayed(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "plain text response\n")
	}))
	t.Cleanup(server.Close)
	t.Setenv("DUNE_COMPUTE_API_URL", server.URL)
	t.Setenv("DUNE_COMPUTE_API_KEY", "KEY")
	t.Setenv("DUNE_COMPUTE_SECRET_KEY", "secret")
	code, out, errOut := capture(t, []string{"--insecure", "zone", "list"})
	if code != 1 || out != "" || errOut != "plain text response\n" {
		t.Fatalf("code %d stdout %q stderr %q", code, out, errOut)
	}
}

func TestRegisterSSHKeyReadsAPublicKeyFile(t *testing.T) {
	fake := newCommandAPI(t)
	useServer(t, fake)
	dir := t.TempDir()
	path := filepath.Join(dir, "id_ed25519.pub")
	const key = "ssh-ed12345 AAATestKey user@host"
	if err := os.WriteFile(path, []byte(key+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := capture(t, []string{"--insecure", "ssh-key", "register", "ops", "--public-key", path})
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	params := fake.paramsFor("registerSSHKeyPair")
	if params == nil || params.Get("name") != "ops" || params.Get("publickey") != key {
		t.Fatalf("params = %v", params)
	}

	missing := filepath.Join(dir, "missing.pub")
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "ssh-key", "register", "ops", "--public-key", missing})
	if code != 0 {
		t.Fatalf("missing path code %d stderr %s", code, errOut)
	}
	params = fake.paramsFor("registerSSHKeyPair")
	if params == nil || params.Get("publickey") != missing {
		t.Fatalf("missing path params = %v", params)
	}

	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "ssh-key", "register", "ops", "--public-key", dir})
	if code != 1 || !strings.Contains(errOut, "not a file") || len(fake.commands()) != 0 {
		t.Fatalf("directory code %d stderr %s commands %v", code, errOut, fake.commands())
	}

	empty := filepath.Join(dir, "empty.pub")
	if err := os.WriteFile(empty, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "ssh-key", "register", "ops", "--public-key", empty})
	if code != 1 || !strings.Contains(errOut, "empty") || len(fake.commands()) != 0 {
		t.Fatalf("empty file code %d stderr %s commands %v", code, errOut, fake.commands())
	}
}

func TestUsageDoesNotCallTheAPI(t *testing.T) {
	fake := newCommandAPI(t)
	useServer(t, fake)
	code, _, errOut := capture(t, []string{"instance", "deploy", "--offering", "off"})
	if code != 2 || !strings.Contains(errOut, "--template") {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	code, _, errOut = capture(t, []string{"disk", "create", "--name", "data", "--zone", "zone"})
	if code != 2 || !strings.Contains(errOut, "--offering") {
		t.Fatalf("disk code %d stderr %s", code, errOut)
	}
	if len(fake.commands()) != 0 {
		t.Fatalf("usage called %v", fake.commands())
	}
	code, _, errOut = capture(t, []string{"volume", "list"})
	if code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	code, _, errOut = capture(t, []string{"--timeout", "0", "zone", "list"})
	if code != 2 || !strings.Contains(errOut, "--timeout") {
		t.Fatalf("timeout code %d stderr %s", code, errOut)
	}
	code, _, errOut = capture(t, []string{"template", "list", "--filter", "all"})
	if code != 2 || !strings.Contains(errOut, `template filter "all"`) {
		t.Fatalf("filter code %d stderr %s", code, errOut)
	}
	if len(fake.commands()) != 0 {
		t.Fatalf("template filter all called %v", fake.commands())
	}
	for _, size := range []string{"0", "-4", "3.5", "40GB"} {
		code, _, errOut = capture(t, []string{"instance", "deploy", "--offering", "off", "--template", "tmpl", "--zone", "zone", "--root-size", size})
		if code != 2 || !strings.Contains(errOut, "--root-size must be a positive integer") {
			t.Fatalf("root size %q code %d stderr %s", size, code, errOut)
		}
	}
	if len(fake.commands()) != 0 {
		t.Fatalf("invalid root size called %v", fake.commands())
	}
}

func TestCommandsCallTheAPI(t *testing.T) {
	fake := newCommandAPI(t)
	useServer(t, fake)
	cases := []struct {
		name    string
		args    []string
		command string
		params  map[string]string
		output  string
	}{
		{"list instances", []string{"instance", "list"}, "listVirtualMachines", nil, ""},
		{"deploy", []string{"instance", "deploy", "--offering", "off", "--template", "tmpl", "--zone", "zone", "--name", "web"}, "deployVirtualMachine", map[string]string{"serviceofferingid": "off", "templateid": "tmpl", "zoneid": "zone", "name": "web"}, "Deployed instance web"},
		{"deploy with keypairs", []string{"instance", "deploy", "--offering", "off", "--template", "tmpl", "--zone", "zone", "--keypair", "ops", "--keypair", "ops-2"}, "deployVirtualMachine", map[string]string{"serviceofferingid": "off", "templateid": "tmpl", "zoneid": "zone", "keypairs": "ops,ops-2"}, "Deployed instance web"},
		{"deploy with root size", []string{"instance", "deploy", "--offering", "off", "--template", "tmpl", "--zone", "zone", "--root-size", "40"}, "deployVirtualMachine", map[string]string{"serviceofferingid": "off", "templateid": "tmpl", "zoneid": "zone", "rootdisksize": "40"}, "Deployed instance web"},
		{"destroy", []string{"instance", "destroy", "i-1"}, "destroyVirtualMachine", map[string]string{"id": "i-1"}, "Destroyed instance i-1"},
		{"expunge", []string{"instance", "expunge", "i-1"}, "expungeVirtualMachine", map[string]string{"id": "i-1"}, "Expunged instance i-1"},
		{"start", []string{"instance", "start", "i-1"}, "startVirtualMachine", map[string]string{"id": "i-1"}, "Started instance i-1"},
		{"stop", []string{"instance", "stop", "i-1"}, "stopVirtualMachine", map[string]string{"id": "i-1"}, "Stopped instance i-1"},
		{"update instance", []string{"instance", "update", "i-1", "--name", "renamed"}, "updateVirtualMachine", map[string]string{"id": "i-1", "name": "renamed"}, "Updated instance i-1"},
		{"change service", []string{"instance", "change-service", "i-1", "--offering", "large"}, "changeServiceForVirtualMachine", map[string]string{"id": "i-1", "serviceofferingid": "large"}, "Changed service for instance i-1"},
		{"list disks", []string{"disk", "list"}, "listVolumes", nil, ""},
		{"create disk", []string{"disk", "create", "--name", "data", "--zone", "zone", "--offering", "local.custom", "--size", "5120"}, "createVolume", map[string]string{"name": "data", "zoneid": "zone", "diskofferingid": "local.custom", "size": "5120"}, "Created disk data"},
		{"delete disk", []string{"disk", "delete", "vol-1"}, "deleteVolume", map[string]string{"id": "vol-1"}, "Deleted disk vol-1"},
		{"attach disk", []string{"disk", "attach", "vol-1", "i-1"}, "attachVolume", map[string]string{"id": "vol-1", "virtualmachineid": "i-1"}, "Attached disk vol-1 to i-1"},
		{"detach disk", []string{"disk", "detach", "vol-1"}, "detachVolume", map[string]string{"id": "vol-1"}, "Detached disk vol-1"},
		{"resize disk", []string{"disk", "resize", "vol-1", "--size", "20"}, "resizeVolume", map[string]string{"id": "vol-1", "size": "20"}, "Resized disk vol-1"},
		{"update disk", []string{"disk", "update", "vol-1", "--name", "data"}, "updateVolume", map[string]string{"id": "vol-1", "name": "data"}, "Updated disk vol-1"},
		{"list templates", []string{"template", "list", "--filter", "executable"}, "listTemplates", map[string]string{"templatefilter": "executable"}, ""},
		{"register template", []string{"template", "register", "--name", "img", "--display-text", "Image", "--format", "QCOW2", "--url", "https://example/t"}, "registerTemplate", map[string]string{"name": "img", "displaytext": "Image", "format": "QCOW2", "hypervisor": "KVM", "url": "https://example/t"}, "Registered template img"},
		{"update template", []string{"template", "update", "t-1", "--name", "img"}, "updateTemplate", map[string]string{"id": "t-1", "name": "img"}, "Updated template t-1"},
		{"delete template", []string{"template", "delete", "t-1"}, "deleteTemplate", map[string]string{"id": "t-1"}, "Deleted template t-1"},
		{"link user data", []string{"template", "link-user-data", "--template", "t-1", "--user-data", "u-1"}, "linkUserDataToTemplate", map[string]string{"templateid": "t-1", "userdataid": "u-1"}, "Linked user data to template t-1"},
		{"list keys", []string{"ssh-key", "list"}, "listSSHKeyPairs", nil, ""},
		{"create key", []string{"ssh-key", "create", "ops"}, "createSSHKeyPair", map[string]string{"name": "ops"}, "Created SSH key ops"},
		{"register key", []string{"ssh-key", "register", "ops", "--public-key", "ssh-ed25519 AAAA"}, "registerSSHKeyPair", map[string]string{"name": "ops", "publickey": "ssh-ed25519 AAAA"}, "Registered SSH key ops"},
		{"delete key", []string{"ssh-key", "delete", "ops"}, "deleteSSHKeyPair", map[string]string{"name": "ops"}, "Deleted SSH key ops"},
		{"list networks", []string{"network", "list"}, "listNetworks", nil, ""},
		{"list zones", []string{"zone", "list"}, "listZones", nil, ""},
		{"list service offerings", []string{"service-offering", "list"}, "listServiceOfferings", nil, ""},
		{"list disk offerings", []string{"disk-offering", "list"}, "listDiskOfferings", nil, ""},
		{"query job", []string{"job", "query", "job-9"}, "queryAsyncJobResult", map[string]string{"jobid": "job-9"}, "succeeded"},
		{"list apis", []string{"api", "list"}, "listApis", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake.reset()
			code, out, errOut := capture(t, append([]string{"--insecure"}, tc.args...))
			if code != 0 {
				t.Fatalf("code %d stderr %s stdout %s", code, errOut, out)
			}
			if strings.Contains(out, "{") {
				t.Fatalf("stdout is JSON: %s", out)
			}
			if tc.output == "" {
				if out != "" {
					t.Fatalf("expected no output, stdout = %q", out)
				}
			} else if !strings.Contains(out, tc.output) {
				t.Fatalf("stdout = %s", out)
			}
			params := fake.paramsFor(tc.command)
			if params == nil {
				t.Fatalf("commands = %v", fake.commands())
			}
			for key, value := range tc.params {
				if params.Get(key) != value {
					t.Fatalf("%s = %q, params %v", key, params.Get(key), params)
				}
			}
			if tc.command == "deployVirtualMachine" && tc.params["rootdisksize"] == "" && params.Get("rootdisksize") != "" {
				t.Fatalf("rootdisksize = %q, params %v", params.Get("rootdisksize"), params)
			}
		})
	}
}

func TestSSHCommandKeepsTheTokenOutOfArgv(t *testing.T) {
	args, err := parseSSHArgs([]string{"--identity", "id_ed25519", "--user", "ubuntu", "i-123", "--", "echo", "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if args.vmID != "i-123" || args.user != "ubuntu" || args.identity != "id_ed25519" || len(args.remote) != 2 {
		t.Fatalf("args = %+v", args)
	}
	command, err := sshCommand("/usr/bin/dune-compute", args)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(command, "\n")
	if !strings.Contains(text, "ssh-proxy") || strings.Contains(strings.ToLower(text), "token") {
		t.Fatalf("command = %v", command)
	}
	if !strings.Contains(text, ".dune-compute/known_hosts") {
		t.Fatalf("known hosts = %v", command)
	}
	if command[len(command)-3] != "--" || command[len(command)-2] != "echo" || command[len(command)-1] != "ok" {
		t.Fatalf("remote = %v", command)
	}
	interactive, err := sshCommand("/usr/bin/dune-compute", sshArgs{vmID: "i-123", user: "ubuntu"})
	if err != nil {
		t.Fatal(err)
	}
	hasT := false
	for _, arg := range interactive {
		if arg == "-t" {
			hasT = true
		}
	}
	if hasT != stdinIsTTY() {
		t.Fatalf("-t present=%v tty=%v", hasT, stdinIsTTY())
	}
}

func TestPrepareSSHUsesTheEndpointAndHidesTheToken(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_, _ = io.WriteString(w, `{"createsshendpointresponse":{"sshendpoint":{"token":"tok","path":"/client/ssh","tokenheader":"X-CloudStack-Ssh-Token","expires":"2020-01-01T00:00:00Z"}}}`)
	}))
	defer server.Close()
	client, err := dune.New(server.URL, "KEY", "secret", dune.Insecure())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUNE_COMPUTE_SSH_WS_URL", "")
	t.Setenv("DUNE_COMPUTE_SSH_TOKEN", "")
	t.Setenv("DUNE_COMPUTE_SSH_INSECURE", "")
	t.Setenv("DUNE_COMPUTE_API_KEY", "WRONG")
	if err := prepareSSH(client, "i-1", "KEY", true); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DUNE_COMPUTE_API_KEY") != "KEY" {
		t.Fatal("api key was not passed to the proxy environment")
	}
	if os.Getenv("DUNE_COMPUTE_SSH_TOKEN") != "tok" {
		t.Fatal("token was not passed to the proxy environment")
	}
	wsURL := os.Getenv("DUNE_COMPUTE_SSH_WS_URL")
	if strings.Contains(wsURL, "tok") || !strings.HasPrefix(wsURL, "wss://") || !strings.HasSuffix(wsURL, "/client/ssh") {
		t.Fatalf("websocket url = %s", wsURL)
	}
	if os.Getenv("DUNE_COMPUTE_SSH_TOKEN_HEADER") != dune.TokenHeader {
		t.Fatal("token header was not set")
	}
	if os.Getenv("DUNE_COMPUTE_SSH_INSECURE") != "1" {
		t.Fatal("insecure tunnel flag was not passed to the proxy")
	}
}

func TestPrepareSSHRejectsAnAPIKeyWithANewline(t *testing.T) {
	if err := prepareSSH(nil, "i-1", "bad\rkey", false); err == nil {
		t.Fatal("newline in api key was accepted")
	}
}

func TestRejectedEndpointIsReportedBeforeSSH(t *testing.T) {
	fake := newCommandAPI(t)
	fake.sshStatus = http.StatusUnauthorized
	useServer(t, fake)
	code, _, errOut := capture(t, []string{"--insecure", "ssh", "i-1"})
	if code != 1 || !strings.Contains(errOut, "ssh endpoint:") {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	if _, err := parseSSHArgs(nil); err == nil {
		t.Fatal("missing instance id was accepted")
	}
}

func TestJSONFlagPrintsTheRawQuery(t *testing.T) {
	fake := newCommandAPI(t)
	useServer(t, fake)
	for _, args := range [][]string{
		{"--json", "instance", "list"},
		{"instance", "list", "--json"},
	} {
		code, out, errOut := capture(t, append([]string{"--insecure"}, args...))
		if code != 0 {
			t.Fatalf("code %d stderr %s", code, errOut)
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Fatalf("stdout is not JSON: %s", out)
		}
		if _, ok := decoded["instances"]; !ok {
			t.Fatalf("stdout = %s", out)
		}
	}
	code, out, errOut := capture(t, []string{"--insecure", "--json", "instance", "deploy", "--offering", "off", "--template", "tmpl", "--zone", "zone"})
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	if strings.Contains(out, "{") || strings.TrimSpace(out) != "Deployed instance web" {
		t.Fatalf("deploy stdout = %s", out)
	}
}

func TestListPrintsTheFieldsThatMatter(t *testing.T) {
	fake := newCommandAPI(t)
	fake.bodies = map[string]string{
		"listVirtualMachines": `{"listvirtualmachinesresponse":{"count":1,"virtualmachine":[{"id":"i-1","name":"web","state":"Running","ipaddress":"10.0.0.8","zonename":"east","serviceofferingname":"small","templatename":"ubuntu"}]}}`,
		"listVolumes":         `{"listvolumesresponse":{"count":1,"volume":[{"id":"vol-1","name":"data","state":"Ready","size":10737418240,"type":"DATADISK","vmname":"web","zonename":"east"}]}}`,
	}
	useServer(t, fake)
	code, out, errOut := capture(t, []string{"--insecure", "instance", "list"})
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	for _, phrase := range []string{"i-1", "web", "Running", "10.0.0.8", "east", "small", "ubuntu"} {
		if !strings.Contains(out, phrase) {
			t.Fatalf("instance table missing %q:\n%s", phrase, out)
		}
	}
	if strings.Contains(out, "cpunumber") || strings.Contains(out, "{") {
		t.Fatalf("instance table includes raw fields:\n%s", out)
	}
	code, out, errOut = capture(t, []string{"--insecure", "disk", "list"})
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	if !strings.Contains(out, "10 GB") || !strings.Contains(out, "DATADISK") {
		t.Fatalf("disk table = %s", out)
	}
}

func TestNameIsResolvedBeforeID(t *testing.T) {
	fake := newCommandAPI(t)
	fake.hits = map[string][]catalogHit{
		"listServiceOfferings": {{id: "so-real", name: "small"}, {id: "so-other", name: "off"}},
		"listDiskOfferings":    {{id: "do-real", name: "local.custom"}, {id: "do-other", name: "offering"}},
		"listZones":            {{id: "zone-real", name: "east"}},
		"listTemplates":        {{id: "tmpl-real", name: "ubuntu"}},
		"listVirtualMachines":  {{id: "i-real", name: "web"}},
		"listVolumes":          {{id: "vol-real", name: "data"}},
		"listUserData":         {{id: "u-real", name: "startup"}},
	}
	useServer(t, fake)
	code, _, errOut := capture(t, []string{"--insecure", "instance", "deploy", "--offering", "small", "--template", "ubuntu", "--zone", "east", "--name", "web"})
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	params := fake.paramsFor("deployVirtualMachine")
	for key, value := range map[string]string{"serviceofferingid": "so-real", "templateid": "tmpl-real", "zoneid": "zone-real"} {
		if params.Get(key) != value {
			t.Fatalf("%s = %q, params %v", key, params.Get(key), params)
		}
	}
	fake.reset()
	code, out, errOut := capture(t, []string{"--insecure", "instance", "destroy", "web"})
	if code != 0 {
		t.Fatalf("destroy code %d stderr %s", code, errOut)
	}
	if fake.paramsFor("destroyVirtualMachine").Get("id") != "i-real" {
		t.Fatalf("destroy params = %v", fake.paramsFor("destroyVirtualMachine"))
	}
	if strings.TrimSpace(out) != "Destroyed instance web" {
		t.Fatalf("destroy stdout = %q", out)
	}
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "instance", "deploy", "--offering", "missing", "--template", "missing-tmpl", "--zone", "missing-zone"})
	if code != 0 {
		t.Fatalf("id code %d stderr %s", code, errOut)
	}
	params = fake.paramsFor("deployVirtualMachine")
	for key, value := range map[string]string{"serviceofferingid": "missing", "templateid": "missing-tmpl", "zoneid": "missing-zone"} {
		if params.Get(key) != value {
			t.Fatalf("%s = %q, params %v", key, params.Get(key), params)
		}
	}
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "instance", "deploy", "--offering", "off", "--template", "tmpl", "--zone", "zone"})
	if code != 0 {
		t.Fatalf("precedence code %d stderr %s", code, errOut)
	}
	if fake.paramsFor("deployVirtualMachine").Get("serviceofferingid") != "so-other" {
		t.Fatalf("name did not win: %v", fake.paramsFor("deployVirtualMachine"))
	}
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "disk", "create", "--name", "data", "--zone", "east", "--offering", "local.custom", "--size", "100"})
	if code != 0 {
		t.Fatalf("disk code %d stderr %s", code, errOut)
	}
	created := fake.paramsFor("createVolume")
	if created.Get("diskofferingid") != "do-real" || created.Get("zoneid") != "zone-real" || created.Get("size") != "100" {
		t.Fatalf("create params = %v", created)
	}
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "disk", "create", "--name", "data", "--zone", "east", "--offering", "missing-offering"})
	if code != 0 {
		t.Fatalf("disk id code %d stderr %s", code, errOut)
	}
	created = fake.paramsFor("createVolume")
	if created.Get("diskofferingid") != "missing-offering" || created.Get("size") != "" {
		t.Fatalf("unresolved create params = %v", created)
	}
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "disk", "attach", "data", "web"})
	if code != 0 {
		t.Fatalf("attach code %d stderr %s", code, errOut)
	}
	attached := fake.paramsFor("attachVolume")
	if attached.Get("id") != "vol-real" || attached.Get("virtualmachineid") != "i-real" {
		t.Fatalf("attach params = %v", attached)
	}
	fake.reset()
	code, _, errOut = capture(t, []string{"--insecure", "template", "link-user-data", "--template", "ubuntu", "--user-data", "startup"})
	if code != 0 {
		t.Fatalf("link code %d stderr %s", code, errOut)
	}
	linked := fake.paramsFor("linkUserDataToTemplate")
	if linked.Get("templateid") != "tmpl-real" || linked.Get("userdataid") != "u-real" {
		t.Fatalf("link params = %v", linked)
	}
}

func TestAmbiguousNameIsRejected(t *testing.T) {
	fake := newCommandAPI(t)
	fake.hits = map[string][]catalogHit{
		"listServiceOfferings": {{id: "so-1", name: "small"}, {id: "so-2", name: "small"}},
	}
	useServer(t, fake)
	code, _, errOut := capture(t, []string{"--insecure", "instance", "deploy", "--offering", "small", "--template", "tmpl", "--zone", "zone"})
	if code != 1 || !strings.Contains(errOut, `named "small"`) {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	if fake.paramsFor("deployVirtualMachine") != nil {
		t.Fatal("deploy ran with an ambiguous offering")
	}
}

func TestJobProgressAndShortResult(t *testing.T) {
	fake := newCommandAPI(t)
	fake.jobBodies = []string{
		`{"queryasyncjobresultresponse":{"jobid":"job-1","jobstatus":0,"jobprocstatus":40}}`,
		`{"queryasyncjobresultresponse":{"jobid":"job-1","jobstatus":1,"jobprocstatus":100,"jobresult":{"virtualmachine":{"id":"vm-9","name":"web","state":"Running"}}}}`,
	}
	useServer(t, fake)
	previous := jobPollDelay
	jobPollDelay = 0
	t.Cleanup(func() { jobPollDelay = previous })
	code, out, errOut := capture(t, []string{"--insecure", "instance", "stop", "web"})
	if code != 0 {
		t.Fatalf("code %d stderr %s stdout %s", code, errOut, out)
	}
	if !strings.Contains(errOut, "Stopping instance web ... 40%") {
		t.Fatalf("stderr = %s", errOut)
	}
	if strings.TrimSpace(out) != "Stopped instance web" {
		t.Fatalf("stdout = %q", out)
	}
	if strings.Contains(out, "{") || strings.Contains(out, "jobid") || strings.Contains(out, "vm-9") {
		t.Fatalf("stdout = %s", out)
	}
}

func TestJobWaitTimesOut(t *testing.T) {
	fake := newCommandAPI(t)
	fake.holdJob = true
	client, err := dune.New(fake.url, "KEY", "secret", dune.Insecure())
	if err != nil {
		t.Fatal(err)
	}
	previous := jobPollDelay
	jobPollDelay = time.Hour
	t.Cleanup(func() { jobPollDelay = previous })
	var errOut bytes.Buffer
	previousErr := stderr
	stderr = &errOut
	t.Cleanup(func() { stderr = previousErr })
	_, err = waitJob(client, "job-1", 30*time.Millisecond, action{doing: "Stopping instance", target: "web"})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for job job-1") {
		t.Fatalf("err = %v", err)
	}
}

func TestChangeSummaryKeepsThePrivateKey(t *testing.T) {
	var out bytes.Buffer
	previous := stdout
	stdout = &out
	t.Cleanup(func() { stdout = previous })
	if err := writeResult(action{doing: "Creating SSH key", done: "Created SSH key", target: "ops"}, map[string]any{"id": "1", "name": "ops", "fingerprint": "SHA256:abc", "privatekey": "SECRET-KEY"}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, phrase := range []string{"Created SSH key ops", "private key:", "SECRET-KEY"} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("summary missing %q:\n%s", phrase, text)
		}
	}
	if strings.Contains(text, "{") {
		t.Fatalf("summary = %s", text)
	}
}

func capture(t *testing.T, argv []string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	previousOut, previousErr := stdout, stderr
	stdout, stderr = &out, &errOut
	t.Cleanup(func() {
		stdout, stderr = previousOut, previousErr
	})
	return run(argv), out.String(), errOut.String()
}

func useServer(t *testing.T, fake *commandAPI) {
	t.Helper()
	t.Setenv("DUNE_COMPUTE_API_URL", fake.url)
	t.Setenv("DUNE_COMPUTE_API_KEY", "KEY")
	t.Setenv("DUNE_COMPUTE_SECRET_KEY", "secret")
	t.Setenv("DUNE_COMPUTE_SSH_INSECURE", "")
}

type commandAPI struct {
	url       string
	sshStatus int
	holdJob   bool
	mu        sync.Mutex
	seen      []signedCall
	hits      map[string][]catalogHit
	bodies    map[string]string
	jobBodies []string
}

type catalogHit struct {
	id   string
	name string
}

type signedCall struct {
	command string
	params  url.Values
}

func newCommandAPI(t *testing.T) *commandAPI {
	t.Helper()
	fake := &commandAPI{}
	server := httptest.NewTLSServer(fake)
	t.Cleanup(server.Close)
	fake.url = server.URL
	return fake
}

func (f *commandAPI) reset() {
	f.mu.Lock()
	f.seen = nil
	f.mu.Unlock()
}

func (f *commandAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !commandSignature(r.Form, "secret") {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	command := r.Form.Get("command")
	copied := url.Values{}
	for key, values := range r.Form {
		copied[key] = append([]string(nil), values...)
	}
	f.mu.Lock()
	f.seen = append(f.seen, signedCall{command: command, params: copied})
	status := f.sshStatus
	holdJob := f.holdJob
	body := ""
	if command == "queryAsyncJobResult" && len(f.jobBodies) > 0 {
		body = f.jobBodies[0]
		f.jobBodies = f.jobBodies[1:]
	}
	if body == "" {
		if preset, ok := f.bodies[command]; ok {
			body = preset
		}
	}
	hits := append([]catalogHit(nil), f.hits[command]...)
	_, hitCommand := f.hits[command]
	f.mu.Unlock()
	if status != 0 && command == "createSshEndpoint" {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"createsshendpointresponse":{"errorcode":531,"errortext":"denied"}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if holdJob && command == "queryAsyncJobResult" {
		_, _ = io.WriteString(w, `{"queryasyncjobresultresponse":{"jobid":"job-1","jobstatus":0,"jobprocstatus":0}}`)
		return
	}
	if body != "" {
		_, _ = io.WriteString(w, body)
		return
	}
	if hitCommand {
		_, _ = io.WriteString(w, catalogBody(command, filterHits(hits, r.Form.Get("name"))))
		return
	}
	_, _ = io.WriteString(w, commandBody(command))
}

func filterHits(hits []catalogHit, name string) []catalogHit {
	if name == "" {
		return hits
	}
	matched := []catalogHit{}
	for _, hit := range hits {
		if hit.name == name {
			matched = append(matched, hit)
		}
	}
	return matched
}

func catalogBody(command string, hits []catalogHit) string {
	wrapper, field := listShape(command)
	rows := make([]map[string]string, 0, len(hits))
	for _, hit := range hits {
		rows = append(rows, map[string]string{"id": hit.id, "name": hit.name})
	}
	payload, _ := json.Marshal(map[string]any{
		wrapper: map[string]any{"count": len(rows), field: rows},
	})
	return string(payload)
}

func listShape(command string) (string, string) {
	switch command {
	case "listServiceOfferings":
		return "listserviceofferingsresponse", "serviceoffering"
	case "listDiskOfferings":
		return "listdiskofferingsresponse", "diskoffering"
	case "listZones":
		return "listzonesresponse", "zone"
	case "listTemplates":
		return "listtemplatesresponse", "template"
	case "listVirtualMachines":
		return "listvirtualmachinesresponse", "virtualmachine"
	case "listVolumes":
		return "listvolumesresponse", "volume"
	case "listUserData":
		return "listuserdataresponse", "userdata"
	case "listNetworks":
		return "listnetworksresponse", "network"
	default:
		return strings.ToLower(command) + "response", "item"
	}
}

func commandBody(command string) string {
	switch command {
	case "queryAsyncJobResult":
		return `{"queryasyncjobresultresponse":{"jobid":"job-1","jobstatus":1,"jobresult":{"result":{"id":"vm-1","name":"web"}}}}`
	case "createSSHKeyPair", "registerSSHKeyPair":
		return `{"` + strings.ToLower(command) + `response":{"keypair":{"name":"ops","id":"1"}}}`
	case "registerTemplate":
		return `{"registertemplateresponse":{"count":1,"template":[{"id":"t-1","name":"img"}]}}`
	case "linkUserDataToTemplate":
		return `{"linkuserdatatotemplateresponse":{"template":{"id":"t-1","name":"img"}}}`
	default:
		return `{"` + strings.ToLower(command) + `response":{"id":"vm-1","name":"web","jobid":"job-1","count":0}}`
	}
}

func (f *commandAPI) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.seen))
	for i, call := range f.seen {
		out[i] = call.command
	}
	return out
}

func (f *commandAPI) paramsFor(command string) url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.seen {
		if call.command == command {
			return call.params
		}
	}
	return nil
}

func commandSignature(params url.Values, secret string) bool {
	unsigned := url.Values{}
	for key, values := range params {
		if key == "signature" {
			continue
		}
		unsigned[key] = append([]string(nil), values...)
	}
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(strings.ToLower(cloudstack.EncodeValues(unsigned))))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(params.Get("signature")), []byte(want))
}
