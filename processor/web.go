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

// ExtractHTTPRequestData parses standard WebProcessor attributes from an *http.Request.
func ExtractHTTPRequestData(req *http.Request) HTTPRequestData {
	if req == nil {
		return HTTPRequestData{}
	}

	ip := clientIP(req)
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
func WithHTTPRequest(ctx context.Context, req *http.Request) context.Context {
	if req == nil {
		return ctx
	}
	data := ExtractHTTPRequestData(req)
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

func clientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return req.RemoteAddr
}

// WebOption configures WebProcessor behavior.
type WebOption func(*webOptions)

type webOptions struct {
	extraKey string
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
	data := ExtractHTTPRequestData(req)
	o := webOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

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
