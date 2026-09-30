// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/apache/cloudstack-go/v2/cloudstack"
)

func TestClientKeepsItsTransportUnexported(t *testing.T) {
	clientType := reflect.TypeOf(Client{})
	for i := 0; i < clientType.NumField(); i++ {
		field := clientType.Field(i)
		if field.IsExported() && strings.Contains(field.Type.String(), "CloudStackClient") {
			t.Fatalf("exported field %s", field.Name)
		}
	}
	if _, ok := any(&Client{}).(interface {
		CloudStack() *cloudstack.CloudStackClient
	}); ok {
		t.Fatal("client exports its transport")
	}
}

func TestWebSocketURL(t *testing.T) {
	got, err := WebSocketURL("https://compute.dunecloud.ai/client/api", "/client/ssh")
	if err != nil {
		t.Fatal(err)
	}
	if got != "wss://compute.dunecloud.ai/client/ssh" {
		t.Fatalf("url = %s", got)
	}
	got, err = WebSocketURL("https://edge.example:8443/client/api?x=1", "client/ssh")
	if err != nil {
		t.Fatal(err)
	}
	if got != "wss://edge.example:8443/client/ssh" {
		t.Fatalf("url = %s", got)
	}
	got, err = WebSocketURL("http://compute.dunecloud.ai/client/api", "/client/ssh")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ws://compute.dunecloud.ai/client/ssh" {
		t.Fatalf("url = %s", got)
	}
	if _, err := WebSocketURL("https://compute.dunecloud.ai/client/api", "/client/ssh?token=secret"); err == nil {
		t.Fatal("query in the tunnel path was accepted")
	}
	if _, err := WebSocketURL("ssh://compute.dunecloud.ai/client/api", "/client/ssh"); err == nil {
		t.Fatal("API URL scheme was accepted")
	}
}

func TestClientSendsEachCall(t *testing.T) {
	fake := newFakeAPI(t)
	client, err := New(fake.url, "KEY", "secret", Insecure())
	if err != nil {
		t.Fatal(err)
	}
	call := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	call("deploy", errOnly(client.Instance.Deploy(client.Instance.NewDeployParams("offering", "template", "zone"))))
	call("destroy", errOnly(client.Instance.Destroy(client.Instance.NewDestroyParams("i-1"))))
	call("expunge", errOnly(client.Instance.Expunge(client.Instance.NewExpungeParams("i-1"))))
	call("start", errOnly(client.Instance.Start(client.Instance.NewStartParams("i-1"))))
	call("stop", errOnly(client.Instance.Stop(client.Instance.NewStopParams("i-1"))))
	call("update instance", errOnly(client.Instance.Update(client.Instance.NewUpdateParams("i-1"))))
	call("change service", errOnly(client.Instance.ChangeService(client.Instance.NewChangeServiceParams("i-1", "offering"))))
	call("list instances", errOnly(client.Instance.List(client.Instance.NewListParams())))
	call("create disk", errOnly(client.Disk.Create(client.Disk.NewCreateParams())))
	call("delete disk", errOnly(client.Disk.Delete(client.Disk.NewDeleteParams("vol-1"))))
	call("attach disk", errOnly(client.Disk.Attach(client.Disk.NewAttachParams("vol-1", "i-1"))))
	call("detach disk", errOnly(client.Disk.Detach(client.Disk.NewDetachParams())))
	call("resize disk", errOnly(client.Disk.Resize(client.Disk.NewResizeParams("vol-1"))))
	call("update disk", errOnly(client.Disk.Update(client.Disk.NewUpdateParams())))
	call("list disks", errOnly(client.Disk.List(client.Disk.NewListParams())))
	call("register template", errOnly(client.Template.Register(client.Template.NewRegisterParams("text", "QCOW2", "name", "https://example/t"))))
	call("update template", errOnly(client.Template.Update(client.Template.NewUpdateParams("t-1"))))
	call("delete template", errOnly(client.Template.Delete(client.Template.NewDeleteParams("t-1"))))
	call("link userdata", errOnly(client.Template.LinkUserData(client.Template.NewLinkUserDataParams())))
	call("list templates", errOnly(client.Template.List(client.Template.NewListParams("executable"))))
	call("create key", errOnly(client.SSHKeyPair.Create(client.SSHKeyPair.NewCreateParams("key"))))
	call("register key", errOnly(client.SSHKeyPair.Register(client.SSHKeyPair.NewRegisterParams("key", "ssh-ed25519 AAAA"))))
	call("delete key", errOnly(client.SSHKeyPair.Delete(client.SSHKeyPair.NewDeleteParams("key"))))
	call("list keys", errOnly(client.SSHKeyPair.List(client.SSHKeyPair.NewListParams())))
	call("list networks", errOnly(client.Network.List(client.Network.NewListParams())))
	call("list zones", errOnly(client.Zone.List(client.Zone.NewListParams())))
	call("list service offerings", errOnly(client.ServiceOffering.List(client.ServiceOffering.NewListParams())))
	call("list disk offerings", errOnly(client.DiskOffering.List(client.DiskOffering.NewListParams())))
	call("query job", errOnly(client.Job.Query(client.Job.NewQueryParams("job-1"))))
	call("list apis", errOnly(client.API.List(client.API.NewListParams())))
	endpoint, err := client.SSH.CreateEndpoint("i-1")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Token != "tok" || endpoint.Path != "/client/ssh" || endpoint.TokenHeader != TokenHeader {
		t.Fatalf("endpoint = %+v", endpoint)
	}
	if endpoint.WebSocketURL != "wss://"+fake.host+"/client/ssh" {
		t.Fatalf("websocket url = %s", endpoint.WebSocketURL)
	}
	if strings.Contains(endpoint.WebSocketURL, "tok") {
		t.Fatal("token was placed in the websocket URL")
	}

	got := uniqueSorted(fake.commands())
	want := uniqueSorted(commandNames)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands sent = %v\nwant %v", got, want)
	}
	deploy := fake.paramsFor("deployVirtualMachine")
	if deploy.Get("serviceofferingid") != "offering" || deploy.Get("templateid") != "template" || deploy.Get("zoneid") != "zone" {
		t.Fatalf("deploy params = %v", deploy)
	}
	sshCall := fake.paramsFor("createSshEndpoint")
	if sshCall.Get("virtualmachineid") != "i-1" {
		t.Fatalf("ssh endpoint params = %v", sshCall)
	}
}

func TestTemplateFilterAllIsRejected(t *testing.T) {
	fake := newFakeAPI(t)
	client, err := New(fake.url, "KEY", "secret", Insecure())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Template.List(client.Template.NewListParams("ALL"))
	if err == nil || !strings.Contains(err.Error(), "all") {
		t.Fatalf("err = %v", err)
	}
	if fake.paramsFor("listTemplates") != nil {
		t.Fatal("listTemplates was called")
	}
}

func TestCreateEndpointRejectsAnEmptyToken(t *testing.T) {
	fake := newFakeAPI(t)
	fake.sshBody = `{"createsshendpointresponse":{"sshendpoint":{"path":"/client/ssh"}}}`
	client, err := New(fake.url, "KEY", "secret", Insecure())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SSH.CreateEndpoint("i-1"); err == nil {
		t.Fatal("empty token was accepted")
	}
}

func TestClientSendsAPIKeyAsBearer(t *testing.T) {
	fake := newFakeAPI(t)
	const apiKey = "test-api-key"
	client, err := New(fake.url, apiKey, "secret", Insecure())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Instance.List(client.Instance.NewListParams()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Instance.Deploy(client.Instance.NewDeployParams("offering", "template", "zone")); err != nil {
		t.Fatal(err)
	}
	headers := fake.authorizations()
	if len(headers) != 2 {
		t.Fatalf("requests = %d", len(headers))
	}
	for _, header := range headers {
		if header != "Bearer "+apiKey {
			t.Fatalf("authorization = %q", header)
		}
	}
}

func TestPlainTextResponseIsTheError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "\nplain text response\n")
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, "KEY", "secret", Insecure())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Zone.List(client.Zone.NewListParams())
	var plain *PlainError
	if !errors.As(err, &plain) || plain.Error() != "plain text response" {
		t.Fatalf("error = %v", err)
	}
}

func TestJSONAPIErrorStaysStructured(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"listzonesresponse":{"errorcode":401,"cserrorcode":0,"errortext":"unable to verify user credentials"}}`)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, "KEY", "secret", Insecure())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Zone.List(client.Zone.NewListParams())
	if err == nil || !strings.Contains(err.Error(), "unable to verify user credentials") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Fatal(err)
	}
}

func TestClientVerifiesTLSUnlessInsecure(t *testing.T) {
	fake := newFakeAPI(t)
	client, err := New(fake.url, "KEY", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Instance.List(client.Instance.NewListParams()); err == nil {
		t.Fatal("certificate verification was skipped")
	}
}

func TestNewRequiresCredentials(t *testing.T) {
	if _, err := New("", "key", "secret"); err == nil {
		t.Fatal("missing API URL was accepted")
	}
	if _, err := New("https://example/client/api", "", "secret"); err == nil {
		t.Fatal("missing API key was accepted")
	}
	if _, err := New("https://example/client/api", "key", ""); err == nil {
		t.Fatal("missing secret key was accepted")
	}
}

func TestFromEnvReadsComputeCredentials(t *testing.T) {
	fake := newFakeAPI(t)
	t.Setenv("DUNE_COMPUTE_API_URL", fake.url)
	t.Setenv("DUNE_COMPUTE_API_KEY", "KEY")
	t.Setenv("DUNE_COMPUTE_SECRET_KEY", "secret")
	client, err := FromEnv(Insecure())
	if err != nil {
		t.Fatal(err)
	}
	listed, err := client.Instance.List(client.Instance.NewListParams())
	if err != nil {
		t.Fatal(err)
	}
	if listed.Instances == nil || listed.Count != 0 {
		t.Fatalf("instances = %+v", listed)
	}
	t.Setenv("DUNE_COMPUTE_API_URL", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("missing credentials were accepted")
	}
}

func TestAsyncClientReturnsTheFinishedInstance(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !validSignature(r.Form, "secret") {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("command") == "queryAsyncJobResult" {
			_, _ = io.WriteString(w, `{"queryasyncjobresultresponse":{"jobid":"job-1","jobstatus":1,"jobresult":{"result":{"id":"vm-9","name":"finished","state":"Running"}}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"deployvirtualmachineresponse":{"jobid":"job-1"}}`)
	}))
	defer server.Close()
	client, err := NewAsyncClient(server.URL, "KEY", "secret", Insecure(), Timeout(5))
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.Instance.Deploy(client.Instance.NewDeployParams("offering", "template", "zone"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Id != "vm-9" || created.Name != "finished" || created.State != "Running" {
		t.Fatalf("created = %+v", created)
	}
}

func TestRequestSignature(t *testing.T) {
	values := url.Values{}
	values.Set("command", "createSshEndpoint")
	values.Set("virtualmachineid", "abc-def")
	values.Set("response", "json")
	values.Set("apiKey", "KEY")
	values.Set("signatureversion", "3")
	values.Set("expires", "2020-01-01T00:00:00Z")
	encoded := strings.ToLower(cloudstack.EncodeValues(values))
	want := "apikey=key&command=createsshendpoint&expires=2020-01-01t00%3a00%3a00z&response=json&signatureversion=3&virtualmachineid=abc-def"
	if encoded != want {
		t.Fatalf("canonical = %s", encoded)
	}
	mac := hmac.New(sha1.New, []byte("secret"))
	mac.Write([]byte(encoded))
	if base64.StdEncoding.EncodeToString(mac.Sum(nil)) == "" {
		t.Fatal("empty signature")
	}
}

var commandNames = []string{
	"attachVolume",
	"changeServiceForVirtualMachine",
	"createSSHKeyPair",
	"createSshEndpoint",
	"createVolume",
	"deleteSSHKeyPair",
	"deleteTemplate",
	"deleteVolume",
	"deployVirtualMachine",
	"destroyVirtualMachine",
	"detachVolume",
	"expungeVirtualMachine",
	"linkUserDataToTemplate",
	"listApis",
	"listDiskOfferings",
	"listNetworks",
	"listSSHKeyPairs",
	"listServiceOfferings",
	"listTemplates",
	"listVirtualMachines",
	"listVolumes",
	"listZones",
	"queryAsyncJobResult",
	"registerSSHKeyPair",
	"registerTemplate",
	"resizeVolume",
	"startVirtualMachine",
	"stopVirtualMachine",
	"updateTemplate",
	"updateVirtualMachine",
	"updateVolume",
}

type fakeAPI struct {
	url           string
	host          string
	sshBody       string
	mu            sync.Mutex
	seen          []signedCall
	authorization []string
}

type signedCall struct {
	command string
	params  url.Values
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	fake := &fakeAPI{}
	server := httptest.NewTLSServer(fake)
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	fake.url = server.URL
	fake.host = parsed.Host
	return fake
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params := r.Form
	if !validSignature(params, "secret") {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	command := params.Get("command")
	copied := url.Values{}
	for key, values := range params {
		copied[key] = append([]string(nil), values...)
	}
	f.mu.Lock()
	f.seen = append(f.seen, signedCall{command: command, params: copied})
	f.authorization = append(f.authorization, r.Header.Get("Authorization"))
	body := f.sshBody
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if command == "createSshEndpoint" {
		if body == "" {
			body = `{"createsshendpointresponse":{"sshendpoint":{"token":"tok","path":"/client/ssh","tokenheader":"X-CloudStack-Ssh-Token","expires":"2020-01-01T00:00:00Z"}}}`
		}
		_, _ = io.WriteString(w, body)
		return
	}
	if command == "createSSHKeyPair" || command == "registerSSHKeyPair" {
		_, _ = io.WriteString(w, `{"`+strings.ToLower(command)+`response":{"keypair":{"name":"key","id":"1"}}}`)
		return
	}
	payload, err := json.Marshal(map[string]any{
		strings.ToLower(command) + "response": map[string]any{"id": "1", "jobid": "job-1", "count": 0},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(payload)
}

func (f *fakeAPI) authorizations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.authorization...)
}

func (f *fakeAPI) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.seen))
	for i, call := range f.seen {
		out[i] = call.command
	}
	return out
}

func (f *fakeAPI) paramsFor(command string) url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.seen {
		if call.command == command {
			return call.params
		}
	}
	return nil
}

func validSignature(params url.Values, secret string) bool {
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

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func errOnly(_ any, err error) error { return err }
