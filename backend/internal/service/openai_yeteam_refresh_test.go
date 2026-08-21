package service

import "testing"

func TestYeTeamCardCodeSupportsMetadataFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "RCL metadata", want: "RCL-ABCD-1234"},
		{name: "team metadata", want: "TEAM-C6F73D-UVJS-04D34DE66753"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Extra: map[string]any{"ye_team_card_code": tc.want}}
			if got := yeTeamCardCode(account); got != tc.want {
				t.Fatalf("yeTeamCardCode() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestYeTeamCardCodeNameFallbackRemainsRCLOnly(t *testing.T) {
	account := &Account{Name: "team-c6f73d-UVJS-04D34DE66753"}
	if got := yeTeamCardCode(account); got != "" {
		t.Fatalf("yeTeamCardCode() = %q for a name-only team identifier", got)
	}
	account.Name = "account RCL-ABCD-1234"
	if got := yeTeamCardCode(account); got != "RCL-ABCD-1234" {
		t.Fatalf("yeTeamCardCode() = %q, want RCL-ABCD-1234", got)
	}
}
