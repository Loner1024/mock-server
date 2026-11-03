package decision

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSystemOneRoute(t *testing.T) {
	router := newTestRouter()
	body := []byte(`{
		"model": "jev-latest",
		"state": "Customer wants a refund",
		"questions": {
			"refund": {"type": "noul", "instructions": "Is this a refund?"},
			"team": {"type": "choice", "instructions": "Which team?", "criteria": {"billing": "payments", "technical": "bugs"}},
			"urgency": {"type": "score", "instructions": "How urgent?", "criteria": ["low", "high"]}
		}
	}`)
	response := perform(router, "/v1/systemone", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload struct {
		Model   string                     `json:"model"`
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Model != "jev-latest" {
		t.Fatalf("model = %q", payload.Model)
	}
	assertJSON(t, payload.Answers["refund"], `{"type":"noul","noul":0.75}`)
	assertJSON(t, payload.Answers["team"], `{
		"type":"choice","choice":"billing","confidence":0.9,
		"probabilities":{"billing":0.9,"technical":0.1}
	}`)
	assertJSON(t, payload.Answers["urgency"], `{
		"type":"score","score":0.5,"confidence":0.8,
		"legend":{"0":"low","1":"high"},
		"probabilities":{"0":0.5,"1":0.5}
	}`)
}

func TestDecisionsRoute(t *testing.T) {
	router := newTestRouter()
	body := []byte(`{
		"model": "luna",
		"input": "Customer wants a refund",
		"questions": [
			{"type": "predicate", "name": "refund", "instructions": "Is this a refund?"},
			{"type": "choice", "name": "team", "instructions": "Which team?", "choices": [{"value": "billing"}, {"value": "technical"}]},
			{"type": "score", "name": "urgency", "instructions": "How urgent?", "levels": [{"label": "low"}, {"label": "high"}]}
		]
	}`)
	response := perform(router, "/v1/decisions", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload struct {
		Model   string           `json:"model"`
		Answers []map[string]any `json:"answers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Model != "luna" {
		t.Fatalf("model = %q", payload.Model)
	}
	if len(payload.Answers) != 3 {
		t.Fatalf("answers = %d", len(payload.Answers))
	}
	if payload.Answers[0]["type"] != "predicate" || payload.Answers[0]["probability"] != 0.75 {
		t.Fatalf("predicate answer = %#v", payload.Answers[0])
	}
	if payload.Answers[1]["choice"] != "billing" {
		t.Fatalf("choice answer = %#v", payload.Answers[1])
	}
	if payload.Answers[2]["score"] != 0.5 {
		t.Fatalf("score answer = %#v", payload.Answers[2])
	}
}

func TestDecisionRoutesRejectInvalidBodies(t *testing.T) {
	router := newTestRouter()
	for _, path := range []string{"/v1/systemone", "/v1/decisions"} {
		response := perform(router, path, []byte(`{"model":"m","questions":[]}`))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", path, response.Code)
		}
	}
}

func TestSystemOneRejectsEmptyChoiceCriteriaAndScoreLevels(t *testing.T) {
	router := newTestRouter()
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "empty choice criteria",
			body: `{"model":"m","state":"x","questions":{"q":{"type":"choice","criteria":{}}}}`,
			want: "choice criteria must not be empty",
		},
		{
			name: "null choice criteria",
			body: `{"model":"m","state":"x","questions":{"q":{"type":"choice","criteria":null}}}`,
			want: "choice criteria must not be empty",
		},
		{
			name: "empty score criteria",
			body: `{"model":"m","state":"x","questions":{"q":{"type":"score","criteria":[]}}}`,
			want: "score criteria must not be empty",
		},
		{
			name: "null score criteria",
			body: `{"model":"m","state":"x","questions":{"q":{"type":"score","criteria":null}}}`,
			want: "score criteria must not be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := perform(router, "/v1/systemone", []byte(tt.body))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body.String())
			}
			var payload struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(payload.Error, tt.want) {
				t.Fatalf("error = %q, want %q", payload.Error, tt.want)
			}
		})
	}
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	SetupRoutes(router)
	return router
}

func perform(router http.Handler, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertJSON(t *testing.T, actual json.RawMessage, expected string) {
	t.Helper()
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(actualValue, expectedValue) {
		t.Fatalf("actual = %s, expected = %s", actual, expected)
	}
}

func jsonEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}
