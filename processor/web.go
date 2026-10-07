package processor

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/githoober/monogo"
)

type httpContextKey struct{}

// HTTPRequestData contains request metadata extracted from an HTTP request.
type HTTPRequestData struct {
	URL       string `json:"url,omitempty"`
	IP        string `json:"ip,omitempty"`
	Method    string `json:"http_method,omitempty"`
	Server    string `json:"server,omitempty"`
	Referrer  string `json:"referrer,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

// RequestOption configures HTTP request data extraction.
type RequestOption func(*requestOptions)

type requestOptions struct {
	trustedProxyFunc func(peerIP net.IP) bool
}

// WithTrustedProxies configures trusted proxy IP addresses or CIDR blocks.
// Forwarded-IP headers (X-Forwarded-For, X-Real-IP) are only honored if the
// immediate peer (RemoteAddr) matches a configured trusted proxy.
func WithTrustedProxies(proxies ...string) RequestOption {
	var subnets []*net.IPNet
	var ips []net.IP
	for _, p := range proxies {
		p = strings.TrimSpace(p)
		if strings.Contains(p, "/") {
			if _, cidr, err := net.ParseCIDR(p); err == nil {
				subnets = append(subnets, cidr)
			}
		} else {
			if ip := net.ParseIP(p); ip != nil {
				ips = append(ips, ip)
			}
		}
	}
	return func(o *requestOptions) {
		prev := o.trustedProxyFunc
		o.trustedProxyFunc = func(peerIP net.IP) bool {
			if peerIP == nil {
				return false
			}
			if prev != nil && prev(peerIP) {
				return true
			}
			for _, ip := range ips {
				if ip.Equal(peerIP) {
					return true
				}
			}
			for _, subnet := range subnets {
				if subnet.Contains(peerIP) {
					return true
				}
			}
			return false
		}
	}
}

// WithTrustedProxyFunc configures a custom predicate function to determine if a peer IP is a trusted proxy.
func WithTrustedProxyFunc(fn func(peerIP net.IP) bool) RequestOption {
	return func(o *requestOptions) {
		o.trustedProxyFunc = fn
	}
}

// ExtractHTTPRequestData parses standard WebProcessor attributes from an *http.Request.
// By default, it derives the client IP strictly from req.RemoteAddr to prevent header spoofing.
// Forwarded headers (X-Forwarded-For, X-Real-IP) are only parsed when trusted proxies are configured.
func ExtractHTTPRequestData(req *http.Request, opts ...RequestOption) HTTPRequestData {
	if req == nil {
		return HTTPRequestData{}
	}

	var ro requestOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&ro)
		}
	}

	ip := clientIP(req, ro)
	urlStr := ""
	if req.URL != nil {
		urlStr = req.URL.String()
	}

	return HTTPRequestData{
		URL:       urlStr,
		IP:        ip,
		Method:    req.Method,
		Server:    req.Host,
		Referrer:  req.Referer(),
		UserAgent: req.UserAgent(),
	}
}

// WithHTTPRequest attaches HTTP request metadata to ctx for extraction by WebProcessor.
func WithHTTPRequest(ctx context.Context, req *http.Request, opts ...RequestOption) context.Context {
	if req == nil {
		return ctx
	}
	data := ExtractHTTPRequestData(req, opts...)
	ctx = context.WithValue(ctx, httpContextKey{}, data)
	ctx = monogo.WithContext(ctx, map[string]interface{}{
		"_monogo_web_url":        data.URL,
		"_monogo_web_ip":         data.IP,
		"_monogo_web_method":     data.Method,
		"_monogo_web_server":     data.Server,
		"_monogo_web_referrer":   data.Referrer,
		"_monogo_web_user_agent": data.UserAgent,
	})
	return ctx
}

func parsePeerHostAndIP(remoteAddr string) (string, net.IP) {
	if remoteAddr == "" {
		return "", nil
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return host, net.ParseIP(host)
}

func clientIP(req *http.Request, opts requestOptions) string {
	peerHost, peerIP := parsePeerHostAndIP(req.RemoteAddr)

	// If no trusted proxy matcher is configured or the peer is not a trusted proxy,
	// default strictly to RemoteAddr to prevent header spoofing from untrusted callers.
	if opts.trustedProxyFunc == nil || peerIP == nil || !opts.trustedProxyFunc(peerIP) {
		return peerHost
	}

	// Peer is a trusted proxy: inspect forwarding headers.
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			ipStr := strings.TrimSpace(part)
			if ipStr != "" {
				return ipStr
			}
		}
	}
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		if ipStr := strings.TrimSpace(xri); ipStr != "" {
			return ipStr
		}
	}

	return peerHost
}

// WebOption configures WebProcessor behavior.
type WebOption func(*webOptions)

type webOptions struct {
	extraKey string
	reqOpts  requestOptions
}

// WithWebTrustedProxies configures trusted proxies for WebFromRequest.
func WithWebTrustedProxies(proxies ...string) WebOption {
	ro := WithTrustedProxies(proxies...)
	return func(o *webOptions) {
		ro(&o.reqOpts)
	}
}

// WithWebExtraKey nests HTTP fields under a specific key in Extra (e.g. Extra["http"]).
// By default, fields are merged directly into Extra (matching PHP Monolog).
func WithWebExtraKey(key string) WebOption {
	return func(o *webOptions) {
		o.extraKey = key
	}
}

// Web creates a processor modeled after PHP Monolog's WebProcessor. It injects
// HTTP request metadata (url, ip, http_method, server, referrer, user_agent) into Record.Extra
// when attached to context via WithHTTPRequest or HTTP middleware.
func Web(opts ...WebOption) monogo.ProcessorFunc {
	o := webOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return func(r monogo.Record) monogo.Record {
		if r.Context == nil {
			return r
		}

		urlVal, hasURL := r.Context["_monogo_web_url"].(string)
		ipVal, hasIP := r.Context["_monogo_web_ip"].(string)
		methodVal, hasMethod := r.Context["_monogo_web_method"].(string)
		serverVal, hasServer := r.Context["_monogo_web_server"].(string)
		refVal, hasRef := r.Context["_monogo_web_referrer"].(string)
		uaVal, hasUA := r.Context["_monogo_web_user_agent"].(string)

		if !hasURL && !hasIP && !hasMethod && !hasServer && !hasRef && !hasUA {
			return r
		}

		delete(r.Context, "_monogo_web_url")
		delete(r.Context, "_monogo_web_ip")
		delete(r.Context, "_monogo_web_method")
		delete(r.Context, "_monogo_web_server")
		delete(r.Context, "_monogo_web_referrer")
		delete(r.Context, "_monogo_web_user_agent")

		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}

		m := make(map[string]interface{})
		if urlVal != "" {
			m["url"] = urlVal
		}
		if ipVal != "" {
			m["ip"] = ipVal
		}
		if methodVal != "" {
			m["http_method"] = methodVal
		}
		if serverVal != "" {
			m["server"] = serverVal
		}
		if refVal != "" {
			m["referrer"] = refVal
		}
		if uaVal != "" {
			m["user_agent"] = uaVal
		}

		if o.extraKey != "" {
			r.Extra[o.extraKey] = m
		} else {
			for k, v := range m {
				r.Extra[k] = v
			}
		}

		return r
	}
}

// WebFromRequest creates a processor pre-bound to an explicit *http.Request.
func WebFromRequest(req *http.Request, opts ...WebOption) monogo.ProcessorFunc {
	o := webOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	data := ExtractHTTPRequestData(req, func(r *requestOptions) { *r = o.reqOpts })

	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}

		m := make(map[string]interface{})
		if data.URL != "" {
			m["url"] = data.URL
		}
		if data.IP != "" {
			m["ip"] = data.IP
		}
		if data.Method != "" {
			m["http_method"] = data.Method
		}
		if data.Server != "" {
			m["server"] = data.Server
		}
		if data.Referrer != "" {
			m["referrer"] = data.Referrer
		}
		if data.UserAgent != "" {
			m["user_agent"] = data.UserAgent
		}

		if o.extraKey != "" {
			r.Extra[o.extraKey] = m
		} else {
			for k, v := range m {
				r.Extra[k] = v
			}
		}

		return r
	}
}
