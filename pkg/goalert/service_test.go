package goalert

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/openshift/configure-goalert-operator/config"
	"github.com/stretchr/testify/assert"
)

func Test_NewRequest(t *testing.T) {
	client := &GraphqlClient{
		sessionCookie: &http.Cookie{
			Name: "test_cookie",
		},
		httpClient: &http.Client{},
	}

	body := struct{ Name string }{"Test body"}
	data, err := json.Marshal(body)
	if err != nil {
		t.Error("Error marshaling JSON:", err)
	}

	method := "POST"
	endpoint := "/api/graphql"

	// create a new http test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			t.Errorf("Expected %s request, got %s", method, r.Method)
		}
		if r.URL.String() != endpoint {
			t.Errorf("Expected %s endpoint, got %s", endpoint, r.URL)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Expected Content-Type header to be application/json, got %s", ct)
		}
		if ac := r.Header.Get("Accept"); ac != "application/json" {
			t.Errorf("Expected Accept header to be application/json, got %s", ac)
		}
		if sc := r.Header.Get("Cookie"); sc != "test_cookie=" {
			t.Errorf("Expected Cookie header to contain test_cookie, got %s", sc)
		}

		reqBody, _ := io.ReadAll(r.Body)
		if !reflect.DeepEqual(reqBody, data) {
			t.Errorf("Expected request body %v, got %v", string(data), string(reqBody))
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("mock response"))
		if err != nil {
			t.Log(err)
		}
	}))
	defer ts.Close()

	// set the test server's URL as our GoAlert API endpoint
	t.Setenv(config.GoalertApiEndpointEnvVar, ts.URL)

	// make the request
	respBytes, err := client.NewRequest(method, body)
	if err != nil {
		t.Error("Error making HTTP request:", err)
	}

	// check the response
	expectedResponse := []byte("mock response")
	if !reflect.DeepEqual(respBytes, expectedResponse) {
		t.Errorf("Expected response %v, got %v", string(expectedResponse), string(respBytes))
	}
}

func Test_CreateService(t *testing.T) {

	mockRespData := []byte(`{"data": {"createService": {"id": "456"}}}`)

	// Test successful response
	data := &Data{
		Name:               "Test",
		Description:        "Test service",
		Favorite:           false,
		EscalationPolicyID: "123",
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := mockRespData

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(resp); err != nil {
			t.Fatalf("Unexpected error writing response from httptest server")
		}
	}))
	defer mockServer.Close()

	t.Setenv(config.GoalertApiEndpointEnvVar, mockServer.URL)

	mockClient := &GraphqlClient{
		sessionCookie: &http.Cookie{
			Name: "test_cookie",
		},
		httpClient: mockServer.Client(),
	}
	expectedID := "456"
	actualID, err := mockClient.CreateService(data)
	assert.NoError(t, err)
	assert.Equal(t, expectedID, actualID)
}

func Test_CreateIntegrationKey(t *testing.T) {

	// Define expected input data and response data
	testData := &Data{
		Id:   "123",
		Type: "test",
		Name: "Test Integration Key",
	}
	expectedResponse := []byte(`{"data":{"createIntegrationKey":{"href":"/integration-keys/123"}}}`)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := expectedResponse

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(resp); err != nil {
			t.Fatalf("Unexpected error writing response from httptest server")
		}
	}))
	defer mockServer.Close()

	t.Setenv(config.GoalertApiEndpointEnvVar, mockServer.URL)

	mockClient := &GraphqlClient{
		sessionCookie: &http.Cookie{
			Name: "test_cookie",
		},
		httpClient: mockServer.Client(),
	}

	key, err := mockClient.CreateIntegrationKey(testData)

	// Assert that the function returns the expected result
	expectedKey := "/integration-keys/123"
	assert.NoError(t, err)
	assert.Equal(t, expectedKey, key)
}

func TestCreateHeartbeatMonitor(t *testing.T) {

	// Define expected input data and response data
	testData := &Data{
		Id:      "123",
		Name:    "Test Heartbeat Monitor",
		Timeout: 15,
	}

	expectedResponse := []byte(`{"data":{"createHeartbeatMonitor":{"href":"/heartbeat-monitors/123"}}}`)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := expectedResponse

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(resp); err != nil {
			t.Fatalf("Unexpected error writing response from httptest server")
		}
	}))
	defer mockServer.Close()

	t.Setenv(config.GoalertApiEndpointEnvVar, mockServer.URL)

	mockClient := &GraphqlClient{
		sessionCookie: &http.Cookie{
			Name: "test_cookie",
		},
		httpClient: mockServer.Client(),
	}
	key, err := mockClient.CreateHeartbeatMonitor(testData)

	// Assert that the function returns the expected result
	expectedKey := "/heartbeat-monitors/123"
	assert.NoError(t, err)
	assert.Equal(t, expectedKey, key)
}

func TestDeleteService(t *testing.T) {

	// Define expected input data and response data
	testData := &Data{
		Id: "123",
	}

	expectedResponse := []byte(`{"data":{"deleteAll":{"bool":true}}}`)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := expectedResponse

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(resp); err != nil {
			t.Fatalf("Unexpected error writing response from httptest server")
		}
	}))
	defer mockServer.Close()

	t.Setenv(config.GoalertApiEndpointEnvVar, mockServer.URL)

	mockClient := &GraphqlClient{
		sessionCookie: &http.Cookie{
			Name: "test_cookie",
		},
		httpClient: mockServer.Client(),
	}

	err := mockClient.DeleteService(testData)

	// Assert that the function returns the expected result
	assert.NoError(t, err)
}
