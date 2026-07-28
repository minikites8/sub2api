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
	require.Len(t, models, 17)
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
	for model, upstreamModel := range map[string]string{
		"veo-3.1": "VE3.1", "veo-3.1-fast": "VE3.1F", "veo-3.1-lite": "VE3.1L",
	} {
		spec, ok := BaiduVODModel(model)
		require.True(t, ok, model)
		require.Equal(t, BaiduVODProviderVeo, spec.Provider)
		require.Equal(t, upstreamModel, spec.UpstreamModel)
		require.Equal(t, model != "veo-3.1-lite", spec.AllowText)
		require.Equal(t, model != "veo-3.1-lite", spec.AllowReferences)
	}
}

func TestBaiduVODModelResolvesVeoSilentAliases(t *testing.T) {
	for alias, baseModel := range map[string]string{
		"veo-3.1-silent":      "veo-3.1",
		"veo-3.1-fast-silent": "veo-3.1-fast",
		"veo-3.1-lite-silent": "veo-3.1-lite",
	} {
		spec, ok := BaiduVODModel(alias)
		require.True(t, ok, alias)
		require.Equal(t, baseModel, spec.Model)
		require.True(t, spec.ForceSilent)
	}

	_, ok := BaiduVODModel("happyhorse-1.1-t2v-silent")
	require.False(t, ok)
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

func TestTranslateBaiduVODVeoSilentAliasForcesAudioOff(t *testing.T) {
	generateAudio := true
	spec, upstream, err := TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1-fast-silent", Prompt: "a paper airplane flies through a library",
		Resolution: "720P", Ratio: "16:9", Duration: 4, GenerateAudio: &generateAudio,
	})
	require.NoError(t, err)
	require.Equal(t, "veo-3.1-fast", spec.Model)
	require.Equal(t, "VE3.1F", upstream.Model)
	require.NotNil(t, upstream.VeoInput)
	require.NotNil(t, upstream.VeoInput.GenerateAudio)
	require.False(t, *upstream.VeoInput.GenerateAudio)

	raw, err := json.Marshal(upstream)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model": "VE3.1F",
		"modelVE31FTaskInput": {
			"prompt": "a paper airplane flies through a library",
			"n": 1,
			"aspectRatio": "16:9",
			"durationSeconds": 4,
			"resolution": "720p",
			"generateAudio": false
		}
	}`, string(raw))
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

func TestTranslateBaiduVODVeoRequest(t *testing.T) {
	generateAudio := false
	seed := int64(42)
	req := BaiduVODVideoRequest{
		Model: "veo-3.1-fast", Prompt: "camera moves forward", NegativePrompt: "blur",
		Resolution: "4K", Ratio: "9:16", Duration: 6,
		FirstFrame:       json.RawMessage(`{"imageUrl":"https://example.com/first.png"}`),
		LastFrame:        json.RawMessage(`"https://example.com/last.png"`),
		GenerateAudio:    &generateAudio,
		PersonGeneration: "disallow",
		Seed:             &seed,
	}
	spec, upstream, err := TranslateBaiduVODVideoRequest(req)
	require.NoError(t, err)
	require.Equal(t, BaiduVODProviderVeo, spec.Provider)
	require.Equal(t, BaiduVODCapabilityI2V, spec.Capability)
	require.Equal(t, "VE3.1F", upstream.Model)
	require.Equal(t, BaiduVODVeoModeImage, upstream.VeoMode)

	raw, err := json.Marshal(upstream)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, "VE3.1F", payload["model"])
	require.NotContains(t, payload, "modelVE31TaskInput")
	input := payload["modelVE31FTaskInput"].(map[string]any)
	require.Equal(t, "camera moves forward", input["prompt"])
	require.Equal(t, "https://example.com/first.png", input["image"].(map[string]any)["imageUrl"])
	require.Equal(t, "https://example.com/last.png", input["lastFrame"].(map[string]any)["imageUrl"])
	require.Equal(t, "4k", input["resolution"])
	require.Equal(t, "9:16", input["aspectRatio"])
	require.Equal(t, float64(6), input["durationSeconds"])
	require.Equal(t, false, input["generateAudio"])
	require.Equal(t, "disallow", input["personGeneration"])
	require.Equal(t, float64(42), input["seed"])
}

func TestTranslateBaiduVODVeoTextRequest(t *testing.T) {
	generateAudio := true
	req := BaiduVODVideoRequest{
		Model: "veo-3.1", Prompt: "a golden dog runs through a flower field", NegativePrompt: "blur",
		Resolution: "720P", Ratio: "16:9", Duration: 4, GenerateAudio: &generateAudio,
	}
	spec, upstream, err := TranslateBaiduVODVideoRequest(req)
	require.NoError(t, err)
	require.Equal(t, BaiduVODCapabilityT2V, spec.Capability)
	require.Equal(t, BaiduVODVeoTextCreatePath, spec.CreatePath)
	require.Equal(t, BaiduVODVeoModeText, upstream.VeoMode)

	raw, err := json.Marshal(upstream)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model":"VE3.1",
		"modelVE31TaskInput":{
			"prompt":"a golden dog runs through a flower field",
			"n":1,
			"aspectRatio":"16:9",
			"durationSeconds":4,
			"resolution":"720p",
			"negativePrompt":"blur",
			"generateAudio":true
		}
	}`, string(raw))
}

func TestTranslateBaiduVODVeoReferenceAndValidation(t *testing.T) {
	spec, upstream, err := TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1", Prompt: "preserve the character", Resolution: "1080P", Ratio: "16:9", Duration: 8,
		ReferenceImages: []json.RawMessage{json.RawMessage(`"https://example.com/reference.png"`)},
	})
	require.NoError(t, err)
	require.Equal(t, BaiduVODCapabilityR2V, spec.Capability)
	require.Equal(t, BaiduVODVeoModeImage, upstream.VeoMode)
	raw, err := json.Marshal(upstream)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model":"VE3.1",
		"modelVE31TaskInput":{
			"prompt":"preserve the character",
			"referenceImages":{"imageUrl":"https://example.com/reference.png"},
			"n":1,
			"aspectRatio":"16:9",
			"durationSeconds":8,
			"resolution":"1080p"
		}
	}`, string(raw))

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1-lite", Prompt: "video", Resolution: "720P", Ratio: "16:9", Duration: 8,
		ReferenceImages: []json.RawMessage{json.RawMessage(`"https://example.com/reference.png"`)},
	})
	require.EqualError(t, err, "model veo-3.1-lite does not support reference images")

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1", Prompt: "video", Resolution: "720P", Ratio: "16:9", Duration: 5,
		Image: json.RawMessage(`"https://example.com/first.png"`),
	})
	require.EqualError(t, err, "model veo-3.1 duration must be 4, 6, or 8 seconds")

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1", Prompt: "video", Resolution: "720P", Ratio: "1:1", Duration: 8,
		Image: json.RawMessage(`"https://example.com/first.png"`),
	})
	require.EqualError(t, err, "model veo-3.1 supports 16:9 and 9:16 ratios")

	_, _, err = TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1-lite", Prompt: "video", Resolution: "720P", Ratio: "16:9", Duration: 4,
	})
	require.EqualError(t, err, "model veo-3.1-lite requires an image input")
}

func TestParseBaiduVODVeoDefaults(t *testing.T) {
	req, err := ParseBaiduVODVideoRequest([]byte(`{"model":"veo-3.1-lite","prompt":"video","image":"https://example.com/first.png"}`))
	require.NoError(t, err)
	require.Equal(t, "720P", req.Resolution)
	require.Equal(t, "16:9", req.Ratio)
	require.Equal(t, 8, req.Duration)
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

func TestBaiduVODSubmitVeoUsesDirectAKSKEndpoint(t *testing.T) {
	_, payload, err := TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1-fast", Prompt: "video", Resolution: "720P", Ratio: "16:9", Duration: 4,
		Image: json.RawMessage(`"https://example.com/first.png"`),
	})
	require.NoError(t, err)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"taskId":"tsk-veo-1","requestId":"req-veo-1"}`)),
	}}
	svc := &BaiduVODVideoService{http: upstream}
	account := &Account{ID: 8, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAKSK, "access_key_id": "veo-ak", "secret_access_key": "veo-sk",
	}}
	result, err := svc.Submit(context.Background(), account, payload)
	require.NoError(t, err)
	require.Equal(t, "tsk-veo-1", result.TaskID)
	require.Equal(t, "req-veo-1", result.RequestID)
	require.Equal(t, "https://vod.bj.baidubce.com"+BaiduVODVeoCreatePath, upstream.lastReq.URL.String())
	require.Regexp(t, `^bce-auth-v1/veo-ak/`, upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("X-DashScope-Async"))
}

func TestBaiduVODSubmitVeoTextUsesTextEndpoint(t *testing.T) {
	_, payload, err := TranslateBaiduVODVideoRequest(BaiduVODVideoRequest{
		Model: "veo-3.1-fast", Prompt: "a paper airplane flies through a library",
		Resolution: "720P", Ratio: "16:9", Duration: 4,
	})
	require.NoError(t, err)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"taskId":"tsk-veo-text-1","requestId":"req-veo-text-1"}`)),
	}}
	svc := &BaiduVODVideoService{http: upstream}
	account := &Account{ID: 18, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAKSK, "access_key_id": "veo-ak", "secret_access_key": "veo-sk",
	}}
	result, err := svc.Submit(context.Background(), account, payload)
	require.NoError(t, err)
	require.Equal(t, "tsk-veo-text-1", result.TaskID)
	require.Equal(t, "https://vod.bj.baidubce.com"+BaiduVODVeoTextCreatePath, upstream.lastReq.URL.String())
	require.Regexp(t, `^bce-auth-v1/veo-ak/`, upstream.lastReq.Header.Get("Authorization"))
}

func TestBaiduVODSelectAccountVeoUsesOnlyV2AKSK(t *testing.T) {
	apiKeyAccount := Account{
		ID: 11, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 100,
		Credentials: map[string]any{"auth_mode": BaiduVODAuthModeAPIKey, "api_key": "v3-key"},
	}
	akskAccount := Account{
		ID: 12, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 1,
		Credentials: map[string]any{"auth_mode": BaiduVODAuthModeAKSK, "access_key_id": "veo-ak", "secret_access_key": "veo-sk"},
	}
	svc := &BaiduVODVideoService{accounts: stubOpenAIAccountRepo{accounts: []Account{apiKeyAccount, akskAccount}}}

	account, err := svc.SelectAccount(context.Background(), nil, "veo-3.1-lite")
	require.NoError(t, err)
	require.Equal(t, akskAccount.ID, account.ID)

	account, err = svc.SelectAccount(context.Background(), nil, "doubao-seedance-2-0-260128")
	require.NoError(t, err)
	require.Equal(t, apiKeyAccount.ID, account.ID)
}

func TestBaiduVODSelectAccountSilentAliasUsesBaseModelMapping(t *testing.T) {
	account := Account{
		ID: 14, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"auth_mode": BaiduVODAuthModeAKSK, "access_key_id": "veo-ak", "secret_access_key": "veo-sk",
			"model_mapping": map[string]any{"veo-3.1-fast": "VE3.1F"},
		},
	}
	svc := &BaiduVODVideoService{accounts: stubOpenAIAccountRepo{accounts: []Account{account}}}

	selected, err := svc.SelectAccount(context.Background(), nil, "veo-3.1-fast-silent")
	require.NoError(t, err)
	require.Equal(t, account.ID, selected.ID)
}

func TestBaiduVODSelectAccountVeoReportsMissingV2Account(t *testing.T) {
	apiKeyAccount := Account{
		ID: 13, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"auth_mode": BaiduVODAuthModeAPIKey, "api_key": "v3-key"},
	}
	svc := &BaiduVODVideoService{accounts: stubOpenAIAccountRepo{accounts: []Account{apiKeyAccount}}}

	_, err := svc.SelectAccount(context.Background(), nil, "veo-3.1-lite")
	require.EqualError(t, err, "no available accounts for Baidu VOD V2 AK/SK model: veo-3.1-lite")
}

func TestBaiduVODPollVeoNormalizesTaskResult(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"taskId":"tsk-veo-2",
			"type":"VIDEO_GENERATE",
			"status":"FINISHED",
			"videoGenerateTaskInfo":{
				"status":"success",
				"videoGenerateTaskOutput":{"mediaBasicInfos":[{
					"source":{"sourceUrl":"https://example.com/veo.mp4"},
					"sourceMetadata":{"durationInSecond":8,"video":{"widthInPixel":3840,"heightInPixel":2160}}
				}]}
			}
		}`)),
	}}
	svc := &BaiduVODVideoService{http: upstream}
	account := &Account{ID: 9, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAKSK, "access_key_id": "veo-ak", "secret_access_key": "veo-sk",
	}}
	task := &BaiduVODVideoTask{Model: "veo-3.1", Provider: BaiduVODProviderVeo, UpstreamTaskID: "tsk-veo-2"}
	result, err := svc.Poll(context.Background(), account, task)
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", result.Output.TaskStatus)
	require.Equal(t, "https://example.com/veo.mp4", result.Output.VideoURL)
	require.NotNil(t, result.Usage)
	require.Equal(t, 8, result.Usage.OutputVideoDuration)
	require.Equal(t, "4K", result.Usage.Resolution)
	require.Equal(t, 1, result.Usage.VideoCount)
	require.Equal(t, "https://vod.bj.baidubce.com"+BaiduVODVeoTaskPath+"tsk-veo-2", upstream.lastReq.URL.String())
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

	account.Credentials["auth_mode"] = BaiduVODAuthModeAKSK
	got, mode, err = baiduVODUpstreamURL(account, BaiduVODProviderVeo, BaiduVODVeoCreatePath)
	require.NoError(t, err)
	require.Equal(t, BaiduVODAuthModeAKSK, mode)
	require.Equal(t, "https://vod.bj.baidubce.com"+BaiduVODVeoCreatePath, got)

	account.Credentials["auth_mode"] = BaiduVODAuthModeAPIKey
	_, _, err = baiduVODUpstreamURL(account, BaiduVODProviderVeo, BaiduVODVeoCreatePath)
	require.EqualError(t, err, "Baidu VOD Veo models require AK/SK authentication")
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
