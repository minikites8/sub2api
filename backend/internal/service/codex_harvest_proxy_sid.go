package service

import (
	"crypto/rand"
	"net/url"
	"regexp"
	"strings"
)

// Residential proxy providers encode a sticky session in the username.
// Rotate the session for each mint attempt and pin its concrete URL to the ticket.
var codexHarvestProxySIDRE = regexp.MustCompile(`(?i)-sid-([a-z0-9]+)(?:-|$)`)

func codexHarvestProxySIDParts(proxy string) (*url.URL, []int) {
	u, err := url.Parse(strings.TrimSpace(proxy))
	if err != nil || u.Host == "" || u.User == nil {
		return nil, nil
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, nil
	}
	match := codexHarvestProxySIDRE.FindStringSubmatchIndex(u.User.Username())
	if len(match) == 0 {
		return nil, nil
	}
	return u, match
}

func codexHarvestProxySID(proxy string) string {
	u, match := codexHarvestProxySIDParts(proxy)
	if u == nil {
		return ""
	}
	return u.User.Username()[match[2]:match[3]]
}

func rotateCodexHarvestProxySID(proxy string) (string, string) {
	u, match := codexHarvestProxySIDParts(proxy)
	if u == nil {
		return proxy, ""
	}
	username := u.User.Username()
	previous := username[match[2]:match[3]]
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	sid := make([]byte, len(previous))
	for {
		for i := range sid {
			// Rejection sampling keeps all 62 characters equally likely.
			var b [1]byte
			for {
				_, _ = rand.Read(b[:])
				if b[0] < 248 {
					sid[i] = alphabet[int(b[0])%len(alphabet)]
					break
				}
			}
		}
		if string(sid) != previous {
			break
		}
	}
	username = username[:match[2]] + string(sid) + username[match[3]:]
	if password, ok := u.User.Password(); ok {
		u.User = url.UserPassword(username, password)
	} else {
		u.User = url.User(username)
	}
	return u.String(), string(sid)
}
