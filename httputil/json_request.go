package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// JSONRequest is a request that is sent as JSON and expects a JSON response or JSON error
type JSONRequest struct {
	// Method is the HTTP method to use
	Method string
	// URL is the URL to send the request to
	URL string
	// Body is the request body, may be nil
	Body any
	// Headers are the headers to send with the request
	Headers map[string]string
}

func (req *JSONRequest) Valid() error {
	if req.Method == "" {
		return fmt.Errorf("method is required")
	}
	if req.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

// JSONResponse is a response that is sent as JSON and expects a JSON response or JSON error
type JSONResponse[Response any, Error error] struct {
	// HTTPStatusCode is the HTTP status code of the response
	HTTPStatusCode int
	// HTTPStatusMessage is the HTTP status message of the response
	HTTPStatusMessage string
	// Response is the response body if the request was successful
	Response *Response
	// err is the error response if the request was not successful
	err *Error
}

// Error returns the error if the request was not successful, or nil if the request was successful
func (resp *JSONResponse[Response, Error]) Error() error {
	if resp.err != nil {
		return fmt.Errorf("%s: %w", resp.HTTPStatusMessage, *resp.err)
	}
	if resp.Response == nil {
		return fmt.Errorf("%s: no response", resp.HTTPStatusMessage)
	}
	return nil
}

// DoJSONRequest sends request to url and parses the response as JSON, or returns an error if the request failed
func DoJSONRequest[Response any, Error error](ctx context.Context, client *http.Client, request JSONRequest) (*JSONResponse[Response, Error], error) {
	if err := request.Valid(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	var body io.Reader
	if request.Body != nil {
		b, err := json.Marshal(request.Body)
		if err != nil {
			return nil, fmt.Errorf("error marshalling JSON: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, request.Method, request.URL, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range request.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResponse Error
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("unexpected status code: %d. Also, error response was not JSON: %w", resp.StatusCode, err)
		}
		return &JSONResponse[Response, Error]{err: &errorResponse, HTTPStatusCode: resp.StatusCode, HTTPStatusMessage: resp.Status}, nil
	}

	var response Response
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("error decoding JSON: %w", err)
	}
	return &JSONResponse[Response, Error]{Response: &response, HTTPStatusCode: resp.StatusCode}, nil
}
