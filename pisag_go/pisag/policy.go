package pisag

import (
	"errors"
	"net/url"
	"strings"

	"example.com/pisag_go/ports"
)

var (
	ErrHostNotAllowed = errors.New("pisag: host not allowed")
	ErrPathNotAllowed = errors.New("pisag: path not allowed")
	ErrPortNotAllowed = errors.New("pisag: port not allowed")
)

func IsAllowed(u *url.URL, policy ports.Policy) error {
	host := strings.ToLower(u.Hostname())

	port := u.Port()
	// default https port
	if port == "" {
		port = "443"
	}

	for _, ah := range policy.AllowedHosts {
		if strings.ToLower(ah.Host) != host {
			continue
		}
		wantPort := ah.Port
		if wantPort == 0 {
			wantPort = 443
		}
		if port != itoa(wantPort) {
			continue
		}

		// Path prefix allow
		p := u.Path
		if p == "" {
			p = "/"
		}
		for _, pref := range ah.PathPrefixes {
			if pref == "" {
				continue
			}
			if !strings.HasPrefix(pref, "/") {
				pref = "/" + pref
			}
			if strings.HasPrefix(p, pref) {
				return nil
			}
		}
		return ErrPathNotAllowed
	}

	return ErrHostNotAllowed
}

func itoa(i int) string {
	// tiny int->string without fmt
	if i == 0 {
		return "0"
	}
	var b [16]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + (i % 10))
		i /= 10
	}
	return string(b[n:])
}
