package hetzner

import "fmt"

type ProviderError struct {
	Code    string
	Message string
}

func (e ProviderError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

func invalidRequestError(message string) error {
	return ProviderError{Code: "invalid_request", Message: message}
}

func notFoundError(message string) error {
	return ProviderError{Code: "not_found", Message: message}
}
