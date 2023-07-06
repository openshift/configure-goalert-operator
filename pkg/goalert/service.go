package goalert

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/openshift/configure-goalert-operator/config"
)

// Client is a wrapper interface for the GraphqlClient to allow for easier testing
type Client interface {
	CreateService(ctx context.Context, data *Data) (string, error)
	CreateIntegrationKey(ctx context.Context, data *Data) (string, error)
	CreateHeartbeatMonitor(ctx context.Context, data *Data) (string, error)
	DeleteService(ctx context.Context, data *Data) error
	NewRequest(ctx context.Context, method string, body interface{}) ([]byte, error)
}

// Wrapper for HTTP client
type GraphqlClient struct {
	httpClient    *http.Client
	sessionCookie *http.Cookie
}

// Wrapper to create new client for GraphQL api calls
func NewClient(sessionCookie *http.Cookie) Client {
	return &GraphqlClient{
		httpClient:    http.DefaultClient,
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
}

// RespIntKeyData describes int key returned from createIntegrationKey
type RespIntKeyData struct {
	Data struct {
		CreateIntKey struct {
			Key string `json:"href"`
		} `json:"createIntegrationKey"`
	} `json:"data"`
}

// RespHeartBeatData describes a heartbeat monitor key from createHeartbeatMonitor
type RespHeartBeatData struct {
	Data struct {
		CreateHeartBeatKey struct {
			Key string `json:"href"`
		} `json:"createHeartbeatMonitor"`
	} `json:"data"`
}

// RespDelete contains boolean returned from deleteAll
type RespDelete struct {
	Data struct {
		DeleteAll bool `json:"deleteAll"`
	} `json:"data"`
}

// NewRequest is a wrapper func to help send the http request
func (c *GraphqlClient) NewRequest(ctx context.Context, method string, body interface{}) ([]byte, error) {

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

	resp, err := ctxhttp.Do(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// CreateService calls GoAlert's GraphQL api to create a new service within GoAlert
func (c *GraphqlClient) CreateService(ctx context.Context, data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createService(input:{name:%s,description:%s,favorite:%t,escalationPolicyID:%s}){id}}`,
		strconv.Quote(data.Name), strconv.Quote(data.Description), data.Favorite, strconv.Quote(data.EscalationPolicyID))

	query = strings.Replace(query, "\t", "", -1)
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
	return r.Data.CreateService.ID, nil
}

// CreateIntegrationKey calls GoAlert's GraphQL api to create a new integration key
func (c *GraphqlClient) CreateIntegrationKey(ctx context.Context, data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createIntegrationKey(input:{serviceID:%s,type:%s,name:%s}){href}}`,
		strconv.Quote(data.Id), data.Type, strconv.Quote(data.Name))

	query = strings.Replace(query, "\t", "", -1)
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

	return r.Data.CreateIntKey.Key, nil
}

// CreateHeartbeatMonitor calls GoAlert's GraphQL api to create a new heartbeat monitor for a GoAlert Service
func (c *GraphqlClient) CreateHeartbeatMonitor(ctx context.Context, data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createHeartbeatMonitor(input: {serviceID: %s,name: %s,timeoutMinutes: %d }){href}}`,
		strconv.Quote(data.Id), strconv.Quote(data.Name), data.Timeout)

	query = strings.Replace(query, "\t", "", -1)
	body := Q{Query: query}
	respData, err := c.NewRequest(ctx, "POST", body)
	if err != nil {
		return "", err
	}

	var r RespHeartBeatData
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
	}
	return r.Data.CreateHeartBeatKey.Key, nil
}

// DeleteService calls GoAlert's GraphQL API to delete a GoAlert service
func (c *GraphqlClient) DeleteService(ctx context.Context, data *Data) error {
	query := fmt.Sprintf(`mutation {
			deleteAll(input: {
				id: %s,
				type: service
			})
		}`, strconv.Quote(data.Id))

	query = strings.Replace(query, "\t", "", -1)
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
