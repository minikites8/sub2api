package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBaiduVODModelsRegistersVideoFamilies(t *testing.T) {
	models := BaiduVODModels()
	require.Len(t, models, 14)
	for _, model := range []string{
		"happyhorse-1.0-t2v", "happyhorse-1.0-i2v", "happyhorse-1.0-r2v", "happyhorse-1.0-video-edit",
		"happyhorse-1.1-t2v", "happyhorse-1.1-i2v", "happyhorse-1.1-r2v", "happyhorse-1.1-video-edit",
	} {
		_, ok := BaiduVODModel(model)
		require.True(t, ok, model)
	}
	for _, model := range []string{
		"doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615",
		"doubao-seedance-1-5-pro-251215", "doubao-seedance-1-0-pro-250528", "doubao-seedance-1-0-pro-fast-251015",
	} {
		spec, ok := BaiduVODModel(model)
		require.True(t, ok, model)
		require.Equal(t, BaiduVODProviderSeedance, spec.Provider)
	}
}

func TestTranslateBaiduVODVideoRequestCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		request   BaiduVODVideoRequest
		mediaType string
	}{
		{name: "text", request: BaiduVODVideoRequest{Model: "happyhorse-1.0-t2v", Prompt: "horse", Resolution: "720P", Duration: 5}},
		{name: "first frame", request: BaiduVODVideoRequest{Model: "happyhorse-1.0-i2v", Prompt: "move", Resolution: "720P", Duration: 5, FirstFrame: json.RawMessage(`"https://example.com/first.png"`)}, mediaType: "first_frame"},
		{name: "reference", request: BaiduVODVideoRequest{Model: "happyhorse-1.1-r2v", Prompt: "reference", Resolution: "1080P", Duration: 5, ReferenceImages: []json.RawMessage{json.RawMessage(`{"url":"https://example.com/ref.png"}`)}}, mediaType: "reference_image"},
		{name: "edit", request: BaiduVODVideoRequest{Model: "happyhorse-1.1-video-edit", Prompt: "change shirt", Resolution: "720P", Duration: 5, Video: json.RawMessage(`{"url":"https://example.com/in.mp4"}`)}, mediaType: "video"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec, upstream, err := TranslateBaiduVODVideoRequest(test.request)
			require.NoError(t, err)
			require.Equal(t, test.request.Model, spec.Model)
			require.Equal(t, test.request.Prompt, upstream.Input.Prompt)
			require.Equal(t, test.request.Resolution, upstream.Parameters.Resolution)
			if test.mediaType == "" {
				require.Empty(t, upstream.Input.Media)
				return
			}
			require.NotEmpty(t, upstream.Input.Media)
			require.Equal(t, test.mediaType, upstream.Input.Media[0].Type)
		})
	}
}

func TestTranslateBaiduVODVideoRequestValidation(t *testing.T) {
	_, _, err := TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{Model: "happyhorse-1.0-t2v", Resolution: "720P", Duration: 5})
	require.EqualError(t, err, "prompt is required")

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{Model: "happyhorse-1.0-t2v", Prompt: "horse", Resolution: "480P", Duration: 5})
	require.EqualError(t, err, "model happyhorse-1.0-t2v does not support resolution 480P")
}

func TestTranslateBaiduVODSeedanceRequest(t *testing.T) {
	generateAudio, watermark := true, false
	req := BaiduVODVideoRequest{
		Model: "doubao-seedance-2-0-260128", Prompt: "cinematic flower field", Resolution: "4K", Ratio: "16:9", Duration: 15,
		FirstFrame:    json.RawMessage(`"https://example.com/first.png"`),
		Video:         json.RawMessage(`{"url":"https://example.com/reference.mp4"}`),
		GenerateAudio: &generateAudio, Watermark: &watermark,
	}
	spec, upstream, err := TranslateBaiduVODVideoRequest(req)
	require.NoError(t, err)
	require.Equal(t, BaiduVODProviderSeedance, spec.Provider)
	require.Len(t, upstream.Content, 3)

	raw, err := json.Marshal(upstream)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, req.Model, payload["model"])
	require.Equal(t, "4k", payload["resolution"])
	require.Equal(t, float64(15), payload["duration"])
	require.Equal(t, true, payload["generate_audio"])
	require.Equal(t, false, payload["watermark"])
	require.NotContains(t, payload, "input")
	require.NotContains(t, payload, "parameters")

	content := payload["content"].([]any)
	require.Equal(t, "text", content[0].(map[string]any)["type"])
	require.Equal(t, "first_frame", content[1].(map[string]any)["role"])
	require.Equal(t, "reference_video", content[2].(map[string]any)["role"])
}

func TestBaiduVODSeedanceValidation(t *testing.T) {
	_, _, err := TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "doubao-seedance-2-0-fast-260128", Prompt: "video", Resolution: "1080P", Duration: 5,
	})
	require.EqualError(t, err, "model doubao-seedance-2-0-fast-260128 does not support resolution 1080P")

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "doubao-seedance-1-5-pro-251215", Prompt: "video", Resolution: "720P", Duration: 13,
	})
	require.EqualError(t, err, "model doubao-seedance-1-5-pro-251215 duration must be between 4 and 12 seconds")

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "doubao-seedance-1-0-pro-250528", Prompt: "video", Resolution: "1080P", Duration: -1,
	})
	require.EqualError(t, err, "model doubao-seedance-1-0-pro-250528 does not support automatic duration")

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "doubao-seedance-2-0-260128", Resolution: "720P", Duration: 5,
		Audio: json.RawMessage(`"https://example.com/audio.mp3"`),
	})
	require.EqualError(t, err, "Seedance audio content requires an image or video")
}

func TestBaiduVODSubmitSeedanceUsesSeedanceEndpoint(t *testing.T) {
	_, payload, err := TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "doubao-seedance-2-0-260128", Prompt: "video", Resolution: "720P", Duration: 5,
	})
	require.NoError(t, err)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"cgt-seedance-1"}`)),
	}}
	svc := &BaiduVODVideoService{http: upstream}
	account := &Account{ID: 7, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAPIKey,
		"api_key":   "seedance-key",
	}}
	result, err := svc.Submit(context.Background(), account, payload)
	require.NoError(t, err)
	require.Equal(t, "cgt-seedance-1", result.TaskID)
	require.Equal(t, "queued", result.TaskStatus)
	require.Equal(t, "https://vod.bj.baidubce.com/v3/aigc/seedance"+BaiduVODSeedanceCreatePath, upstream.lastReq.URL.String())
	require.Equal(t, "Bearer seedance-key", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("X-DashScope-Async"))
}

func TestParseBaiduVODSeedanceDefaults(t *testing.T) {
	req, err := ParseBaiduVODVideoRequest([]byte(`{"model":"doubao-seedance-2-0-260128","prompt":"video"}`))
	require.NoError(t, err)
	require.Equal(t, "720P", req.Resolution)
	require.Equal(t, "adaptive", req.Ratio)
	require.Equal(t, 5, req.Duration)

	req, err = ParseBaiduVODVideoRequest([]byte(`{"model":"doubao-seedance-1-0-pro-250528","prompt":"video"}`))
	require.NoError(t, err)
	require.Equal(t, "1080P", req.Resolution)
	require.Equal(t, "16:9", req.Ratio)
}

func TestBCECanonicalEncoding(t *testing.T) {
	require.Equal(t, "/v1/folder%20name/%E4%B8%AD", canonicalBCEURI("/v1/folder name/中"))
	query := url.Values{
		"space key":     {"a b"},
		"中":             {"值"},
		"~":             {"x/y"},
		"Authorization": {"skip"},
	}
	require.Equal(t, "%E4%B8%AD=%E5%80%BC&space%20key=a%20b&~=x%2Fy", canonicalBCEQuery(query))
}

func TestBCEAuthV1UsesDeterministicSignedHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://vod.bj.baidubce.com/v2/aigc/bailian/api/v1/tasks/task-1?q=a+b", nil)
	require.NoError(t, err)
	now := time.Date(2026, 7, 27, 8, 9, 10, 0, time.UTC)
	require.NoError(t, BCEAuthV1(req, "test-ak", "test-sk", now, 30*time.Minute))
	require.Equal(t, "2026-07-27T08:09:10Z", req.Header.Get("x-bce-date"))
	require.Regexp(t, `^bce-auth-v1/test-ak/2026-07-27T08:09:10Z/1800/host;x-bce-date/[0-9a-f]{64}$`, req.Header.Get("Authorization"))
}

func TestBaiduVODUpstreamURLReplacesKnownAuthPrefix(t *testing.T) {
	account := &Account{Credentials: map[string]any{"auth_mode": BaiduVODAuthModeAKSK, "base_url": "https://vod.bj.baidubce.com/v3/aigc/bailian"}}
	got, mode, err := baiduVODUpstreamURL(account, BaiduVODProviderHappyHorse, BaiduVODCreatePath)
	require.NoError(t, err)
	require.Equal(t, BaiduVODAuthModeAKSK, mode)
	require.Equal(t, "https://vod.bj.baidubce.com/v2/aigc/bailian"+BaiduVODCreatePath, got)

	account.Credentials["auth_mode"] = BaiduVODAuthModeAPIKey
	account.Credentials["base_url"] = "https://vod.bj.baidubce.com/v2/aigc/bailian"
	got, mode, err = baiduVODUpstreamURL(account, BaiduVODProviderHappyHorse, BaiduVODTaskPath+"task-1")
	require.NoError(t, err)
	require.Equal(t, BaiduVODAuthModeAPIKey, mode)
	require.Equal(t, "https://vod.bj.baidubce.com/v3/aigc/bailian"+BaiduVODTaskPath+"task-1", got)

	account.Credentials["base_url"] = "https://vod.bj.baidubce.com/v3/aigc/bailian"
	got, mode, err = baiduVODUpstreamURL(account, BaiduVODProviderSeedance, BaiduVODSeedanceCreatePath)
	require.NoError(t, err)
	require.Equal(t, BaiduVODAuthModeAPIKey, mode)
	require.Equal(t, "https://vod.bj.baidubce.com/v3/aigc/seedance"+BaiduVODSeedanceCreatePath, got)
}

func TestParseBaiduVODVideoURLExpiry(t *testing.T) {
	expires := ParseBaiduVODVideoURLExpiry("https://example.com/video.mp4?Expires=1785148800&Signature=x")
	require.NotNil(t, expires)
	require.Equal(t, int64(1785148800), expires.Unix())
}

func TestDefaultHappyHorseVideoPrice(t *testing.T) {
	price, ok := getDefaultHappyHorseVideoPrice("happyhorse-1.0-t2v", "720P")
	require.True(t, ok)
	require.Equal(t, 0.9, price)
	price, ok = getDefaultHappyHorseVideoPrice("happyhorse-1.0-video-edit", "1080P")
	require.True(t, ok)
	require.Equal(t, 1.6, price)
	price, ok = getDefaultHappyHorseVideoPrice("happyhorse-1.1-r2v", "1080P")
	require.True(t, ok)
	require.Equal(t, 1.2, price)
}

func TestDefaultSeedanceVideoPrice(t *testing.T) {
	price, ok := getDefaultSeedanceVideoPrice("doubao-seedance-2-0-260128", "4K")
	require.True(t, ok)
	require.Equal(t, 5.05, price)
	price, ok = getDefaultSeedanceVideoPrice("doubao-seedance-2-0-mini-260615", "720P")
	require.True(t, ok)
	require.Equal(t, 0.50, price)
	price, ok = getDefaultSeedanceVideoPrice("doubao-seedance-1-5-pro-251215", "1080P")
	require.True(t, ok)
	require.Equal(t, 0.778, price)
}
