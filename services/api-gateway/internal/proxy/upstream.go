package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func New(target string) (http.Handler, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("proxy: invalid target %q: %w", target, err)
	}
	return httputil.NewSingleHostReverseProxy(u), nil
}
