package service

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBaiduVODModelsRegistersHappyHorseFamilies(t *testing.T) {
	models := BaiduVODModels()
	require.Len(t, models, 8)
	for _, model := range []string{
		"happyhorse-1.0-t2v", "happyhorse-1.0-i2v", "happyhorse-1.0-r2v", "happyhorse-1.0-video-edit",
		"happyhorse-1.1-t2v", "happyhorse-1.1-i2v", "happyhorse-1.1-r2v", "happyhorse-1.1-video-edit",
	} {
		_, ok := BaiduVODModel(model)
		require.True(t, ok, model)
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
	require.EqualError(t, err, "unsupported HappyHorse resolution: 480P")
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
	got, mode, err := baiduVODUpstreamURL(account, BaiduVODCreatePath)
	require.NoError(t, err)
	require.Equal(t, BaiduVODAuthModeAKSK, mode)
	require.Equal(t, "https://vod.bj.baidubce.com/v2/aigc/bailian"+BaiduVODCreatePath, got)

	account.Credentials["auth_mode"] = BaiduVODAuthModeAPIKey
	account.Credentials["base_url"] = "https://vod.bj.baidubce.com/v2/aigc/bailian"
	got, mode, err = baiduVODUpstreamURL(account, BaiduVODTaskPath+"task-1")
	require.NoError(t, err)
	require.Equal(t, BaiduVODAuthModeAPIKey, mode)
	require.Equal(t, "https://vod.bj.baidubce.com/v3/aigc/bailian"+BaiduVODTaskPath+"task-1", got)
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
