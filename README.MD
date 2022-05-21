# zafnz/go-httptrack

The httptrack library is for http api servers that need to pass some tracking value(s) with all the http call it itself
makes on behalf of the inbound request -- think microservices environment where an inbound call from a browser can result
in multiple internal calls.

For example; if all incoming requests have a http header called "x-tracking-id", and the service needs to make other calls
to satisfy the request (eg a microservices environment) and each of the calls should also include that tracking id, then
this library makes it a bit simplier and easier to not forget to pass it.

## Usage
Install the httptrack.Handler as middleware, specifying what values to pass through:
```
 handler := httptrack.Handler(r, httptrack.Options{}, []httptrack.Value{
       // Inbound location, inbound name, outbound location, outbound name, missingfunc
	    {LocationHeader, "x-tracking-id", LocationHeader, "x-tracking-id", nil},
 })
```
Then later in your code, as a response to an inbound request, because you've been passing ctx to
all of your functions, you need to make a http GET, instead of calling `http.NewRequestWithContext`
you call `httptrack.NewRequestWithContext` as a drop in replacement:
```
 req := httptrack.NewRequestWithContext(ctx, "GET", "http://microservice1.example.com", nil)
 httptrack.Do(req) // The x-tracking-id header value from the inbound request is set for this http request too.
 // or simply use the handy:
 httptrack.Get(ctx, "http://microservice1.example.com")
```
You can use all of the normal net/http functions, and just use `httptrack.AddContextData(req http.Request)` to set
the values for you.

## Example
```
func ExampleHandler() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// This Get request will go out with the x-tracking-id and x-client-session-id header
		// values set. (assuming the correct inbound values came in, see MissingFunc example)
		httptrack.Get(ctx, "http://microservice1.example.com/serviceCall")
	})
	handler := httptrack.Handler(mux, httptrack.Options{}, []httptrack.Value{
		{httptrack.LocationHeader, "x-tracking-id", httptrack.LocationHeader, "x-tracking-id", nil},
		{httptrack.LocationCookie, "session-id", httptrack.LocationHeader, "x-client-session-id", nil},
	})
	http.ListenAndServe("127.0.0.1:3000", handler)
}
```

## Locations
The handler can look for values in the HTTP header (`LocationHeader`), Cookie (`LocationCookie`), and in a query
parameter (`LocationQueryParam`). Then any outbound calls can have those values set in any of those locations as 
well (allowing you to take a value that is in the clients cookies (eg "session-id") and set it as a header (eg
"x-client-session-id") for all outbound requests).

## Missing values
The final parameter in httptrack.Value is `MissingFunc`. If an inbound request does not have the specified
field set, then `MissingFunc(field string, r http.Request)` will be called (if not nil), supplied with the
outbound field name and a copy of the http request. This function will only be called once per inbound request
so it can be safely used to generate a session ID or similar that will be the same for all outbound calls.

```
func ExampleHandler_missingfunc() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// If the inbound request does not have x-tracking-id set, then this outbound call will
		// have the x-tracking-id set to "blah"
		httptrack.Get(ctx, "http://microservice1.example.com/serviceCall")
	})

	missingFuncHandler := func(name string, r http.Request) string {
		// Here we have access to the incoming request, and need to generate the x-tracking-id header
		// value. If we return empty string no x-tracking-id will be set.
		return "blah"
	}

	handler := httptrack.Handler(mux, httptrack.Options{}, []httptrack.Value{
		{httptrack.LocationHeader, "x-tracking-id", httptrack.LocationHeader, "x-tracking-id", missingFuncHandler},
	})
	http.ListenAndServe("127.0.0.1:3000", handler)
}
```
