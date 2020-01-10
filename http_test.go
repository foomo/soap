package soap

import (
	"net/http"
)

var _ http.RoundTripper = (*RoundTrip)(nil)

type RoundTrip func(r *http.Request) (*http.Response, error)

func (rt RoundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}
