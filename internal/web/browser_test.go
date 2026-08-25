package web

import "testing"

func TestServerOpensInjectedBrowser(t *testing.T) {
	var opened string
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		OpenBrowser: func(url string) error {
			opened = url
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	if err := server.OpenBrowser(); err != nil {
		t.Fatal(err)
	}
	if opened != server.CapabilityURL() {
		t.Fatalf("opened URL = %q, want %q", opened, server.CapabilityURL())
	}
}
