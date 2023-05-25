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

// Client is a wrapper interface for the graphqlClient to allow for easier testing
type Client interface {
	CreateService(data *Data, sessionCookie *http.Cookie) (string, error)
	CreateIntegrationKey(data *Data, sessionCookie *http.Cookie) (string, error)
	CreateHeartbeatMonitor(data *Data, sessionCookie *http.Cookie) (string, error)
	DeleteService(data *Data, sessionCookie *http.Cookie)
}

// Wrapper for HTTP client
type graphqlClient struct {
	BaseURL    *url.URL
	httpClient *http.Client
}

// Data describes the data that is needed for Goalert GraphQL api calls
type Data struct {
	Name               string `json:"name"`
	Id                 string `json:"id"`
	Description        string `json:"description,omitempty"`
	Favorite           bool   `json:"favorite,omitempty"`
	EscalationPolicyID string `json:"escalationPolicyID"`
	Type               string `json:"type"`
	Timeout            int    `json:"timeoutMinutes"`
	DeleteAll          bool   `json:"deleteAll"`
}

// Wrapper func to help send the http request
func (c *graphqlClient) newRequest(method string, body interface{}, sessionCookie *http.Cookie) (*Data, error) {

	var respData Data
	goalertApiEndpoint := os.Getenv(config.GoalertApiEndpointEnvVar)
	var buf io.ReadWriter

	if body != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(method, goalertApiEndpoint+"/api/graphql", buf)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie)

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
func (c *graphqlClient) CreateService(data *Data, sessionCookie *http.Cookie) (string, error) {

	createClusterSvcData := map[string]string{
		"mutation": fmt.Sprintf(
			`{createService(input: {
				name: %s,
				description: %s,
				favorite: %t,
				escalationPolicyID: %s
			}){
				id
			}`, data.Name, data.Description, data.Favorite, data.EscalationPolicyID),
	}

	respData, err := c.newRequest("POST", createClusterSvcData, sessionCookie)
	if err != nil {
		return "", err
	}
	return respData.Id, nil
}

// Creates new integration key
func (c *graphqlClient) CreateIntegrationKey(data *Data, sessionCookie *http.Cookie) (string, error) {

	createIntegrationKeyData := map[string]string{
		"mutation": fmt.Sprintf(`{
				createIntegrationKey(input: {
					serviceID: %s,
					type: %s,
					name: %s
				}){id}
			}`, data.Id, data.Type, data.Name),
	}

	respData, err := c.newRequest("POST", createIntegrationKeyData, sessionCookie)
	if err != nil {
		return "", err
	}
	return respData.Id, nil
}

// Creates new heartbeatmonitor
func (c *graphqlClient) CreateHeartbeatMonitor(data *Data, sessionCookie *http.Cookie) (string, error) {

	createHeartbeatMonitorData := map[string]string{
		"mutation": fmt.Sprintf(`{
			createHeartbeatMonitor(input: {
				serviceID: %s,
				name: %s,
				timeoutMinutes: %d 
			}){id}
		}`, data.Id, data.Name, data.Timeout),
	}

	respData, err := c.newRequest("POST", createHeartbeatMonitorData, sessionCookie)
	if err != nil {
		return "", err
	}
	return respData.Id, nil
}

// Deletes service
func (c *graphqlClient) DeleteService(data *Data, sessionCookie *http.Cookie) error {
	deleteSvcData := map[string]string{
		"mutation": fmt.Sprintf(`{
			deleteAll(input: {
				id: %s,
				type: service
			})
		}`, data.Id),
	}

	respData, err := c.newRequest("POST", deleteSvcData, sessionCookie)
	if err != nil {
		return err
	}
	if !respData.DeleteAll {
		fmt.Print("Failed to delete service")
	}
	return nil
}
