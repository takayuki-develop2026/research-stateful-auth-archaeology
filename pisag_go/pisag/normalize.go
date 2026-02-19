package pisag

import (
	"errors"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/idna"
)

var (
	ErrInvalidURL       = errors.New("pisag: invalid url")
	ErrNonHTTPS         = errors.New("pisag: only https is allowed")
	ErrUserInfoRejected = errors.New("pisag: userinfo is rejected")
)

func NormalizeURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return nil, ErrInvalidURL
	}

	// Scheme: https only (v4 contract)
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, ErrNonHTTPS
	}

	// Reject userinfo (user:pass@host) to avoid spoofing
	if u.User != nil {
		return nil, ErrUserInfoRejected
	}

	// IDNA normalize host
	host := u.Hostname()
	if host == "" {
		return nil, ErrInvalidURL
	}
	asciiHost, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return nil, ErrInvalidURL
	}

	// Clean path (prevent .. / encoded path trickery)
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	// url.Path is decoded-ish; we want stable canonical path.
	decodedPath, _ := url.PathUnescape(p)
	clean := path.Clean("/" + decodedPath)
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}

	// Rebuild host with explicit port if provided
	port := u.Port()
	if port != "" {
		u.Host = asciiHost + ":" + port
	} else {
		u.Host = asciiHost
	}
	u.Path = clean

	// Remove fragment
	u.Fragment = ""

	return u, nil
}