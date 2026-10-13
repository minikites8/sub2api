package service

import (
	"net/url"
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexHarvestProxySIDPreservesProxyParameters(t *testing.T) {
	for _, proxy := range []string{
		"socks5://user-region-Rand-sid-OldSID12-t-5:secret@proxy.example:3000",
		"socks5h://user-region-US-sid-OldSID12-t-15:p%40ss%3Aword%2Fsecret@[::1]:3000",
		"https://user%40account-SID-OldSID12:secret@proxy.example:443",
		"http://user-sid-OldSID12@proxy.example:8080",
	} {
		t.Run(proxy, func(t *testing.T) {
			original, err := url.Parse(proxy)
			require.NoError(t, err)
			rotated, sid := rotateCodexHarvestProxySID(proxy)
			require.Len(t, sid, 8)
			require.Regexp(t, `^[a-zA-Z0-9]{8}$`, sid)
			require.NotEqual(t, "OldSID12", sid)
			require.Equal(t, sid, codexHarvestProxySID(rotated))
			actual, err := url.Parse(rotated)
			require.NoError(t, err)
			require.Equal(t, original.Scheme, actual.Scheme)
			require.Equal(t, original.Host, actual.Host)
			require.Equal(t, original.Path, actual.Path)
			require.Equal(t, original.RawQuery, actual.RawQuery)
			password, hasPassword := original.User.Password()
			gotPassword, gotHasPassword := actual.User.Password()
			require.Equal(t, password, gotPassword)
			require.Equal(t, hasPassword, gotHasPassword)
			require.Equal(t, original.User.Username(), regexp.MustCompile(sid).ReplaceAllString(actual.User.Username(), "OldSID12"))
		})
	}
}

func TestCodexHarvestProxySIDOnlyRotatesSupportedUsernameSessions(t *testing.T) {
	for _, proxy := range []string{
		"", "socks5://127.0.0.1:1080", "http://user:password-sid-OldSID12@proxy.example:8080",
		"socks5://user-session-OldSID12:secret@proxy.example:3000",
		"socks5://user-sid-:secret@proxy.example:3000", "socks5://user-sid-Old_SID:secret@proxy.example:3000",
		"ippool://active", "ftp://user-sid-OldSID12:secret@proxy.example:3000", "socks5://%bad@proxy.example:3000",
	} {
		rotated, sid := rotateCodexHarvestProxySID(proxy)
		require.Equal(t, proxy, rotated)
		require.Empty(t, sid)
	}
}

func TestCodexHarvestProxySIDConcurrentAttemptsGetIndependentSessions(t *testing.T) {
	const proxy = "socks5://user-sid-OldSID12-t-5:secret@proxy.example:3000"
	var wg sync.WaitGroup
	var seen sync.Map
	for range 128 {
		wg.Go(func() {
			_, sid := rotateCodexHarvestProxySID(proxy)
			_, duplicate := seen.LoadOrStore(sid, true)
			if duplicate {
				t.Errorf("duplicate session %q", sid)
			}
		})
	}
	wg.Wait()
}
