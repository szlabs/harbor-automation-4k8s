// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"errors"

	gruntime "github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	hc "github.com/szlabs/harbor-automation-4k8s/pkg/sdk/harbor/client"
	hc2 "github.com/szlabs/harbor-automation-4k8s/pkg/sdk/harbor_v2/client"
	corev1 "k8s.io/api/core/v1"
)

const (
	accessKey    = "accessKey"
	accessSecret = "accessSecret"
)

// AccessCred contains credential data for accessing the harbor server
type AccessCred struct {
	AccessKey    string
	AccessSecret string
}

// FillIn put secret into AccessCred
func (ac *AccessCred) FillIn(secret *corev1.Secret) error {
	decodedAK, ok1 := secret.Data[accessKey]
	decodedAS, ok2 := secret.Data[accessSecret]
	if !(ok1 && ok2) {
		return errors.New("invalid access secret")
	}

	ac.AccessKey = string(decodedAK)
	ac.AccessSecret = string(decodedAS)
	return nil
}

// Validate validates wether the key and secret has correct format
func (ac *AccessCred) Validate(secret *corev1.Secret) error {
	if len(ac.AccessKey) == 0 || len(ac.AccessSecret) == 0 {
		return errors.New("access key and secret can't be empty")
	}
	return nil
}

// HarborServer contains connection data
type HarborServer struct {
	ServerURL  string
	AccessCred *AccessCred
	InSecure   bool
}

// NewHarborServer returns harbor server with inputs
func NewHarborServer(serverURL string, accessCred *AccessCred, insecure bool) *HarborServer {
	return &HarborServer{
		ServerURL:  serverURL,
		AccessCred: accessCred,
		InSecure:   insecure,
	}
}

// HarborClient keeps Harbor client
type HarborClient struct {
	Client *hc.Harbor
	Auth   gruntime.ClientAuthInfoWriter
}

// HarborClientV2 keeps Harbor client v2
type HarborClientV2 struct {
	Client *hc2.Harbor
	Auth   gruntime.ClientAuthInfoWriter
}

// Client created based on the server data
func (h *HarborServer) Client() *HarborClient {
	// New client
	cfg := &hc.TransportConfig{
		Host:     h.ServerURL,
		BasePath: hc.DefaultBasePath,
		Schemes:  hc.DefaultSchemes,
	}

	if h.InSecure {
		cfg.Schemes = []string{"http"}
	}

	c := hc.NewHTTPClientWithConfig(nil, cfg)
	auth := httptransport.BasicAuth(h.AccessCred.AccessKey, h.AccessCred.AccessSecret)

	return &HarborClient{
		Client: c,
		Auth:   auth,
	}
}

// Client created based on the server data
func (h *HarborServer) ClientV2() *HarborClientV2 {
	// New client
	cfg := &hc2.TransportConfig{
		Host:     h.ServerURL,
		BasePath: hc2.DefaultBasePath,
		Schemes:  hc2.DefaultSchemes,
	}

	if h.InSecure {
		cfg.Schemes = []string{"http"}
	}

	c := hc2.NewHTTPClientWithConfig(nil, cfg)
	auth := httptransport.BasicAuth(h.AccessCred.AccessKey, h.AccessCred.AccessSecret)

	return &HarborClientV2{
		Client: c,
		Auth:   auth,
	}
}

// Robot contains info of robot account
type Robot struct {
	ID    int64
	Name  string
	Token string
}
