package goalert

import (
	"encoding/json"
	"golang.org/x/net/context"
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
	ctx := context.Background()
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
	respBytes, err := client.NewRequest(ctx, method, body)
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

	tests := []struct {
		name        string
		data        *Data
		expectedID  string
		respData    []byte
		expectedErr bool
	}{
		{
			name: "Successful createService",
			data: &Data{
				Name:               "Test",
				Description:        "Test service",
				Favorite:           false,
				EscalationPolicyID: "123",
			},
			expectedID:  "456",
			respData:    []byte(`{"data": {"createService": {"id": "456"}}}`),
			expectedErr: false,
		},
		{
			name: "Unsuccessful createService",
			data: &Data{
				Name:               "Test2",
				Description:        "Test service",
				Favorite:           false,
				EscalationPolicyID: "123-bad",
			},
			expectedID:  "",
			respData:    []byte(`{"data":{"createService":null}}`),
			expectedErr: false,
		},
		{
			name: "Failed unmarshalling response",
			data: &Data{
				Name:               "Test3",
				Description:        "Test service",
				Favorite:           false,
				EscalationPolicyID: "890",
			},
			expectedID:  "",
			respData:    []byte(`nmuyrufcewrqrew`),
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := test.respData

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

			actualID, err := mockClient.CreateService(ctx, test.data)
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, test.expectedID, actualID)
				assert.Nil(t, err)
			}
		})
	}

}

func Test_CreateIntegrationKey(t *testing.T) {

	tests := []struct {
		name        string
		data        *Data
		expectedKey string
		respData    []byte
		expectedErr bool
	}{
		{
			name: "Successful createIntegrationKey",
			data: &Data{
				Id:   "123",
				Type: "test",
				Name: "Test Integration Key",
			},
			expectedKey: "/integration-keys/123",
			respData:    []byte(`{"data":{"createIntegrationKey":{"href":"/integration-keys/123"}}}`),
			expectedErr: false,
		},
		{
			name: "Unsuccessful createIntegrationKey",
			data: &Data{
				Id:   "123-badID",
				Type: "test",
				Name: "Test Integration Key",
			},
			expectedKey: "",
			respData:    []byte(`{"data":{"createIntegrationKey":null}}`),
			expectedErr: false,
		},
		{
			name: "Failed unmarshalling response",
			data: &Data{
				Id:   "123",
				Type: "test",
				Name: "Test Integration Key",
			},
			expectedKey: "",
			respData:    []byte(`vfrsbhtnrhurdsbvr`),
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := test.respData

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

			key, err := mockClient.CreateIntegrationKey(ctx, test.data)
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, test.expectedKey, key)
				assert.Nil(t, err)
			}
		})
	}
}

func Test_CreateHeartbeatMonitor(t *testing.T) {

	tests := []struct {
		name        string
		data        *Data
		expectedKey string
		respData    []byte
		expectedErr bool
	}{
		{
			name: "Successful createHeartbeatMonitor",
			data: &Data{
				Id:      "123",
				Name:    "Test Heartbeat Monitor",
				Timeout: 15,
			},
			expectedKey: "/heartbeat-monitors/123",
			respData:    []byte(`{"data":{"createHeartbeatMonitor":{"href":"/heartbeat-monitors/123"}}}`),
			expectedErr: false,
		},
		{
			name: "Unsuccessful createHeartbeatMonitor",
			data: &Data{
				Id:      "123-badID",
				Name:    "Test Heartbeat Monitor",
				Timeout: 15,
			},
			expectedKey: "",
			respData:    []byte(`{"data":{"createHeartbeatMonitor":null}}`),
			expectedErr: false,
		},
		{
			name: "Failed unmarshalling response",
			data: &Data{
				Id:      "123",
				Name:    "Test Heartbeat Monitor",
				Timeout: 15,
			},
			expectedKey: "",
			respData:    []byte(`tsrgafcvarvsgtrb`),
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := test.respData

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

			key, err := mockClient.CreateHeartbeatMonitor(ctx, test.data)
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, test.expectedKey, key)
				assert.Nil(t, err)
			}
		})
	}
}

func TestDeleteService(t *testing.T) {

	tests := []struct {
		name        string
		data        *Data
		respData    []byte
		expectedErr bool
	}{
		{
			name: "Successful deleteAll",
			data: &Data{
				Id: "123",
			},
			respData:    []byte(`{"data":{"deleteAll":true}}`),
			expectedErr: false,
		},
		{
			name: "Unsuccessful deleteAll",
			data: &Data{
				Id: "123-badID",
			},
			respData:    []byte(`{"data":null}`),
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := test.respData

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

			err := mockClient.DeleteService(ctx, test.data)
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
