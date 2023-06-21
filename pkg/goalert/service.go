package goalert

import (
	"bytes"
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

// Client is a wrapper interface for the GraphqlClient to allow for easier testing
type Client interface {
	CreateService(data *Data) (string, error)
	CreateIntegrationKey(data *Data) (string, error)
	CreateHeartbeatMonitor(data *Data) (string, error)
	DeleteService(data *Data) error
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

// q describes GraphQL query payload
type q struct {
	Query string
}

// RespSvData describes Svc ID returned from createService
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

// RespHeartBeatData describes heartbeatmonitor key from createHeartbeatMonitor
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
		Bool bool `json:"deleteAll"`
	} `json:"data"`
}

// Wrapper func to help send the http request
func (c *GraphqlClient) NewRequest(method string, body interface{}) ([]byte, error) {

	goalertApiEndpoint := os.Getenv(config.GoalertApiEndpointEnvVar)

	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(method, goalertApiEndpoint+"/api/graphql", bytes.NewBuffer(data))
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
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// Creates new service
func (c *GraphqlClient) CreateService(data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createService(input:{name:%s,description:%s,favorite:%t,escalationPolicyID:%s}){id}}`,
		strconv.Quote(data.Name), strconv.Quote(data.Description), data.Favorite, strconv.Quote(data.EscalationPolicyID))

	query = strings.Replace(query, "\t", "", -1)
	body := q{Query: query}
	respData, err := c.NewRequest("POST", body)
	if err != nil {
		return "", err
	}

	var r RespSvcData
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", err
	}
	return r.Data.CreateService.ID, nil
}

// Creates new integration key
func (c *GraphqlClient) CreateIntegrationKey(data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createIntegrationKey(input:{serviceID:%s,type:%s,name:%s}){href}}`,
		strconv.Quote(data.Id), data.Type, strconv.Quote(data.Name))

	query = strings.Replace(query, "\t", "", -1)
	body := q{Query: query}
	respData, err := c.NewRequest("POST", body)
	if err != nil {
		return "", err
	}

	var r RespIntKeyData
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", err
	}

	return r.Data.CreateIntKey.Key, nil
}

// Creates new heartbeatmonitor
func (c *GraphqlClient) CreateHeartbeatMonitor(data *Data) (string, error) {

	query := fmt.Sprintf(`mutation {createHeartbeatMonitor(input: {serviceID: %s,name: %s,timeoutMinutes: %d }){href}}`,
		strconv.Quote(data.Id), strconv.Quote(data.Name), data.Timeout)

	query = strings.Replace(query, "\t", "", -1)
	body := q{Query: query}
	respData, err := c.NewRequest("POST", body)
	if err != nil {
		return "", err
	}

	var r RespHeartBeatData
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return "", err
	}
	return r.Data.CreateHeartBeatKey.Key, nil
}

// Deletes service
func (c *GraphqlClient) DeleteService(data *Data) error {
	query := fmt.Sprintf(`mutation {
			deleteAll(input: {
				id: %s,
				type: service
			})
		}`, strconv.Quote(data.Id))

	query = strings.Replace(query, "\t", "", -1)
	body := q{Query: query}
	respData, err := c.NewRequest("POST", body)
	if err != nil {
		return err
	}

	var r RespDelete
	err = json.Unmarshal(respData, &r)
	if err != nil {
		return err
	}

	if !r.Data.Bool {
		return errors.New("failed to delete service")
	}
	return nil
}
