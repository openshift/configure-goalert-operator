package goalert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/openshift/configure-goalert-operator/config"
)

// Client is a wrapper interface for the GraphqlClient to allow for easier testing
type Client interface {
	CreateService(data *Data) (string, error)
	CreateIntegrationKey(data *Data) (string, error)
	CreateHeartbeatMonitor(data *Data) (string, error)
	DeleteService(data *Data) error
}

func defaultURL() *url.URL {
	url, _ := url.Parse(os.Getenv(config.GoalertApiEndpointEnvVar))
	return url
}

// Wrapper for HTTP client
type GraphqlClient struct {
	BaseURL       *url.URL
	httpClient    *http.Client
	sessionCookie *http.Cookie
}

func NewClient(sessionCookie *http.Cookie) Client {
	return &GraphqlClient{
		BaseURL:       defaultURL(),
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

// Wrapper func to help send the http request
func (c *GraphqlClient) NewRequest(method string, body interface{}) (*Data, error) {

	goalertApiEndpoint := os.Getenv(config.GoalertApiEndpointEnvVar)
	rel := &url.URL{Path: goalertApiEndpoint}
	u := c.BaseURL.ResolveReference(rel)
	var respData Data
	var buf io.ReadWriter

	if body != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(method, u.String()+"/api/graphql", buf)
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
	err = json.Unmarshal(respBytes, &respData)
	if err != nil {
		return nil, err
	}
	return &respData, nil
}

// Creates new service
func (c *GraphqlClient) CreateService(data *Data) (string, error) {

	createClusterSvcData := map[string]string{
		"mutation": fmt.Sprintf(`{createService(input:{name:%s,description:%s,favorite:%t,escalationPolicyID:%s}){id}`, data.Name, data.Description, data.Favorite, data.EscalationPolicyID)}

	respData, err := c.NewRequest("POST", createClusterSvcData)
	if err != nil {
		return "", err
	}
	return respData.Id, nil
}

// Creates new integration key
func (c *GraphqlClient) CreateIntegrationKey(data *Data) (string, error) {

	createIntegrationKeyData := map[string]string{
		"mutation": fmt.Sprintf(`{
				createIntegrationKey(input: {
					serviceID: %s,
					type: %s,
					name: %s
				}){id}
			}`, data.Id, data.Type, data.Name),
	}

	respData, err := c.NewRequest("POST", createIntegrationKeyData)
	if err != nil {
		return "", err
	}
	return respData.Id, nil
}

// Creates new heartbeatmonitor
func (c *GraphqlClient) CreateHeartbeatMonitor(data *Data) (string, error) {

	createHeartbeatMonitorData := map[string]string{
		"mutation": fmt.Sprintf(`{
			createHeartbeatMonitor(input: {
				serviceID: %s,
				name: %s,
				timeoutMinutes: %d 
			}){id}
		}`, data.Id, data.Name, data.Timeout),
	}

	respData, err := c.NewRequest("POST", createHeartbeatMonitorData)
	if err != nil {
		return "", err
	}
	return respData.Id, nil
}

// Deletes service
func (c *GraphqlClient) DeleteService(data *Data) error {
	deleteSvcData := map[string]string{
		"mutation": fmt.Sprintf(`{
			deleteAll(input: {
				id: %s,
				type: service
			})
		}`, data.Id),
	}

	respData, err := c.NewRequest("POST", deleteSvcData)
	if err != nil {
		return err
	}
	if !respData.DeleteAll {
		fmt.Print("Failed to delete service")
	}
	return nil
}
