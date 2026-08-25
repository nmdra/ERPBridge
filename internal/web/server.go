// Package web hosts the local, read-only ERPBridge Console.
package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// CapabilityHeader carries the per-launch local console capability.
	CapabilityHeader = "X-ERPBridge-Console-Capability"
	healthPath       = "/healthz"
	apiPrefix        = "/api/"
	loopbackAddress  = "127.0.0.1"
	defaultListen    = loopbackAddress + ":0"
)

// BrowserOpener opens a URL in the user's default browser.
type BrowserOpener func(string) error

// Options configures a local console server.
type Options struct {
	ListenAddress string
	Handler       http.Handler
	HTTPServer    *http.Server
	Listener      net.Listener
	OpenBrowser   BrowserOpener
	Capability    string
	URLBuilder    func(net.Addr) string
	Logger        *slog.Logger
}

// Server hosts the console and owns its listener lifetime.
type Server struct {
	httpServer    *http.Server
	listener      net.Listener
	handler       http.Handler
	browserOpener BrowserOpener
	capability    string
	baseURL       string
	host          string
	origin        string
}

// NewServer creates a console server on a literal loopback address.
func NewServer(options Options) (*Server, error) {
	address := options.ListenAddress
	if address == "" {
		address = defaultListen
	}

	listener := options.Listener
	if listener == nil {
		if err := validateLoopbackAddress(address); err != nil {
			return nil, err
		}
		var err error
		listener, err = net.Listen("tcp", address)
		if err != nil {
			return nil, fmt.Errorf("listen on %s: %w", address, err)
		}
	}
	if err := validateLoopbackListener(listener); err != nil {
		_ = listener.Close()
		return nil, err
	}

	capability := options.Capability
	if capability == "" {
		var err error
		capability, err = generateCapability()
		if err != nil {
			_ = listener.Close()
			return nil, err
		}
	}
	if len(capability) < 32 {
		_ = listener.Close()
		return nil, errors.New("console capability is too short")
	}

	baseURL := defaultURL(listener.Addr())
	if options.URLBuilder != nil {
		baseURL = strings.TrimRight(options.URLBuilder(listener.Addr()), "/")
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || parsedURL.Path != "" {
		_ = listener.Close()
		return nil, fmt.Errorf("invalid console URL %q", baseURL)
	}

	server := &Server{
		listener:      listener,
		browserOpener: options.OpenBrowser,
		capability:    capability,
		baseURL:       baseURL,
		host:          parsedURL.Host,
		origin:        parsedURL.Scheme + "://" + parsedURL.Host,
	}
	if server.browserOpener == nil {
		server.browserOpener = defaultBrowserOpener()
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	server.handler = server.secureHandler(options.Handler, options.Logger)
	server.httpServer = options.HTTPServer
	if server.httpServer == nil {
		server.httpServer = &http.Server{ReadHeaderTimeout: 5 * time.Second}
	}
	server.httpServer.Handler = server.handler
	return server, nil
}

// Serve runs the HTTP server until the listener closes.
func (s *Server) Serve() error {
	err := s.httpServer.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// Run opens the browser and serves until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("console run context is nil")
	}
	if err := s.OpenBrowser(); err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- s.Serve() }()
	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownContext); err != nil {
			return err
		}
		return nil
	}
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("console shutdown context is nil")
	}
	return s.httpServer.Shutdown(ctx)
}

// Close closes the server listener.
func (s *Server) Close() error {
	if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

// OpenBrowser opens the capability URL with the configured browser opener.
func (s *Server) OpenBrowser() error {
	if s.browserOpener == nil {
		return nil
	}
	return s.browserOpener(s.CapabilityURL())
}

// Handler returns the fully protected HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// URL returns the console base URL without the capability.
func (s *Server) URL() string { return s.baseURL }

// CapabilityURL returns the one-time capability URL for the browser.
func (s *Server) CapabilityURL() string { return s.baseURL + "#cap=" + s.capability }

// Capability returns the per-launch capability.
func (s *Server) Capability() string { return s.capability }

// Host returns the exact Host header accepted by the server.
func (s *Server) Host() string { return s.host }

// Port returns the listener port as a string.
func (s *Server) Port() string {
	_, port, err := net.SplitHostPort(s.host)
	if err != nil {
		return ""
	}
	return port
}

// Origin returns the exact Origin accepted by the server.
func (s *Server) Origin() string { return s.origin }

func validateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid loopback listen address %q: %w", address, err)
	}
	if host != loopbackAddress && host != ipv6Loopback {
		return fmt.Errorf("listen address %q is not a literal loopback address", address)
	}
	return nil
}

func validateLoopbackListener(listener net.Listener) error {
	if listener == nil || listener.Addr() == nil {
		return errors.New("listener is nil")
	}
	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return fmt.Errorf("invalid listener address: %w", err)
	}
	if host != loopbackAddress && host != ipv6Loopback {
		return fmt.Errorf("listener address %q is not a literal loopback address", listener.Addr())
	}
	return nil
}

func defaultURL(address net.Addr) string {
	host, port, err := net.SplitHostPort(address.String())
	if err != nil {
		return "http://" + address.String()
	}
	return "http://" + net.JoinHostPort(host, port)
}

func generateCapability() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate console capability: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
