A subrouter with several routes returns the wrong status when the method doesn't match. With

```go
sub := router.PathPrefix("/v1").Subrouter()
sub.HandleFunc("/api", h).Methods(http.MethodGet)
sub.HandleFunc("/api/{id}", h).Methods(http.MethodGet)
```

a `PUT /v1/api` gets a 404, but the path matches a route and only the method is wrong, so it should get 405 Method Not Allowed, as it does when the subrouter has one route. Find the cause and fix it. `go test ./...` must pass.
