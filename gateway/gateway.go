package gateway

import "net/http"

type Router interface {
	Register(mux *http.ServeMux)
}

type Server struct {
	Addr string
	Mux  *http.ServeMux
}
