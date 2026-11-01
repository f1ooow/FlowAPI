package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fetchRouteTestTransport func(*http.Request) (*http.Response, error)

func (f fetchRouteTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// The SOCKS destination is recorded but never used to dial: this fixture only
// forwards to the explicitly supplied local TLS server, or stalls for cancellation.
func newFetchRouteSOCKSFixture(t *testing.T, localTarget string) (string, <-chan []byte) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	targets := make(chan []byte, 8)
	acceptDone := make(chan struct{})
	var connections sync.Map
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Store(conn, struct{}{})
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer connections.Delete(conn)
				defer conn.Close()
				var greeting [3]byte
				if _, err := io.ReadFull(conn, greeting[:]); err != nil {
					return
				}
				if greeting != [3]byte{5, 1, 0} {
					return
				}
				if _, err := conn.Write([]byte{5, 0}); err != nil {
					return
				}
				var target [10]byte
				if _, err := io.ReadFull(conn, target[:]); err != nil {
					return
				}
				targets <- append([]byte(nil), target[:]...)
				if localTarget == "" {
					_, _ = io.Copy(io.Discard, conn)
					return
				}
				upstream, err := net.Dial("tcp", localTarget)
				if err != nil {
					return
				}
				defer upstream.Close()
				if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
					return
				}
				uploadDone := make(chan struct{})
				go func() {
					_, _ = io.Copy(upstream, conn)
					_ = upstream.Close()
					close(uploadDone)
				}()
				_, _ = io.Copy(conn, upstream)
				_ = conn.Close()
				<-uploadDone
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-acceptDone
		connections.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
		workers.Wait()
	})
	return "socks5://" + listener.Addr().String(), targets
}

func TestFetchProxyRouteIntegrationTLSAndRedirectSelection(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	originalClient, originalProtected := httpClient, ssrfProtectedHTTPClient
	originalInsecure := common.TLSInsecureSkipVerify
	t.Cleanup(func() {
		httpClient, ssrfProtectedHTTPClient = originalClient, originalProtected
		common.TLSInsecureSkipVerify = originalInsecure
	})
	common.TLSInsecureSkipVerify = true
	var genericCalls atomic.Int32
	httpClient = &http.Client{Transport: fetchRouteTestTransport(func(req *http.Request) (*http.Response, error) {
		genericCalls.Add(1)
		assert.Equal(t, "outside.example", req.URL.Hostname())
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("generic bytes")), Request: req}, nil
	})}
	type receivedRequest struct{ path, host, sni string }
	received := make(chan receivedRequest, 8)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- receivedRequest{req.URL.Path, req.Host, req.TLS.ServerName}
		switch req.URL.Path {
		case "/same":
			http.Redirect(w, req, "/bytes", http.StatusFound)
		case "/cross":
			http.Redirect(w, req, "https://outside.example/bytes", http.StatusFound)
		default:
			_, _ = w.Write([]byte("verified TLS bytes"))
		}
	}))
	t.Cleanup(server.Close)
	require.NotEmpty(t, server.Certificate().DNSNames)
	host := server.Certificate().DNSNames[0]
	proxyURL, targets := newFetchRouteSOCKSFixture(t, server.Listener.Addr().String())
	var directCalls atomic.Int32
	client := newProtectedFetchHTTPClientWithDialer(staticSSRFResolver{
		host:            {{IP: net.ParseIP("39.101.67.52")}},
		"wrong.example": {{IP: net.ParseIP("39.101.67.52")}},
	}, func(context.Context, string, string) (net.Conn, error) {
		directCalls.Add(1)
		return nil, errors.New("unexpected direct dial")
	}, nil)
	config, err := common.Marshal(map[string]string{"https://" + host: proxyURL, "https://wrong.example": proxyURL})
	require.NoError(t, err)
	require.NoError(t, configureFetchProxyRoutes(client, string(config)))
	ssrfProtectedHTTPClient = client
	require.Same(t, client, GetSSRFProtectedHTTPClient())
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	for _, transport := range client.Transport.(*fetchProxyRouter).routes {
		// Even a globally insecure provider setting must not disable route TLS checks.
		require.Nil(t, transport.TLSClientConfig)
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	t.Cleanup(client.CloseIdleConnections)
	for _, tc := range []struct{ path, want string }{{"/same", "verified TLS bytes"}, {"/cross", "generic bytes"}} {
		response, err := GetSSRFProtectedHTTPClient().Get("https://" + host + tc.path)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		assert.Equal(t, tc.want, string(body))
	}
	for _, path := range []string{"/same", "/bytes", "/cross"} {
		got := <-received
		assert.Equal(t, receivedRequest{path, host, host}, got)
	}
	assert.Equal(t, int32(1), genericCalls.Load())
	assert.Equal(t, int32(0), directCalls.Load())
	assert.Equal(t, []byte{5, 1, 0, 1, 39, 101, 67, 52, 1, 187}, <-targets)

	response, err := client.Get("https://wrong.example/bytes")
	require.Error(t, err)
	assert.Nil(t, response)
	var hostnameError x509.HostnameError
	assert.ErrorAs(t, err, &hostnameError)
	assert.Equal(t, int32(1), genericCalls.Load(), "TLS failure must not fall back to the generic client")
	assert.Equal(t, int32(0), directCalls.Load())
}

func TestFetchProxyRouteIntegrationRejectsPrivateAndMixedDNS(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, mixed := range []bool{false, true} {
			t.Run(fmt.Sprintf("protection=%t/mixed=%t", enabled, mixed), func(t *testing.T) {
				configureSSRFTestFetchSetting(t)
				setting := system_setting.GetFetchSetting()
				setting.EnableSSRFProtection = enabled
				setting.ApplyIPFilterForDomain = false
				setting.AllowPrivateIp = true
				addresses := []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}
				if mixed {
					addresses = append([]net.IPAddr{{IP: net.ParseIP("39.101.67.52")}}, addresses...)
				}
				client := newProtectedFetchHTTPClientWithDialer(staticSSRFResolver{"media.example": addresses},
					func(context.Context, string, string) (net.Conn, error) {
						t.Error("blocked route must not use direct dialer")
						return nil, errors.New("unexpected direct dial")
					}, nil)
				require.NoError(t, configureFetchProxyRoutes(client, `{"https://media.example":"socks5://127.0.0.1:1"}`))
				t.Cleanup(client.CloseIdleConnections)
				response, err := client.Get("https://media.example/bytes")
				require.Error(t, err)
				assert.Nil(t, response)
				assert.Contains(t, err.Error(), "private IP address not allowed")
			})
		}
	}
}

func TestFetchProxyRouteIntegrationCancellationDoesNotFallBack(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	proxyURL, targets := newFetchRouteSOCKSFixture(t, "")
	var directCalls atomic.Int32
	client := newProtectedFetchHTTPClientWithDialer(staticSSRFResolver{"media.example": {{IP: net.ParseIP("39.101.67.52")}}},
		func(context.Context, string, string) (net.Conn, error) {
			directCalls.Add(1)
			return nil, errors.New("unexpected direct dial")
		}, nil)
	config, err := common.Marshal(map[string]string{"https://media.example": proxyURL})
	require.NoError(t, err)
	require.NoError(t, configureFetchProxyRoutes(client, string(config)))
	t.Cleanup(client.CloseIdleConnections)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://media.example/bytes", nil)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		response, err := client.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-targets:
		cancel()
	case err := <-done:
		t.Fatalf("request ended before SOCKS handshake: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("local SOCKS handshake did not start")
	}
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not interrupt SOCKS handshake")
	}
	assert.Equal(t, int32(0), directCalls.Load())

	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer deadlineCancel()
	deadlineRequest, err := http.NewRequestWithContext(deadlineCtx, http.MethodGet, "https://media.example/bytes", nil)
	require.NoError(t, err)
	response, err := client.Do(deadlineRequest)
	require.Error(t, err)
	assert.Nil(t, response)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(0), directCalls.Load(), "a timed-out route must not try a direct connection")
}
