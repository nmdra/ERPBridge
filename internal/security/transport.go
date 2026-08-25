// Package security contains small, dependency-free security controls.
package security

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// InsecureAuthAllowedHostsEnv contains the comma-separated host:port values
// that may receive credentials over HTTP in development-only deployments.
const (
	InsecureAuthAllowedHostsEnv = "INSECURE_AUTH_ALLOWED_HOSTS"
	transportSchemeHTTPS        = "https"
	transportSchemeHTTP         = "http"
)

var errCredentialedHTTPNotAllowed = errors.New("credentialed outbound HTTP endpoint is not allowed")

// ValidateOutboundTransport validates that an outbound endpoint may carry a
// credential. It returns the endpoint's normalized host:port and whether an
// explicitly allowed insecure HTTP exception was used. Unauthenticated calls
// retain their existing transport behavior.
func ValidateOutboundTransport(endpoint *url.URL, credentialPresent bool) (string, bool, error) {
	if endpoint == nil || endpoint.User != nil {
		return "", false, errCredentialedHTTPNotAllowed
	}
	if !credentialPresent {
		return "", false, nil
	}

	scheme := strings.ToLower(endpoint.Scheme)
	if scheme != transportSchemeHTTP && scheme != transportSchemeHTTPS {
		return "", false, errCredentialedHTTPNotAllowed
	}

	hostPort, err := normalizedEndpointHostPort(endpoint, scheme)
	if err != nil {
		return "", false, errCredentialedHTTPNotAllowed
	}
	if scheme == transportSchemeHTTPS {
		return hostPort, false, nil
	}
	if allowedInsecureHost(hostPort) {
		return hostPort, true, nil
	}
	return "", false, errCredentialedHTTPNotAllowed
}

func normalizedEndpointHostPort(endpoint *url.URL, scheme string) (string, error) {
	host := strings.ToLower(endpoint.Hostname())
	if host == "" {
		return "", errors.New("endpoint host is required")
	}

	port := endpoint.Port()
	if port == "" {
		if scheme == transportSchemeHTTPS {
			port = "443"
		} else {
			port = "80"
		}
	}
	if !validPort(port) {
		return "", errors.New("endpoint port is invalid")
	}
	return net.JoinHostPort(host, port), nil
}

func allowedInsecureHost(hostPort string) bool {
	for _, entry := range strings.Split(os.Getenv(InsecureAuthAllowedHostsEnv), ",") {
		normalized, err := normalizeAllowedHostPort(strings.TrimSpace(entry))
		if err == nil && normalized == hostPort {
			return true
		}
	}
	return false
}

func normalizeAllowedHostPort(value string) (string, error) {
	host, port, err := net.SplitHostPort(value)
	if err != nil || host == "" || !validPort(port) {
		return "", errors.New("invalid allowed host")
	}
	return net.JoinHostPort(strings.ToLower(host), port), nil
}

func validPort(port string) bool {
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= 65535
}
