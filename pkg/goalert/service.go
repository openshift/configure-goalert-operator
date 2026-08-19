/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package goalert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/openshift/configure-goalert-operator/config"
)

// Client is a wrapper interface for the GraphqlClient to allow for easier testing.
type Client interface {
	// CreateService creates a GoAlert service and returns its ID.
	CreateService(ctx context.Context, data *Data) (string, error)
	// CreateIntegrationKey creates an integration key for a GoAlert service and returns the key URL.
	CreateIntegrationKey(ctx context.Context, data *Data) (string, error)
	// CreateHeartbeatMonitor creates a heartbeat monitor for a GoAlert service and returns the key URL and monitor ID.
	CreateHeartbeatMonitor(ctx context.Context, data *Data) (string, string, error)
	// DeleteService deletes a GoAlert service by ID.
	DeleteService(ctx context.Context, data *Data) error
	// NewRequest sends an HTTP request to the GoAlert GraphQL API and returns the response body.
	NewRequest(ctx context.Context, method string, body any) ([]byte, error)
	// IsHeartbeatMonitorInactive checks whether a heartbeat monitor is in the inactive state.
	IsHeartbeatMonitorInactive(ctx context.Context, data *Data) (bool, error)
	// GetServiceIDByName returns the ID of the GoAlert service whose name exactly matches name,
	// or "" if no such service exists.
	GetServiceIDByName(ctx context.Context, name string) (string, error)
	// GetIntegrationKeyHref returns the href of the integration key on the given service whose
	// name matches keyName, or "" if none exists.
	GetIntegrationKeyHref(ctx context.Context, serviceID, keyName, keyType string) (string, error)
	// GetHeartbeatMonitor returns the href and id of the heartbeat monitor on the given service
	// whose name matches monitorName, or ("","") if none exists.
	GetHeartbeatMonitor(ctx context.Context, serviceID, monitorName string) (href string, id string, err error)
}

// Wrapper for HTTP client
type GraphqlClient struct {
	httpClient    *http.Client
	sessionCookie *http.Cookie
}

// Wrapper to create new client for GraphQL api calls
func NewClient(sessionCookie *http.Cookie) Client {
	return &GraphqlClient{
		httpClient:    config.HTTPClient(),
		sessionCookie: sessionCookie,
	}
}

// Data describes the data that is needed for Goalert GraphQL api calls
type Data struct {
	Name               string `json:"name"`
	Id                 string `json:"id,omitempty"`
	Description        string `json:"description,omitempty"`
	Favorite           bool   `json:"favorite,omitempty"`
	EscalationPolicyID string `json:"escalationPolicyID,omitempty"`
	Type               string `json:"type,omitempty"`
	Timeout            int    `json:"timeoutMinutes,omitempty"`
	DeleteAll          bool   `json:"deleteAll,omitempty"`
}

// Q describes GraphQL query payload
type Q struct {
	Query string
}

// RespSvcData describes Svc ID returned from createService
type RespSvcData struct {
	Data struct {
		CreateService struct {
			ID string `json:"id"`
		} `json:"createService"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// RespIntKeyData describes int key returned from createIntegrationKey
type RespIntKeyData struct {
	Data struct {
		CreateIntKey struct {
			Key string `json:"href"`
		} `json:"createIntegrationKey"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// RespHeartBeatData describes a heartbeat monitor key from createHeartbeatMonitor
type RespHeartBeatData struct {
	Data struct {
		CreateHeartBeatKey struct {
			Key string `json:"href"`
			Id  string `json:"id"`
		} `json:"createHeartbeatMonitor"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// RespDelete contains boolean returned from deleteAll
type RespDelete struct {
	Data struct {
		DeleteAll bool `json:"deleteAll"`
	} `json:"data"`
}

// RespHeartbeatState describes the heartbeat monitor state returned from a heartbeatMonitor query.
type RespHeartbeatState struct {
	Data struct {
		Heatbeatmonitor struct {
			LastState string `json:"lastState"`
		} `json:"heartbeatMonitor"`
	} `json:"data"`
}

// RespServiceSearch describes the services returned from a services search query.
type RespServiceSearch struct {
	Data struct {
		Services struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"services"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// RespServiceIntKeys describes the integration keys returned from a service query.
type RespServiceIntKeys struct {
	Data struct {
		Service struct {
			IntegrationKeys []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
				Href string `json:"href"`
			} `json:"integrationKeys"`
		} `json:"service"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// RespServiceHeartbeats describes the heartbeat monitors returned from a service query.
type RespServiceHeartbeats struct {
	Data struct {
		Service struct {
			HeartbeatMonitors []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Href string `json:"href"`
			} `json:"heartbeatMonitors"`
		} `json:"service"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// GraphQLError represents a single error in a GraphQL response.
type GraphQLError struct {
	Message string `json:"message"`
	Path    []any  `json:"path,omitempty"`
}

// GraphQLErrorResponse wraps GraphQL errors returned in HTTP 200 responses.
type GraphQLErrorResponse struct {
	Errors []GraphQLError `json:"errors,omitempty"`
}

// NewRequest is a wrapper func to help send the http request
func (c *GraphqlClient) NewRequest(ctx context.Context, method string, body any) ([]byte, error) {

	goalertApiEndpoint := os.Getenv(config.GoalertApiEndpointEnvVar)

	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, goalertApiEndpoint+"/api/graphql", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.AddCookie(c.sessionCookie)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GoAlert API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, nil
}

// CreateService calls GoAlert's GraphQL api to create a new service within GoAlert
func (c *GraphqlClient) CreateService(ctx context.Context, data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createService(input:{name:%s,description:%s,favorite:%t,escalationPolicyID:%s}){id}}`,
		strconv.Quote(data.Name), strconv.Quote(data.Description), data.Favorite, strconv.Quote(data.EscalationPolicyID))

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return "", err
	}

	var r RespSvcData
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if len(r.Errors) > 0 {
		return "", fmt.Errorf("GoAlert GraphQL error creating service: %s", r.Errors[0].Message)
	}

	if r.Data.CreateService.ID == "" {
		return "", fmt.Errorf("GoAlert returned empty service ID (response: %s)", string(respData))
	}

	return r.Data.CreateService.ID, nil
}

// CreateIntegrationKey calls GoAlert's GraphQL api to create a new integration key
func (c *GraphqlClient) CreateIntegrationKey(ctx context.Context, data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createIntegrationKey(input:{serviceID:%s,type:%s,name:%s}){href}}`,
		strconv.Quote(data.Id), data.Type, strconv.Quote(data.Name))

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return "", err
	}

	var r RespIntKeyData
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if len(r.Errors) > 0 {
		return "", fmt.Errorf("GoAlert GraphQL error creating integration key: %s", r.Errors[0].Message)
	}

	if r.Data.CreateIntKey.Key == "" {
		return "", fmt.Errorf("GoAlert returned empty integration key (response: %s)", string(respData))
	}

	return r.Data.CreateIntKey.Key, nil
}

// CreateHeartbeatMonitor calls GoAlert's GraphQL api to create a new heartbeat monitor for a GoAlert Service
func (c *GraphqlClient) CreateHeartbeatMonitor(ctx context.Context, data *Data) (string, string, error) {

	query := fmt.Sprintf(`mutation {createHeartbeatMonitor(input: {serviceID: %s,name: %s,timeoutMinutes: %d }){href,id}}`,
		strconv.Quote(data.Id), strconv.Quote(data.Name), data.Timeout)

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return "", "", err
	}

	var r RespHeartBeatData
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", "", fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if len(r.Errors) > 0 {
		return "", "", fmt.Errorf("GoAlert GraphQL error creating heartbeat monitor: %s", r.Errors[0].Message)
	}

	if r.Data.CreateHeartBeatKey.Key == "" || r.Data.CreateHeartBeatKey.Id == "" {
		return "", "", fmt.Errorf("GoAlert returned empty heartbeat key or ID (response: %s)", string(respData))
	}

	return r.Data.CreateHeartBeatKey.Key, r.Data.CreateHeartBeatKey.Id, nil
}

// DeleteService calls GoAlert's GraphQL API to delete a GoAlert service
func (c *GraphqlClient) DeleteService(ctx context.Context, data *Data) error {
	query := fmt.Sprintf(`mutation {
			deleteAll(input: {
				id: %s,
				type: service
			})
		}`, strconv.Quote(data.Id))

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return err
	}

	var r RespDelete
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if !r.Data.DeleteAll {
		return errors.New("failed to delete service")
	}
	return nil
}

// IsHeartbeatMonitorInactive queries GoAlert to determine if the specified heartbeat monitor is inactive.
func (c *GraphqlClient) IsHeartbeatMonitorInactive(ctx context.Context, data *Data) (bool, error) {
	query := fmt.Sprintf(`query {
		heartbeatMonitor(
			id: %s,
		){lastState}}`, strconv.Quote(data.Id))

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return false, err
	}

	var r RespHeartbeatState
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return false, fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if r.Data.Heatbeatmonitor.LastState != "inactive" {
		return false, nil
	}
	return true, nil
}

// GetServiceIDByName queries GoAlert for a service whose name exactly matches name and returns its ID, or "" if none exists.
func (c *GraphqlClient) GetServiceIDByName(ctx context.Context, name string) (string, error) {
	query := fmt.Sprintf(`query {services(input:{search:%s,first:100}){nodes{id name}}}`, strconv.Quote(name))

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return "", err
	}

	var r RespServiceSearch
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if len(r.Errors) > 0 {
		return "", fmt.Errorf("GoAlert GraphQL error searching for service: %s", r.Errors[0].Message)
	}

	// GoAlert search is fuzzy/substring, so scan for the exact name match rather than
	// returning the first node.
	for _, node := range r.Data.Services.Nodes {
		if node.Name == name {
			return node.ID, nil
		}
	}

	return "", nil
}

// GetIntegrationKeyHref queries GoAlert for an integration key on the given service whose name matches keyName and returns its href, or "" if none exists.
func (c *GraphqlClient) GetIntegrationKeyHref(ctx context.Context, serviceID, keyName, keyType string) (string, error) {
	query := fmt.Sprintf(`query {service(id:%s){integrationKeys{id name type href}}}`, strconv.Quote(serviceID))

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return "", err
	}

	var r RespServiceIntKeys
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if len(r.Errors) > 0 {
		return "", fmt.Errorf("GoAlert GraphQL error querying integration keys: %s", r.Errors[0].Message)
	}

	// Match by name only. keyType is accepted for interface symmetry but is not used for
	// matching because GoAlert returns the type as a JSON string while creates send it as an enum.
	for _, key := range r.Data.Service.IntegrationKeys {
		if key.Name == keyName {
			return key.Href, nil
		}
	}

	return "", nil
}

// GetHeartbeatMonitor queries GoAlert for a heartbeat monitor on the given service whose name matches monitorName and returns its href and id, or ("","") if none exists.
func (c *GraphqlClient) GetHeartbeatMonitor(ctx context.Context, serviceID, monitorName string) (string, string, error) {
	query := fmt.Sprintf(`query {service(id:%s){heartbeatMonitors{id name href}}}`, strconv.Quote(serviceID))

	query = strings.ReplaceAll(query, "\t", "")
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return "", "", err
	}

	var r RespServiceHeartbeats
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", "", fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}

	if len(r.Errors) > 0 {
		return "", "", fmt.Errorf("GoAlert GraphQL error querying heartbeat monitors: %s", r.Errors[0].Message)
	}

	for _, monitor := range r.Data.Service.HeartbeatMonitors {
		if monitor.Name == monitorName {
			return monitor.Href, monitor.ID, nil
		}
	}

	return "", "", nil
}
