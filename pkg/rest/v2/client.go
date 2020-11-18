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

package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/szlabs/harbor-automation-4k8s/pkg/utils"

	ghttp "github.com/szlabs/harbor-automation-4k8s/pkg/http"
	"github.com/szlabs/harbor-automation-4k8s/pkg/rest/model"
	"github.com/szlabs/harbor-automation-4k8s/pkg/sdk/harbor_v2/client/project"
	v2models "github.com/szlabs/harbor-automation-4k8s/pkg/sdk/harbor_v2/models"
)

// Client for talking to Harbor V2 API
// Wrap based on sdk v2
type Client struct {
	// Server info for talking to
	server *model.HarborServer
	// Timeout for client connection
	timeout time.Duration
	// Context for doing client connection
	context context.Context
	// Insecure HTTP client
	insecureClient *http.Client
	// Harbor API client
	harborClient *model.HarborClientV2
}

// New V2 client
func New() *Client {
	// Initialize with default settings
	return &Client{
		timeout:        30 * time.Second,
		context:        context.Background(),
		insecureClient: ghttp.Client,
	}
}

// NewWithServer new V2 client with provided server
func NewWithServer(s *model.HarborServer) *Client {
	// Initialize with default settings
	c := New()
	c.server = s
	c.harborClient = s.ClientV2()

	return c
}

func (c *Client) WithServer(s *model.HarborServer) *Client {
	if s != nil {
		c.server = s
		c.harborClient = s.ClientV2()
	}

	return c
}

func (c *Client) WithContext(ctx context.Context) *Client {
	if ctx != nil {
		c.context = ctx
	}

	return c
}

func (c *Client) WithTimeout(timeout time.Duration) *Client {
	c.timeout = timeout
	return c
}

// EnsureProject ensures the specified project is on the harbor server
// If project with name is existing, then error will be nil
func (c *Client) EnsureProject(name string) (int64, error) {
	if len(name) == 0 {
		return -1, errors.New("project name is empty")
	}

	if c.harborClient == nil {
		return -1, errors.New("nil harbor client")
	}

	// Check existence first
	p, err := c.GetProject(name)
	if err == nil {
		return int64(p.ProjectID), nil
	}

	// Create one when the project does not exist
	cparams := project.NewCreateProjectParamsWithContext(c.context).
		WithTimeout(c.timeout).
		WithHTTPClient(c.insecureClient).
		WithProject(&v2models.ProjectReq{
			ProjectName: name,
			Metadata: &v2models.ProjectMetadata{
				Public: "false",
			},
		})
	cp, err := c.harborClient.Client.Project.CreateProject(cparams, c.harborClient.Auth)
	if err != nil {
		return -1, fmt.Errorf("ensure proejct error: %w", err)
	}

	return utils.ExtractID(cp.Location)
}

// GetProject gets the project data
func (c *Client) GetProject(name string) (*v2models.Project, error) {
	if len(name) == 0 {
		return nil, errors.New("project name is empty")
	}

	if c.harborClient == nil {
		return nil, errors.New("nil harbor client")
	}

	params := project.NewListProjectsParamsWithContext(c.context).
		WithTimeout(c.timeout).
		WithHTTPClient(c.insecureClient).
		WithName(&name)

	res, err := c.harborClient.Client.Project.ListProjects(params, c.harborClient.Auth)
	if err != nil {
		return nil, fmt.Errorf("get project error: %w", err)
	}

	if len(res.Payload) < 1 {
		return nil, fmt.Errorf("no project with name %s exitsing", name)
	}

	return res.Payload[0], nil
}

// DeleteProject deletes project
func (c *Client) DeleteProject(name string) error {
	if len(name) == 0 {
		return errors.New("project name is empty")
	}

	if c.harborClient == nil {
		return errors.New("nil harbor client")
	}

	// Get ID first
	p, err := c.GetProject(name)
	if err != nil {
		return fmt.Errorf("delete project error: %w", err)
	}

	params := project.NewDeleteProjectParamsWithContext(c.context).
		WithTimeout(c.timeout).
		WithHTTPClient(c.insecureClient).
		WithProjectID((int64)(p.ProjectID))
	if _, err = c.harborClient.Client.Project.DeleteProject(params, c.harborClient.Auth); err != nil {
		return err
	}

	return nil
}
