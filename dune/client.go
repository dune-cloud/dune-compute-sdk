// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

// Package dune is the Go client for the Dune API.
// It is a thin wrapper over cloudstack-go.
package dune

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/apache/cloudstack-go/v2/cloudstack"
)

// Client calls the Dune API.
type Client struct {
	cs *cloudstack.CloudStackClient

	Instance        *InstanceService
	Disk            *DiskService
	Template        *TemplateService
	SSHKeyPair      *SSHKeyPairService
	Network         *NetworkService
	Zone            *ZoneService
	ServiceOffering *ServiceOfferingService
	DiskOffering    *DiskOfferingService
	Job             *JobService
	API             *APIService
	SSH             *SSHService
	UserData        *UserDataService
}

// Option configures a client.
type Option func(*clientOptions)

type clientOptions struct {
	async     bool
	verifySSL bool
	timeout   int64
}

// Async waits for asynchronous jobs before returning.
func Async() Option {
	return func(o *clientOptions) { o.async = true }
}

// Insecure skips TLS certificate verification.
func Insecure() Option {
	return func(o *clientOptions) { o.verifySSL = false }
}

// Timeout sets how long an async client waits for a job, in seconds.
func Timeout(seconds int64) Option {
	return func(o *clientOptions) { o.timeout = seconds }
}

// New returns a synchronous client. apiURL is the full API URL.
func New(apiURL, apiKey, secretKey string, opts ...Option) (*Client, error) {
	if apiURL == "" || apiKey == "" || secretKey == "" {
		return nil, errors.New("api URL, api key, and secret key are required")
	}
	options := clientOptions{verifySSL: true}
	for _, opt := range opts {
		opt(&options)
	}
	var csOpts []cloudstack.ClientOption
	if options.timeout > 0 {
		csOpts = append(csOpts, cloudstack.WithAsyncTimeout(options.timeout))
	}
	csOpts = append(csOpts, cloudstack.WithHTTPClient(httpClient(apiKey, options.verifySSL)))
	var cs *cloudstack.CloudStackClient
	if options.async {
		cs = cloudstack.NewAsyncClient(apiURL, apiKey, secretKey, options.verifySSL, csOpts...)
	} else {
		cs = cloudstack.NewClient(apiURL, apiKey, secretKey, options.verifySSL, csOpts...)
	}
	return &Client{
		cs:              cs,
		Instance:        &InstanceService{cs: cs},
		Disk:            &DiskService{cs: cs},
		Template:        &TemplateService{cs: cs},
		SSHKeyPair:      &SSHKeyPairService{cs: cs},
		Network:         &NetworkService{cs: cs},
		Zone:            &ZoneService{cs: cs},
		ServiceOffering: &ServiceOfferingService{cs: cs},
		DiskOffering:    &DiskOfferingService{cs: cs},
		Job:             &JobService{cs: cs},
		API:             &APIService{cs: cs},
		SSH:             &SSHService{cs: cs, apiURL: apiURL},
		UserData:        &UserDataService{cs: cs},
	}, nil
}

// NewAsyncClient waits for asynchronous jobs. Deploy, destroy, and other
// asynchronous calls block until the job finishes or times out.
func NewAsyncClient(apiURL, apiKey, secretKey string, opts ...Option) (*Client, error) {
	return New(apiURL, apiKey, secretKey, append(opts, Async())...)
}

// FromEnv reads DUNE_COMPUTE_API_URL, DUNE_COMPUTE_API_KEY, and DUNE_COMPUTE_SECRET_KEY.
func FromEnv(opts ...Option) (*Client, error) {
	return New(os.Getenv("DUNE_COMPUTE_API_URL"), os.Getenv("DUNE_COMPUTE_API_KEY"), os.Getenv("DUNE_COMPUTE_SECRET_KEY"), opts...)
}

func httpClient(apiKey string, verifySSL bool) *http.Client {
	base := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: !verifySSL},
	}
	return &http.Client{Transport: bearerTransport{base: base, token: apiKey}}
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}
	return jsonOrPlain(resp)
}

// jsonOrPlain passes a JSON body through. Any other body is returned as plain text.
func jsonOrPlain(resp *http.Response) (*http.Response, error) {
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if jsonObject(body) {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		return resp, nil
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = strings.TrimSpace(resp.Status)
	}
	if text == "" {
		text = "empty response"
	}
	return nil, &PlainError{text: text}
}

// PlainError is a response body that is not JSON.
type PlainError struct {
	text string
}

func (e *PlainError) Error() string {
	if e == nil {
		return ""
	}
	return e.text
}

func jsonObject(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return false
	}
	return json.Valid(trimmed)
}
