package httputil

import (
	"net/http"
	"net/http/httputil"
)

type debugRoundTripper struct {
	printer   func(string)
	transport http.RoundTripper
}

// NewDebugRoundTripper logs request and response including full JSON request and response bodies
func NewDebugRoundTripper(printer func(string), rt http.RoundTripper) http.RoundTripper {
	return &debugRoundTripper{
		printer:   printer,
		transport: rt,
	}
}

// NewDebugHTTPClient returns a new http.Client that logs request and response including full JSON request and response bodies
func NewDebugHTTPClient(printer func(string)) *http.Client {
	client := new(http.Client)
	*client = *http.DefaultClient
	client.Transport = NewDebugRoundTripper(printer, http.DefaultTransport)
	return client
}

// RoundTrip logs request and response including full JSON request and response bodies
func (rt *debugRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	data, err := httputil.DumpRequest(req, true)
	if err != nil {
		return nil, err
	}
	rt.printer(string(data))

	resp, err := rt.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	data, err = httputil.DumpResponse(resp, true)
	if err != nil {
		return nil, err
	}
	rt.printer(string(data))

	return resp, nil
}
