# zafnz/go-httptrack

The httptrack library is for http api servers that need to pass some tracking value(s) with all the http call it itself
makes on behalf of the inbound request -- think microservices environment where an inbound call from a browser can result
in multiple internal calls.

For example; if all incoming requests have a http header called "x-tracking-id", and the service needs to make other calls
to satisfy the request (eg a microservices environment) and each of the calls should also include that tracking id, then
this library makes it a bit simplier and easier to not forget to pass it.

## Usage
Install the httptrack.Handler as middleware, specifying what values to pass through:
```golang
// handler is a http.Handler
handler := httptrack.Handler(r, httptrack.Options{}, []httptrack.Value{
    {
        InboundLocation: httptrack.LocationHeader,
        InboundName: "x-tracking-id", 
        OutboundLocation: httptrack.LocationHeader, 
        OutboundName: "x-tracking-id",
        MissingFunc: nil
    }, 
    // Or, more condensely, and demonstrating changing type:
    { httptrack.LocationCookie, "session-id", httptrack.LocationHeader, "x-client-session", nil},
})

```
Then later in your code, as a response to an inbound request, because you've been passing ctx to
all of your functions, you need to make a http GET, instead of calling `http.NewRequestWithContext`
you call `httptrack.NewRequestWithContext` as a drop in replacement:

```golang
req := httptrack.NewRequestWithContext(ctx, "GET", "http://microservice1.example.com", nil)
http.Do(req) 

// or simply use the handy:
httptrack.Get(ctx, "http://microservice1.example.com")
```
You can also just use all of the normal net/http functions, and use `httptrack.AddContextData(req http.Request)` 
to set the values of the http.Request.

## Example
```golang
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
        // ... Stuff happens...
        // This Get request will go out with the x-tracking-id and x-client-session-id header
        // values set. (assuming the correct inbound values came in, see MissingFunc example)
        httptrack.Get(ctx, "http://microservice1.example.com/serviceCall")
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

```golang
func missingFunc(name string, r http.Request) string {
    // Here we have access to the incoming request, and need to generate the x-tracking-id header
    // value. If we return empty string no x-tracking-id will be set.
    return "blah"
}

func ExampleHandler_missingfunc() {
    mux := http.NewServeMux()

    // The inbound HTTP header x-tracking-id should be copied to all outbound calls as an HTTP header
    // with the same name. If the inbound call does not have an x-tracking-id header, then missingFuncHandler()
    // is called, supplying "x-tracking-id" and a copy of the inbound http.Request
    handler := httptrack.Handler(mux, httptrack.Options{}, []httptrack.Value{
        {httptrack.LocationHeader, "x-tracking-id", httptrack.LocationHeader, "x-tracking-id", missingFunc},
    })

    mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        // If the inbound request does not have x-tracking-id set, then this outbound call will
        // have the x-tracking-id set to "blah"
        httptrack.Get(ctx, "http://microservice1.example.com/serviceCall")
    })

    http.ListenAndServe("127.0.0.1:3000", handler)
}
```

Another usecase for MissingFunc is it can be combined with a blank inbound name to provide x-forwarded-for:
```golang 
func getRequestIp(name string, r http.Request) string {
    // Here we have access to the incoming request, and need to generate the x-tracking-id header
    // value. If we return empty string no x-tracking-id will be set.
    idx := strings.LastIndex(r.RemoteAddr, ":")
    if idx == -1 {
        return r.RemoteAddr
    }
    return r.RemoteAddr[0:idx]
}

func main() {
    // ... 
    // There is never a header with no name, so the MissingFunc will always be called. So the client
    // IP address is added to outbound requests with the "x-forwarded-for" header.
    handler := httptrack.Handler(mux, httptrack.Options{}, []httptrack.Value{
        {httptrack.LocationHeader, "", httptrack.LocationHeader, "x-forwarded-for", getRequestIp},
    })
}
```

## Client libraries
Inside your company you may use multiple client libraries to make those outbound calls for your API
server. Fortunately by design the client libraries can be switched over to using httptrack for 
it's outbound requests with no impact or knowledge of callers requirements. Instead of using
`http.NewRequestWithContext(ctx...)` they use the httptrack version, which is functionaly identical.

When an HTTP service uses that library and uses httptrack, the outbound header fields are set 
automatically, simply because the client library already uses httptrack. There is very little cost
or reason not to convert your client libraries to use httptrack ahead of all the http api services.