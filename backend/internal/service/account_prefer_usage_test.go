package service

import "testing"

func TestAccountIsPreferUsageEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil account", account: nil, want: false},
		{name: "missing extra", account: &Account{}, want: false},
		{name: "enabled", account: &Account{Extra: map[string]any{"prefer_usage": true}}, want: true},
		{name: "disabled", account: &Account{Extra: map[string]any{"prefer_usage": false}}, want: false},
		{name: "invalid value", account: &Account{Extra: map[string]any{"prefer_usage": "true"}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.IsPreferUsageEnabled(); got != tt.want {
				t.Fatalf("IsPreferUsageEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
