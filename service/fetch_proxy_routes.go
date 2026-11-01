package service

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Routes are a bounded startup configuration, never populated from request URLs.
type fetchProxyRouter struct {
	base   *ssrfProtectedRoundTripper
	routes map[string]*http.Transport
}

func fetchProxyOrigin(u *url.URL) string {
	if u.Scheme != "https" || u.Hostname() == "" {
		return ""
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port != 443 {
			return ""
		}
	}
	return strings.ToLower(u.Hostname())
}

func configureFetchProxyRoutes(client *http.Client, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var routes map[string]string
	if err := common.UnmarshalJsonStr(raw, &routes); err != nil || routes == nil {
		return fmt.Errorf("FETCH_PROXY_ROUTES must be a JSON object of HTTPS origins to SOCKS5 URLs")
	}
	if len(routes) == 0 {
		return nil
	}
	router := &fetchProxyRouter{
		base:   client.Transport.(*ssrfProtectedRoundTripper),
		routes: make(map[string]*http.Transport, len(routes)),
	}
	for origin, rawProxy := range routes {
		u, err := url.Parse(origin)
		if err != nil || fetchProxyOrigin(u) == "" || u.User != nil || strings.ContainsAny(u.Host, "*%") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return fmt.Errorf("FETCH_PROXY_ROUTES requires exact HTTPS origins on port 443 without paths or credentials")
		}
		key := fetchProxyOrigin(u)
		if _, exists := router.routes[key]; exists {
			return fmt.Errorf("FETCH_PROXY_ROUTES contains duplicate normalized origins")
		}
		proxyURL, err := common.ParseProxyURLStrict(rawProxy)
		if err != nil || proxyURL == nil || (proxyURL.Scheme != "socks5" && proxyURL.Scheme != "socks5h") {
			return fmt.Errorf("FETCH_PROXY_ROUTES requires valid SOCKS5 proxy URLs")
		}
		transport := router.base.newTransport(nil)
		// Keep TLS verification against the original origin, not the proxy or IP.
		transport.TLSClientConfig = nil
		if err := configureProxyTransport(transport, proxyURL); err != nil {
			return fmt.Errorf("FETCH_PROXY_ROUTES could not initialize SOCKS5 transport")
		}
		dialer := &protectedFetchDialer{
			resolver:      router.base.resolver,
			dialContext:   transport.DialContext,
			getProtection: router.protection,
		}
		transport.DialContext = dialer.DialContext
		router.routes[key] = transport
	}
	client.Transport = router
	return nil
}

func (r *fetchProxyRouter) protection() (*common.SSRFProtection, bool, error) {
	p, enabled, err := r.base.getProtection()
	if err != nil {
		return nil, true, err
	}
	policy := common.SSRFProtection{AllowedPorts: []int{443}}
	if enabled {
		policy = *p
	}
	// SOCKS must receive a locally validated IP, even if global checks are off.
	policy.AllowPrivateIp = false
	policy.ApplyIPFilterForDomain = true
	return &policy, true, nil
}

func (r *fetchProxyRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("invalid request")
	}
	if transport := r.routes[fetchProxyOrigin(req.URL)]; transport != nil {
		if req.URL.User != nil {
			return nil, fmt.Errorf("routed media URLs must not contain credentials")
		}
		if err := ValidateSSRFProtectedFetchURL(req.URL.String()); err != nil {
			return nil, err
		}
		return transport.RoundTrip(req)
	}
	_, enabled, err := r.base.getProtection()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return GetHttpClient().Transport.RoundTrip(req)
	}
	return r.base.RoundTrip(req)
}

func (r *fetchProxyRouter) CloseIdleConnections() {
	r.base.CloseIdleConnections()
	for _, transport := range r.routes {
		transport.CloseIdleConnections()
	}
}
