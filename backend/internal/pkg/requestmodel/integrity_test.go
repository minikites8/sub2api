package requestmodel

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"testing"

	"github.com/tidwall/gjson"
)

func TestModelParserDifferential(t *testing.T) {
	body := []byte(`{"model":"cheap","model":"expensive"}`)
	var upstream struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &upstream); err != nil {
		t.Fatal(err)
	}
	if gateway := gjson.GetBytes(body, "model").String(); gateway != "cheap" || upstream.Model != "expensive" {
		t.Fatalf("gateway=%q, upstream=%q", gateway, upstream.Model)
	}
}

func TestValidateJSONModels(t *testing.T) {
	for _, tc := range []struct {
		body     string
		conflict bool
	}{
		{`{"model":"cheap","model":"expensive"}`, true},
		{`{"model":"cheap","Model":"expensive"}`, true},
		{`{"model":"cheap","\u006dodel":"expensive"}`, true},
		{`{"model":"cheap","model":null}`, true},
		{`{"model":null,"model":"expensive"}`, true},
		{`{"model":"cheap","model":""}`, true},
		{`{"session":{"model":"cheap","model":"expensive"}}`, true},
		{`{"session":{"model":"cheap"},"Session":{"model":"expensive"}}`, true},
		{`{"session":{"model":"cheap"},"session":{}}`, true},
		{`{"model":"cheap"}`, false},
		{`{"model":"cheap","model":"cheap"}`, true},
		{`{"model":"cheap","Model":"cheap"}`, true},
		{`{"Model":"cheap"}`, true},
		{`{"model":"cheap","input":[{"model":"application-data"}]}`, false},
		{`{"session":{"model":"live"},"sdp":"v=0"}`, false},
		{`{"type":"response.create"}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			err := ValidateJSONModels([]byte(tc.body))
			if errors.Is(err, ErrAmbiguousModel) != tc.conflict {
				t.Fatalf("error=%v, conflict=%v", err, tc.conflict)
			}
		})
	}
}

func TestValidateMultipartModels(t *testing.T) {
	for _, tc := range []struct {
		name             string
		models, sessions []string
		conflict         bool
	}{
		{name: "single", models: []string{"cheap"}},
		{name: "same", models: []string{"cheap", "cheap"}, conflict: true},
		{name: "different", models: []string{"cheap", "expensive"}, conflict: true},
		{name: "empty final", models: []string{"cheap", ""}, conflict: true},
		{name: "session", sessions: []string{`{"model":"live"}`}},
		{name: "duplicate session model", sessions: []string{`{"model":"cheap","model":"expensive"}`}, conflict: true},
		{name: "duplicate sessions", sessions: []string{`{"model":"cheap"}`, `{}`}, conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			w := multipart.NewWriter(&body)
			for _, model := range tc.models {
				_ = w.WriteField("model", model)
			}
			for _, session := range tc.sessions {
				_ = w.WriteField("session", session)
			}
			_ = w.WriteField("input", `{"model":"nested"}`)
			_ = w.Close()
			err := ValidateBodyModels(w.FormDataContentType(), body.Bytes())
			if errors.Is(err, ErrAmbiguousModel) != tc.conflict {
				t.Fatalf("error=%v, conflict=%v", err, tc.conflict)
			}
		})
	}
}
