package httptrack_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zafnz/go-httptrack"
)

func ExampleHandler() {
	mux := http.NewServeMux()

	// Create the middleware hander, here we are specifying that the inbound HTTP header named
	// "x-tracking-id" should be copied to all outbound calls, by setting their HTTP header to
	// the same value.
	// In addition, the inbound HTTP cookie named "session-id" should be converted into a HTTP
	// header and set for all outbound calls.
	handler := httptrack.Handler(mux, httptrack.Options{}, []httptrack.Value{
		{httptrack.LocationHeader, "x-tracking-id", httptrack.LocationHeader, "x-tracking-id", nil},
		{httptrack.LocationCookie, "session-id", httptrack.LocationHeader, "x-client-session-id", nil},
	})

	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// This Get request will go out with the x-tracking-id and x-client-session-id header
		// values set. (assuming the correct inbound values came in, see MissingFunc example)
		httptrack.Get(ctx, "http://microservice1.example.com/serviceCall")
	})

	http.ListenAndServe("127.0.0.1:3000", handler)
}

func ExampleHandler_missingfunc() {
	mux := http.NewServeMux()

	missingFuncHandler := func(name string, r http.Request) string {
		// Here we have access to the incoming request, and need to generate the x-tracking-id header
		// value. If we return empty string no x-tracking-id will be set.
		return "blah"
	}

	// The inbound HTTP header x-tracking-id should be copied to all outbound calls as an HTTP header
	// with the same name. If the inbound call does not have an x-tracking-id header, then missingFuncHandler()
	// is called, supplying "x-tracking-id" and a copy of the inbound http.Request
	handler := httptrack.Handler(mux, httptrack.Options{}, []httptrack.Value{
		{httptrack.LocationHeader, "x-tracking-id", httptrack.LocationHeader, "x-tracking-id", missingFuncHandler},
	})

	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// If the inbound request does not have x-tracking-id set, then this outbound call will
		// have the x-tracking-id set to "blah"
		httptrack.Get(ctx, "http://microservice1.example.com/serviceCall")
	})

	http.ListenAndServe("127.0.0.1:3000", handler)
}

func testCall(t *testing.T, req *http.Request, trackOptions httptrack.Options, trackVals []httptrack.Value, testHandler http.HandlerFunc) {
	// Setup our various pass throughs.
	middleware := httptrack.Handler(testHandler, trackOptions, trackVals)
	rr := httptest.NewRecorder()
	// Make the call!
	middleware.ServeHTTP(rr, req)
}

func outboundCall(t *testing.T, ctx context.Context) {
	// We will make our call outbound, and because we are passing ctx around the
	// various headers, cookies, and query parameters should be set.

	newReq, err := httptrack.NewRequestWithContext(ctx, "GET", "/downstream", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Validate that the various values have been set in the newReq.
	if newReq.Header.Get("x-new-header") != "header-value" {
		t.Error("header value was not passed through")
	}
	cookie, err := newReq.Cookie("new-cookie")
	if err != nil || cookie.Value != "cookie-value" {
		t.Error("cookie value was not passed through")
	}
	if newReq.URL.Query().Get("new-query") != "query-value" {
		t.Error("query value was not passed through")
	}
	// Make sure that the Inbound names weren't used
	if newReq.Header.Get("x-header") != "" {
		t.Error("inbound name used as outbound name")
	}
	cookie, err = newReq.Cookie("cookie")
	if err != nil {
		if cookie != nil && cookie.Value != "" {
			t.Error("inbound cookie name used as outbound cookie name")
		}
	}
	if newReq.URL.Query().Get("query") != "" {
		t.Error("inbound query name used as outbound query name")
	}
}
func TestHandler(t *testing.T) {
	// Setup our various pass throughs.
	trackValues := []httptrack.Value{
		{httptrack.LocationHeader, "x-header", httptrack.LocationHeader, "x-new-header", nil},
		{httptrack.LocationQueryParam, "query", httptrack.LocationQueryParam, "new-query", nil},
		{httptrack.LocationCookie, "cookie", httptrack.LocationCookie, "new-cookie", nil},
	}

	// This is the external client making it's call to us. (or probably the loadbalancer making
	// the call from the client to us)
	req, err := http.NewRequest("GET", "/test?query=query-value", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-header", "header-value")
	req.AddCookie(&http.Cookie{Name: "cookie", Value: "cookie-value"})

	// Make the call!
	testCall(t, req, httptrack.Options{}, trackValues, func(w http.ResponseWriter, r *http.Request) {
		// Make an outbound call that should have the tracking things set
		outboundCall(t, r.Context())
	})
}

func TestLostContext(t *testing.T) {
	ctx := context.Background()
	req, err := httptrack.NewRequestWithContext(ctx, "GET", "/", nil)
	if err != nil {
		t.Fatal("NewRequestWithContext with context.Background() returned an error")
	}
	err = httptrack.AddContextData(req)
	if !errors.Is(err, httptrack.ErrMissingContext) {
		t.Fatal("AddContextData did not return error when context was lost")
	}
}

func TestNoContext(t *testing.T) {

	// Setup our various pass throughs.
	trackValues := []httptrack.Value{
		{httptrack.LocationHeader, "x-header", httptrack.LocationHeader, "x-new-header", nil},
	}

	// Do not put in the expected x-header
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Make the call!
	testCall(t, req, httptrack.Options{}, trackValues, func(w http.ResponseWriter, r *http.Request) {
		_, err := httptrack.NewRequestWithContext(r.Context(), "GET", "/downstream", nil)
		if err != nil {
			// There should be no error, despite there not being any values, it should always work.
			t.Fatal(err)
		}
		newReq, _ := http.NewRequestWithContext(r.Context(), "GET", "/outbound", nil)
		err = httptrack.AddContextData(newReq)
		if err != nil {
			// There should still not be an error, because while there were no headers in the inbound
			// request, the call did route through httptrack.Handler, so it should still continue.
			t.Fatal(err)
		}
	})
}

func TestMissingFunc(t *testing.T) {
	// Return blah if header missing
	missingFunc := func(field string, req http.Request) string {
		fmt.Printf("missingFunc called\n")
		return "blah"
	}
	// Look for x-header
	vals := []httptrack.Value{
		{httptrack.LocationHeader, "x-header", httptrack.LocationHeader, "x-header", missingFunc},
	}
	// A request with no header
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Make the request and check
	testCall(t, req, httptrack.Options{}, vals, func(w http.ResponseWriter, r *http.Request) {
		newReq, _ := httptrack.NewRequestWithContext(r.Context(), "GET", "/", nil)
		if newReq.Header.Get("x-header") != "blah" {
			t.Error("x-header defaut value missing")
		}
	})
}
