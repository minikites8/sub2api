package requestretry

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, optIn, max string
		enabled          bool
		limit            int
	}{
		{name: "default", limit: 3},
		{name: "opt in", optIn: "True", enabled: true, limit: 3},
		{name: "case and whitespace", optIn: " TRUE ", max: " 2 ", enabled: true, limit: 2},
		{name: "zero", optIn: "true", max: "0", enabled: true, limit: 0},
		{name: "server cap", optIn: "true", max: "100", enabled: true, limit: 3},
		{name: "false", optIn: "False", max: "0", limit: 3},
		{name: "limit alone", max: "0", limit: 3},
		{name: "invalid boolean", optIn: "1", max: "0", limit: 3},
		{name: "negative", optIn: "true", max: "-1", enabled: true, limit: 3},
		{name: "fraction", optIn: "true", max: "1.5", enabled: true, limit: 3},
		{name: "overflow", optIn: "true", max: "999999999999999999999999", enabled: true, limit: 3},
		{name: "list", optIn: "true", max: "1, 2", enabled: true, limit: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/responses", nil)
			if tc.optIn != "" {
				r.Header.Set(AccountHeader, tc.optIn)
			}
			if tc.max != "" {
				r.Header.Set(MaxRetriesHeader, tc.max)
			}
			require.Equal(t, tc.enabled, Enabled(r))
			require.Equal(t, tc.limit, Limit(r, 3))
		})
	}
	// Repeated control headers cannot select an arbitrary first value.
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Add(AccountHeader, "true")
	r.Header.Add(AccountHeader, "false")
	require.False(t, Enabled(r))
	r.Header.Set(AccountHeader, "true")
	r.Header.Add(MaxRetriesHeader, "0")
	r.Header.Add(MaxRetriesHeader, "2")
	require.Equal(t, 3, Limit(r, 3))
	require.False(t, Enabled(nil))
	require.Equal(t, 3, Limit(nil, 3))
}
