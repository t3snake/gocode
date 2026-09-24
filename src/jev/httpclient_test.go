package jev

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testClient(t *testing.T, transport roundTripFunc) *Client {
	t.Helper()
	t.Setenv("TYPESAFE_API_KEY", "test-key")
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	client.http_client.Transport = transport
	return client
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

var one_question = map[string]Question{
	"urgent": {Type: Noul, Instructions: "Is this urgent?"},
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	for _, key := range []string{"", " \t\n"} {
		t.Setenv("TYPESAFE_API_KEY", key)
		if _, err := NewClient(); err == nil {
			t.Fatalf("key %q: expected an error", key)
		}
	}
}

func TestEvaluateRequestAndResponse(t *testing.T) {
	questions := map[string]Question{
		"urgent":  {Type: Noul, Instructions: "Is this urgent?"},
		"team":    {Type: Choice, Instructions: map[string]string{"question": "Which team?"}, Criteria: map[string]any{"billing": nil, "technical": "Bugs"}},
		"feeling": {Type: Score, Instructions: []string{"Rate frustration"}, Criteria: []string{"Calm", "Angry"}},
	}
	const want_request = `{
		"state": {"message": "hello"},
		"model": "jev-latest",
		"questions": {
			"urgent": {"type": "noul", "instructions": "Is this urgent?"},
			"team": {"type": "choice", "instructions": {"question": "Which team?"}, "criteria": {"billing": null, "technical": "Bugs"}},
			"feeling": {"type": "score", "instructions": ["Rate frustration"], "criteria": ["Calm", "Angry"]}
		}
	}`
	const server_response = `{
		"model": "jev-1",
		"answers": {
			"urgent": {"type": "noul", "noul": 0.95},
			"team": {"type": "choice", "choice": "billing", "probabilities": {"billing": 0.88, "technical": 0.12}, "confidence": 0.81},
			"feeling": {"type": "score", "score": 0.5, "probabilities": {"0": 0.3, "1": 0.7}, "confidence": 0.7, "legend": {"0": "Calm", "1": "Angry"}}
		},
		"usage": {"input_tokens": 10, "output_tokens": 2}
	}`

	client := testClient(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://api.typesafe.ai/v1/systemone" {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var got, want any
		if err := json.UnmarshalRead(request.Body, &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(want_request), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("request body = %#v, want %#v", got, want)
		}
		return response(http.StatusOK, server_response), nil
	})

	got, err := client.Evaluate(context.Background(), map[string]any{"message": "hello"}, questions)
	if err != nil {
		t.Fatal(err)
	}
	want := JevResponse{
		Model: "jev-1",
		Answers: map[string]Answer{
			"urgent":  {Type: Noul, Noul: 0.95},
			"team":    {Type: Choice, Choice: "billing", Probabilities: map[string]float64{"billing": 0.88, "technical": 0.12}, Confidence: 0.81},
			"feeling": {Type: Score, Score: 0.5, Probabilities: map[string]float64{"0": 0.3, "1": 0.7}, Confidence: 0.7, Legend: map[string]string{"0": "Calm", "1": "Angry"}},
		},
		Usage: UsageJevResponse{InputTokens: 10, OutputTokens: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("response = %#v, want %#v", got, want)
	}
}

func TestEvaluateRejectsInvalidRequests(t *testing.T) {
	test_cases := []struct {
		name          string
		uninitialized bool
		ctx           context.Context
		state         any
	}{
		{"uninitialized client", true, context.Background(), "state"},
		{"nil context", false, nil, "state"},
		{"unencodable state", false, context.Background(), make(chan int)},
	}
	for _, tt := range test_cases {
		t.Run(tt.name, func(t *testing.T) {
			client := testClient(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("unexpected HTTP request")
				return nil, nil
			})
			if tt.uninitialized {
				client = &Client{}
			}
			got, err := client.Evaluate(tt.ctx, tt.state, one_question)
			if err == nil || !reflect.DeepEqual(got, JevResponse{}) {
				t.Fatalf("Evaluate() = (%#v, %v), want an error and empty response", got, err)
			}
		})
	}

	client := testClient(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected HTTP request")
		return nil, nil
	})
	got, err := client.Evaluate(context.Background(), "state", nil)
	if err != nil || !reflect.DeepEqual(got, JevResponse{}) {
		t.Fatalf("empty questions = (%#v, %v), want empty response without HTTP", got, err)
	}
}

func TestEvaluateHTTPError(t *testing.T) {
	const body = `{"detail":"rate limited"}`
	calls := 0
	client := testClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusTooManyRequests, body), nil
	})
	got, err := client.Evaluate(context.Background(), "state", one_question)
	var api_err *APIError
	if !errors.As(err, &api_err) || api_err.StatusCode != http.StatusTooManyRequests || api_err.Body != body ||
		!reflect.DeepEqual(got, JevResponse{}) {
		t.Fatalf("Evaluate() = (%#v, %v), want APIError with status and body", got, err)
	}
	if calls != 1 {
		t.Errorf("HTTP requests = %d, want 1 (no retries)", calls)
	}
}

func TestEvaluateResponseErrors(t *testing.T) {
	test_cases := []struct {
		name string
		body string
	}{
		{"malformed JSON", `not json`},
		{"wrong answer count", `{"answers":{}}`},
	}
	for _, tt := range test_cases {
		t.Run(tt.name, func(t *testing.T) {
			client := testClient(t, func(*http.Request) (*http.Response, error) {
				return response(http.StatusOK, tt.body), nil
			})
			got, err := client.Evaluate(context.Background(), "state", one_question)
			if err == nil || !reflect.DeepEqual(got, JevResponse{}) {
				t.Fatalf("Evaluate() = (%#v, %v), want an error and empty response", got, err)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, err_read }

var err_read = errors.New("read failed")

func TestEvaluateIOErrors(t *testing.T) {
	t.Run("send", func(t *testing.T) {
		transport_err := errors.New("send failed")
		client := testClient(t, func(*http.Request) (*http.Response, error) {
			return nil, transport_err
		})
		_, err := client.Evaluate(context.Background(), "state", one_question)
		if !errors.Is(err, transport_err) {
			t.Fatalf("error = %v, want send failure", err)
		}
	})

	t.Run("read", func(t *testing.T) {
		client := testClient(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{})}, nil
		})
		_, err := client.Evaluate(context.Background(), "state", one_question)
		if !errors.Is(err, err_read) {
			t.Fatalf("error = %v, want read failure", err)
		}
	})
}
