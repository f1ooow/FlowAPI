package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchProxyRoutePinsTargetAndDoesNotFallBack(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	// The route must pin locally even when the optional global DNS check is off.
	system_setting.GetFetchSetting().ApplyIPFilterForDomain = false
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	target := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var greeting [3]byte
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			return
		}
		var request [10]byte
		if _, err := io.ReadFull(conn, request[:]); err != nil {
			return
		}
		target <- request[:]
		// Refuse the request: routing must not retry through the direct dialer.
		_, _ = conn.Write([]byte{5, 2, 0, 1, 0, 0, 0, 0, 0, 0})
	}()
	directCalls := 0
	client := newProtectedFetchHTTPClientWithDialer(
		staticSSRFResolver{"media.example": {{IP: net.ParseIP("39.101.67.52")}}},
		func(context.Context, string, string) (net.Conn, error) {
			directCalls++
			return nil, errors.New("unexpected direct connection")
		}, nil,
	)
	client.Timeout = time.Second
	config, err := common.Marshal(map[string]string{"https://media.example": "socks5://" + listener.Addr().String()})
	require.NoError(t, err)
	require.NoError(t, configureFetchProxyRoutes(client, string(config)))
	t.Cleanup(client.CloseIdleConnections)
	resp, err := client.Get("https://media.example:443/input.mp4")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 0, directCalls)
	select {
	case got := <-target:
		assert.Equal(t, []byte{5, 1, 0, 1, 39, 101, 67, 52, 1, 187}, got)
	default:
		t.Fatal("SOCKS proxy did not receive the pinned IPv4 target")
	}
}

func TestFetchProxyRoutesRejectInvalidConfiguration(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{"https://media.example":42}`,
		`{"http://media.example":"socks5://127.0.0.1:1080"}`,
		`{"https://*.example":"socks5://127.0.0.1:1080"}`,
		`{"https://media.example:8443":"socks5://127.0.0.1:1080"}`,
		`{"https://user@media.example":"socks5://127.0.0.1:1080"}`,
		`{"https://media.example/path":"socks5://127.0.0.1:1080"}`,
		`{"https://media.example?token=secret":"socks5://127.0.0.1:1080"}`,
		`{"https://media.example#secret":"socks5://127.0.0.1:1080"}`,
		`{"https://media.example":"http://user@127.0.0.1:1080"}`,
		`{"https://media.example":"socks5://user@127.0.0.1:1080/path"}`,
		`{"https://media.example":"socks5://127.0.0.1:1080","https://MEDIA.EXAMPLE:443/":"socks5://127.0.0.1:1080"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			client := newProtectedFetchHTTPClientWithDialer(nil, nil, nil)
			err := configureFetchProxyRoutes(client, raw)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestFetchProxyRoutesOnlySelectExactHTTPSOrigin(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().ApplyIPFilterForDomain = false
	system_setting.GetFetchSetting().AllowedPorts = []string{"80", "443", "8443"}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	proxied := make(chan struct{}, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			proxied <- struct{}{}
			conn.Close()
		}
	}()
	directCalls := 0
	client := newProtectedFetchHTTPClientWithDialer(
		staticSSRFResolver{
			"media.example": {{IP: net.ParseIP("39.101.67.52")}},
			"MEDIA.EXAMPLE": {{IP: net.ParseIP("39.101.67.52")}},
		},
		func(context.Context, string, string) (net.Conn, error) {
			directCalls++
			return nil, errors.New("direct request")
		}, nil,
	)
	client.Timeout = time.Second
	config, err := common.Marshal(map[string]string{"https://media.example": "socks5://" + listener.Addr().String()})
	require.NoError(t, err)
	require.NoError(t, configureFetchProxyRoutes(client, string(config)))
	t.Cleanup(client.CloseIdleConnections)
	for _, target := range []string{"https://media.example/input", "https://media.example:443/input", "https://MEDIA.EXAMPLE/input", "https://media.example:0443/input"} {
		_, err := client.Get(target)
		require.Error(t, err)
		select {
		case <-proxied:
		default:
			t.Fatalf("expected proxy connection for %s", target)
		}
	}
	assert.Zero(t, directCalls)
	_, err = client.Get("https://user@media.example/input")
	require.ErrorContains(t, err, "credentials")
	assert.Zero(t, directCalls, "userinfo must not bypass an origin's configured route")
	for _, target := range []string{"http://media.example/input", "https://media.example:8443/input", "https://sub.media.example/input", "https://media.example.evil/input"} {
		_, err := client.Get(target)
		require.ErrorContains(t, err, "direct request")
	}
	assert.Equal(t, 4, directCalls)
	assert.Empty(t, proxied)
}

func TestFetchProxyRouteRejectsUserinfoRegardlessOfGlobalProtection(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			configureSSRFTestFetchSetting(t)
			system_setting.GetFetchSetting().EnableSSRFProtection = enabled
			original := httpClient
			t.Cleanup(func() { httpClient = original })
			calls := 0
			httpClient = &http.Client{Transport: fetchRouteTestTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected generic request")
			})}
			client := newProtectedFetchHTTPClientWithDialer(nil, func(context.Context, string, string) (net.Conn, error) {
				calls++
				return nil, errors.New("unexpected direct request")
			}, nil)
			require.NoError(t, configureFetchProxyRoutes(client, `{"https://media.example":"socks5://127.0.0.1:1080"}`))
			_, err := client.Get("https://user@media.example/input")
			require.ErrorContains(t, err, "credentials")
			assert.Zero(t, calls)
		})
	}
}
