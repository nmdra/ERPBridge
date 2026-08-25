package security

import (
	"net/url"
	"testing"
)

const transportTestHTTPEndpoint = "http://erp.example.test/orders"

func TestValidateOutboundTransport(t *testing.T) {
	tests := []struct {
		name              string
		endpoint          string
		credentialPresent bool
		allowedHosts      string
		wantAllowedHTTP   bool
		wantErr           bool
	}{
		{
			name:              "accepts credentialed HTTPS",
			endpoint:          "https://ERP.Example.Test/orders",
			credentialPresent: true,
		},
		{
			name:              "rejects credentialed HTTP",
			endpoint:          transportTestHTTPEndpoint,
			credentialPresent: true,
			wantErr:           true,
		},
		{
			name:              "allows exact normalized HTTP host port",
			endpoint:          "http://ERP.Example.Test/orders",
			credentialPresent: true,
			allowedHosts:      "erp.example.test:80",
			wantAllowedHTTP:   true,
		},
		{
			name:              "rejects nonmatching HTTP host port",
			endpoint:          transportTestHTTPEndpoint,
			credentialPresent: true,
			allowedHosts:      "other.example.test:80",
			wantErr:           true,
		},
		{
			name:              "allows unauthenticated HTTP",
			endpoint:          transportTestHTTPEndpoint,
			credentialPresent: false,
		},
		{
			name:              "rejects endpoint userinfo",
			endpoint:          "http://user:secret@erp.example.test/orders", // #nosec G101 -- regression test for rejected URL userinfo.
			credentialPresent: false,
			wantErr:           true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(InsecureAuthAllowedHostsEnv, test.allowedHosts)
			endpoint, err := url.Parse(test.endpoint)
			if err != nil {
				t.Fatal(err)
			}

			_, allowedHTTP, err := ValidateOutboundTransport(endpoint, test.credentialPresent)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected transport validation error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if allowedHTTP != test.wantAllowedHTTP {
				t.Fatalf("allowed insecure HTTP = %t, want %t", allowedHTTP, test.wantAllowedHTTP)
			}
		})
	}
}

func TestValidateOutboundTransportNormalizesDefaultPorts(t *testing.T) {
	t.Setenv(InsecureAuthAllowedHostsEnv, "[::1]:80")
	endpoint, err := url.Parse("http://[::1]/orders")
	if err != nil {
		t.Fatal(err)
	}

	hostPort, allowedHTTP, err := ValidateOutboundTransport(endpoint, true)
	if err != nil {
		t.Fatal(err)
	}
	if hostPort != "[::1]:80" {
		t.Fatalf("host port = %q, want [::1]:80", hostPort)
	}
	if !allowedHTTP {
		t.Fatal("expected allowed insecure HTTP")
	}
}
