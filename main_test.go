package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func seedRequests(tb testing.TB, count int) {
	tb.Helper()
	mutex.Lock()
	oldStore, oldIDs, oldNextID := requestsStore, requestIDs, nextID
	requestsStore = make(map[int]RequestInfo)
	requestIDs = nil
	nextID = count
	for id := 0; id < count; id++ {
		requestsStore[id] = RequestInfo{
			ID: id, Method: "POST", Path: fmt.Sprintf("/sample/%d", id),
			RequestURI:  fmt.Sprintf("/sample/%d?tag=one&tag=two", id),
			QueryParams: url.Values{"tag": {"one", "two"}}, Proto: "HTTP/1.1",
			Headers:   http.Header{"Content-Type": {"application/json"}, "X-Sample": {"one", "two"}},
			Body:      `{"message":"<script>alert('sample')</script>"}`,
			Timestamp: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC), RemoteAddr: "127.0.0.1:1234",
		}
		requestIDs = append(requestIDs, id)
	}
	mutex.Unlock()
	tb.Cleanup(func() {
		mutex.Lock()
		requestsStore, requestIDs, nextID = oldStore, oldIDs, oldNextID
		mutex.Unlock()
	})
}

func TestRequestDetailsAPI(t *testing.T) {
	seedRequests(t, 2)
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"details", "GET", "/__tiny_requestbin/api/requests/1", http.StatusOK},
		{"missing", "GET", "/__tiny_requestbin/api/requests/999", http.StatusNotFound},
		{"invalid", "GET", "/__tiny_requestbin/api/requests/invalid", http.StatusBadRequest},
		{"zero", "GET", "/__tiny_requestbin/api/requests/0", http.StatusOK},
		{"empty", "GET", "/__tiny_requestbin/api/requests/", http.StatusBadRequest},
		{"negative", "GET", "/__tiny_requestbin/api/requests/-1", http.StatusBadRequest},
		{"nested", "GET", "/__tiny_requestbin/api/requests/1/extra", http.StatusBadRequest},
		{"method", "POST", "/__tiny_requestbin/api/requests/1", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, tc.status, response.Body.String())
			}
			if tc.status == http.StatusOK {
				if got := response.Header().Get("Content-Type"); got != "application/json" {
					t.Fatalf("Content-Type = %q, want JSON instead of a full HTML page", got)
				}
				if got := response.Header().Get("Cache-Control"); got != "no-store" {
					t.Errorf("Cache-Control = %q, want no-store", got)
				}
				var detail RequestInfo
				if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
					t.Fatal(err)
				}
				mutex.RLock()
				wantID := 1
				if tc.name == "zero" {
					wantID = 0
				}
				want := requestsStore[wantID]
				mutex.RUnlock()
				if !reflect.DeepEqual(detail, want) {
					t.Errorf("detail = %+v, want %+v", detail, want)
				}
			}
			if tc.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "GET" {
				t.Error("missing Allow: GET")
			}
		})
	}
	mutex.RLock()
	defer mutex.RUnlock()
	if len(requestsStore) != 2 || nextID != 2 {
		t.Error("detail API requests were captured")
	}
}

func TestMainPageDetails(t *testing.T) {
	seedRequests(t, 2)
	for _, path := range []string{"/?view_id=1", "/?path=missing"} {
		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "</html>") {
			t.Fatalf("page %s failed: %d %s", path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "<script>alert('sample')</script>") {
			t.Error("captured request body rendered as executable HTML")
		}
	}
}

func TestRequestsAPIPath(t *testing.T) {
	seedRequests(t, 3)
	response := httptest.NewRecorder()
	handler(response, httptest.NewRequest(http.MethodGet, "/__tiny_requestbin/api/requests?page=1&limit=1&path=/sample/1", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("list API failed: %d %s", response.Code, response.Body.String())
	}
	var result PaginatedResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Limit != 1 || len(result.Requests) != 1 || result.Requests[0].ID != 1 {
		t.Fatalf("unexpected search result: %+v", result)
	}
	response = httptest.NewRecorder()
	handler(response, httptest.NewRequest(http.MethodPost, "/__tiny_requestbin/api/requests/clear", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("clear API failed: %d %s", response.Code, response.Body.String())
	}
	mutex.RLock()
	defer mutex.RUnlock()
	if len(requestsStore) != 0 || len(requestIDs) != 0 || nextID != 3 {
		t.Error("clear API did not clear the store or changed the next request ID")
	}
}

func TestCommonAPIPathsAreCaptured(t *testing.T) {
	seedRequests(t, 0)
	oldMax := maxRequests
	maxRequests = 10
	t.Cleanup(func() { maxRequests = oldMax })
	for id, path := range []string{"/api/requests?page=1&limit=20&path=demo", "/api/requests/clear", "/api/requests/1?tag=one", "/api/requests/invalid", "/api/requests/1/extra"} {
		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader("sample body")))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Request captured successfully") {
			t.Fatalf("capture %s failed: %d %s", path, response.Code, response.Body.String())
		}
		mutex.RLock()
		captured, ok := requestsStore[id]
		mutex.RUnlock()
		if !ok || captured.RequestURI != path || captured.Body != "sample body" || captured.Method != http.MethodPost {
			t.Fatalf("request %s was not captured correctly: %+v", path, captured)
		}
	}
}

func BenchmarkRequestDetails(b *testing.B) {
	seedRequests(b, 100)
	for _, path := range []string{"/?view_id=1", "/__tiny_requestbin/api/requests/1"} {
		b.Run(path, func(b *testing.B) {
			request := httptest.NewRequest("GET", path, nil)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				response := httptest.NewRecorder()
				handler(response, request)
				b.ReportMetric(float64(response.Body.Len()), "bytes/response")
			}
		})
	}
}
