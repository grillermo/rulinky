package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
)

// readParams merges query parameters with a JSON or form-encoded body (the
// body wins), like Rails' params.
func readParams(r *http.Request) (map[string]any, error) {
	params := map[string]any{}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(body)) == 0 {
			return params, nil
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, err
		}
		maps.Copy(params, data)
		return params, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for k, v := range r.PostForm {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	return params, nil
}

// stringParam is Ruby's params[key].to_s: "" for missing/null, the string
// itself, or the value printed.
func stringParam(params map[string]any, key string) string {
	switch v := params[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// castBool mirrors ActiveModel::Type::Boolean: ok is false for nil and "",
// a fixed set of strings and 0 mean false, everything else means true.
func castBool(v any) (value, ok bool) {
	switch b := v.(type) {
	case nil:
		return false, false
	case bool:
		return b, true
	case float64:
		return b != 0, true
	case string:
		switch b {
		case "":
			return false, false
		case "0", "f", "F", "false", "FALSE", "off", "OFF":
			return false, true
		}
		return true, true
	}
	return true, true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorsBody(field, message string) map[string]map[string]string {
	return map[string]map[string]string{"errors": {field: message}}
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}

// clientIP prefers Cloudflare's header (the app sits behind a tunnel).
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
