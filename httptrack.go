// The httptrack library is for http api servers that need to pass some tracking value(s) with all the http call it itself
// makes on behalf of the inbound request -- think microservices environment where an inbound call from a browser can result
// in multiple internal calls.
//
// For example; if all incoming requests have a http header called "x-tracking-id", and the service needs to make other calls
// to satisfy the request (eg a microservices environment) and each of the calls should also include that tracking id, then
// this library makes it a bit simplier and easier to not forget to pass it.
//
// Usage:
//
// Install the httptrack.Handler as middleware, specifying what values to pass through:
//  handler := httptrack.Handler(r, httptrack.Options{}, []httptrack.Value{
//      // Inbound location, name, outbound location, name
//	    {LocationHeader, "x-tracking-id", LocationHeader, "x-tracking-id"},
//  })
// Then later in your code, as a response to an inbound request, because you've been passing ctx to
// all of your functions, you need to make a http GET, instead of calling `http.NewRequestWithContext`
// you call `httptrack.NewRequestWithContext` as a drop in replacement:
//
//  req := httptrack.NewRequestWithContext(ctx, "GET", "http://microservice1.example.com", nil)
//  httptrack.Do(req) // The x-tracking-id header value from the inbound request is set for this http request too.
//  // or simply use the handy:
//  httptrack.Get(ctx, "http://microservice1.example.com")
//
// You can use all of the normal net/http functions, and just use `httptrack.AddContextData(req http.Request)` to set
// the values for you.
//
package httptrack

import (
	"context"
	"errors"
	"io"
	"net/http"
)

// The location where the data is.
const (
	LocationHeader     = 1
	LocationCookie     = 2
	LocationQueryParam = 3
)

// When a value is found in `InboundLocation` named `InboundName`, set
// `OutboundLocation`.`OutboundName` to that value for all outbound requests. Commonly you'd see
// something like:
//
//  httptrack.Value{LocationCookie, "session-id", LocationHeader, "x-client-session-id", nil}
//
// or in a microservices environment:
//  httptrack.Value{LocationHeader, "x-track", LocationHeader, "x-track", nil}
//
type Value struct {
	InboundLocation  int
	InboundName      string
	OutboundLocation int
	OutboundName     string
	// An optional function that will be called if the inbound request does not have the specified
	// field named `InboundName` in `InboundLocation`. The function is given the `OutboundName` and
	// a copy of the inbound `http.Request`.
	// NOTE: This function is called only once per inbound request, so you can use it to generate a
	// new random tracking ID and all outbound calls for that request will share the same tracking ID.
	MissingFunc func(string, http.Request) string
}

// There are no Options currently available to set.
type Options struct {
}

// Internal, this is how we pass through the name+value we need to set on the outbound request
type ctxValue struct {
	location int
	name     string
	value    string
}

type ctxDataName string
type ctxData struct {
	values []ctxValue
}

// Add this handler as middleware for your http server.
//
// eg:
//  mux := http.NewServeMux()
//  handler := httptrack.Handler(mux, httptrack.Options{})
//  http.ListenAndServe("127.0.0.1:3000", handler)
func Handler(next http.Handler, options Options, values []Value) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header
		var ctxValues []ctxValue
		for _, v := range values {
			var outboundValue string
			switch v.InboundLocation {
			case LocationHeader:
				if val := h.Get(v.InboundName); val != "" {
					outboundValue = val
				}
			case LocationCookie:
				cookie, err := r.Cookie(v.InboundName)
				if err == nil && cookie.Value != "" {
					outboundValue = cookie.Value
				}
			case LocationQueryParam:
				if r.URL != nil {
					val := r.URL.Query().Get(v.InboundName)
					if val != "" {
						outboundValue = val
					}
				}
			}
			// If value is missing and there is a missing func, then call it
			if outboundValue == "" && v.MissingFunc != nil {
				outboundValue = v.MissingFunc(v.OutboundName, *r)
			}
			if outboundValue != "" {
				ctxValues = append(ctxValues, ctxValue{
					location: v.OutboundLocation,
					name:     v.OutboundName,
					value:    outboundValue,
				})

			}
		}
		r = r.WithContext(context.WithValue(r.Context(), ctxDataName("httptrack"), ctxData{values: ctxValues}))
		next.ServeHTTP(w, r)
	})
}

var ErrMissingContext = errors.New("no httptrack data found in context. Probably ctx wasn't set in httptrack.Handler or has become context.Background() at some point")

// Create a new http.Request, but setting the appropriate outbound header/query/cookie (as specified in the original
// httptrack.Handler functions value parameter). These values are passed around in the `ctx context.Context`, so you
// should ensure that you pass ctx around when handling inbound requests (this is best practice anyhow).
//
// If `ctx` is context.Background(), or some other context that doesn't contain the httptrack then this function
// will return normally without taking any additional actions.
//
// Note: This function is actually just a wrapper around `http.NewRequestWithContext()` and `httptrack.AddContextData`
func NewRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	err = AddContextData(req)
	if err != nil {
		// It's ok if the context is missing
		if !errors.Is(err, ErrMissingContext) {
			return nil, err
		}
	}
	return req, nil
}

// While this library provides some nice wrapper functions, they all wrap around this. This function adds the
// appropriate headers/cookies/params that were specified in the handler setup. If there is no http track
// data in the context (eg this did not come from an inbound http event via the httptrack.Handler), then this
// function returns httptrack.ErrMissingContext. You probably should just use httptrack.NewRequestWithContext()
// instead -- unless you can't create the new request yourself, or you want to know if context is lost.
//
// NOTE: If you don't mind if there is no tracking data set, then you should ignore the ErrMissingContext returned
// via:
//  if errors.Is(err, httptrack.ErrMissingContext)
func AddContextData(req *http.Request) error {
	data, ok := req.Context().Value(ctxDataName("httptrack")).(ctxData)
	if !ok {
		return ErrMissingContext
	}
	for _, v := range data.values {
		switch v.location {
		case LocationHeader:
			req.Header.Add(v.name, v.value)
		case LocationCookie:
			req.AddCookie(&http.Cookie{
				Name:  v.name,
				Value: v.value,
			})
		case LocationQueryParam:
			if req.URL == nil {
				return errors.New("request has no URL set")
			}
			q := req.URL.Query()
			q.Add(v.name, v.value)
			req.URL.RawQuery = q.Encode()
		}
	}
	return nil
}

// A wrapper function around httptrack.NewRequestWithContext(ctx, "GET"...) and http.Client.Do()
func Get(ctx context.Context, url string) (resp *http.Response, err error) {
	req, err := NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	resp, err = client.Do(req)
	return resp, err
}

// A wrapper function around httptrack.NewRequestWithContext(ctx, "POST"...) and http.Client.Do()
func Post(ctx context.Context, url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", contentType)
	client := &http.Client{}
	resp, err = client.Do(req)
	return resp, err
}
