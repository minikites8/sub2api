package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	BaiduVODDefaultBaseURL       = "https://vod.bj.baidubce.com"
	BaiduVODCreatePath           = "/api/v1/services/aigc/video-generation/video-synthesis"
	BaiduVODTaskPath             = "/api/v1/tasks/"
	BaiduVODTaskStatusPath       = "/tasks/"
	BaiduVODSeedanceCreatePath   = "/api/v3/contents/generations/tasks"
	BaiduVODSeedanceTaskPath     = "/api/v3/contents/generations/tasks/"
	BaiduVODVeoCreatePath        = "/v2/aigc/image_to_video"
	BaiduVODVeoTextCreatePath    = "/v2/aigc/text_to_video"
	BaiduVODVeoTaskPath          = "/v2/tasks/"
	BaiduVODProviderHappyHorse   = "happyhorse"
	BaiduVODProviderSeedance     = "seedance"
	BaiduVODProviderVeo          = "veo"
	BaiduVODProvider             = BaiduVODProviderHappyHorse
	BaiduVODAuthModeAPIKey       = "apikey"
	BaiduVODAuthModeAKSK         = "aksk"
	baiduVODDefaultDuration      = 5
	baiduVODSeedanceMaxMediaSecs = 15
)

type BaiduVODVideoCapability string

const (
	BaiduVODCapabilityT2V   BaiduVODVideoCapability = "t2v"
	BaiduVODCapabilityI2V   BaiduVODVideoCapability = "i2v"
	BaiduVODCapabilityR2V   BaiduVODVideoCapability = "r2v"
	BaiduVODCapabilityEdit  BaiduVODVideoCapability = "video_edit"
	BaiduVODCapabilityMulti BaiduVODVideoCapability = "multimodal"
)

type BaiduVODModelSpec struct {
	Model             string
	UpstreamModel     string
	Provider          string
	Capability        BaiduVODVideoCapability
	CreatePath        string
	TaskPath          string
	Resolutions       []string
	DefaultResolution string
	DefaultRatio      string
	DefaultDuration   int
	MinDuration       int
	MaxDuration       int
	AllowAutoDuration bool
	AllowText         bool
	AllowReferences   bool
}

var baiduVODModelRegistry = map[string]BaiduVODModelSpec{
	"happyhorse-1.0-t2v":                  happyHorseModelSpec("happyhorse-1.0-t2v", BaiduVODCapabilityT2V),
	"happyhorse-1.0-i2v":                  happyHorseModelSpec("happyhorse-1.0-i2v", BaiduVODCapabilityI2V),
	"happyhorse-1.0-r2v":                  happyHorseModelSpec("happyhorse-1.0-r2v", BaiduVODCapabilityR2V),
	"happyhorse-1.0-video-edit":           happyHorseModelSpec("happyhorse-1.0-video-edit", BaiduVODCapabilityEdit),
	"happyhorse-1.1-t2v":                  happyHorseModelSpec("happyhorse-1.1-t2v", BaiduVODCapabilityT2V),
	"happyhorse-1.1-i2v":                  happyHorseModelSpec("happyhorse-1.1-i2v", BaiduVODCapabilityI2V),
	"happyhorse-1.1-r2v":                  happyHorseModelSpec("happyhorse-1.1-r2v", BaiduVODCapabilityR2V),
	"happyhorse-1.1-video-edit":           happyHorseModelSpec("happyhorse-1.1-video-edit", BaiduVODCapabilityEdit),
	"doubao-seedance-2-0-260128":          seedanceModelSpec("doubao-seedance-2-0-260128", []string{"480P", "720P", "1080P", "4K"}, "720P", 4, 15, true),
	"doubao-seedance-2-0-fast-260128":     seedanceModelSpec("doubao-seedance-2-0-fast-260128", []string{"480P", "720P"}, "720P", 4, 15, true),
	"doubao-seedance-2-0-mini-260615":     seedanceModelSpec("doubao-seedance-2-0-mini-260615", []string{"480P", "720P"}, "720P", 4, 15, true),
	"doubao-seedance-1-5-pro-251215":      seedanceModelSpec("doubao-seedance-1-5-pro-251215", []string{"480P", "720P", "1080P"}, "720P", 4, 12, true),
	"doubao-seedance-1-0-pro-250528":      seedanceModelSpec("doubao-seedance-1-0-pro-250528", []string{"480P", "720P", "1080P"}, "1080P", 2, 12, false),
	"doubao-seedance-1-0-pro-fast-251015": seedanceModelSpec("doubao-seedance-1-0-pro-fast-251015", []string{"480P", "720P", "1080P"}, "1080P", 2, 12, false),
	"veo-3.1":                             veoModelSpec("veo-3.1", "VE3.1", true, true),
	"veo-3.1-fast":                        veoModelSpec("veo-3.1-fast", "VE3.1F", true, true),
	"veo-3.1-lite":                        veoModelSpec("veo-3.1-lite", "VE3.1L", false, false),
}

func happyHorseModelSpec(model string, capability BaiduVODVideoCapability) BaiduVODModelSpec {
	return BaiduVODModelSpec{
		Model: model, UpstreamModel: model, Provider: BaiduVODProviderHappyHorse, Capability: capability,
		CreatePath: BaiduVODCreatePath, TaskPath: BaiduVODTaskPath,
		Resolutions: []string{"720P", "1080P"}, DefaultResolution: "720P", DefaultRatio: "16:9",
		DefaultDuration: baiduVODDefaultDuration,
	}
}

func seedanceModelSpec(model string, resolutions []string, defaultResolution string, minDuration, maxDuration int, allowAutoDuration bool) BaiduVODModelSpec {
	defaultRatio := "adaptive"
	if strings.Contains(model, "seedance-1-0") {
		defaultRatio = "16:9"
	}
	return BaiduVODModelSpec{
		Model: model, UpstreamModel: model, Provider: BaiduVODProviderSeedance, Capability: BaiduVODCapabilityMulti,
		CreatePath: BaiduVODSeedanceCreatePath, TaskPath: BaiduVODSeedanceTaskPath,
		Resolutions: resolutions, DefaultResolution: defaultResolution, DefaultRatio: defaultRatio,
		DefaultDuration: baiduVODDefaultDuration, MinDuration: minDuration, MaxDuration: maxDuration, AllowAutoDuration: allowAutoDuration,
	}
}

func veoModelSpec(model, upstreamModel string, allowText, allowReferences bool) BaiduVODModelSpec {
	return BaiduVODModelSpec{
		Model: model, UpstreamModel: upstreamModel, Provider: BaiduVODProviderVeo, Capability: BaiduVODCapabilityI2V,
		CreatePath: BaiduVODVeoCreatePath, TaskPath: BaiduVODVeoTaskPath,
		Resolutions: []string{"720P", "1080P", "4K"}, DefaultResolution: "720P", DefaultRatio: "16:9",
		DefaultDuration: 8, MinDuration: 4, MaxDuration: 8, AllowText: allowText, AllowReferences: allowReferences,
	}
}

func BaiduVODModel(model string) (BaiduVODModelSpec, bool) {
	spec, ok := baiduVODModelRegistry[strings.ToLower(strings.TrimSpace(model))]
	return spec, ok
}

func baiduVODModelForUpstream(model string) (BaiduVODModelSpec, bool) {
	if spec, ok := BaiduVODModel(model); ok {
		return spec, true
	}
	model = strings.TrimSpace(model)
	for _, spec := range baiduVODModelRegistry {
		if strings.EqualFold(spec.UpstreamModel, model) {
			return spec, true
		}
	}
	return BaiduVODModelSpec{}, false
}

func BaiduVODModels() []BaiduVODModelSpec {
	models := make([]BaiduVODModelSpec, 0, len(baiduVODModelRegistry))
	for _, spec := range baiduVODModelRegistry {
		models = append(models, spec)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Model < models[j].Model })
	return models
}

type BaiduVODMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Role string `json:"role,omitempty"`
}

type BaiduVODVideoRequest struct {
	Model            string            `json:"model"`
	Prompt           string            `json:"prompt"`
	NegativePrompt   string            `json:"negative_prompt"`
	Seconds          int               `json:"seconds"`
	Duration         int               `json:"duration"`
	N                int               `json:"n"`
	Size             string            `json:"size"`
	Resolution       string            `json:"resolution"`
	Ratio            string            `json:"ratio"`
	Image            json.RawMessage   `json:"image"`
	Images           []json.RawMessage `json:"images"`
	FirstFrame       json.RawMessage   `json:"first_frame"`
	LastFrame        json.RawMessage   `json:"last_frame"`
	ReferenceImages  []json.RawMessage `json:"reference_images"`
	Video            json.RawMessage   `json:"video"`
	Videos           []json.RawMessage `json:"videos"`
	ReferenceVideos  []json.RawMessage `json:"reference_videos"`
	Audio            json.RawMessage   `json:"audio"`
	Audios           []json.RawMessage `json:"audios"`
	ReferenceAudios  []json.RawMessage `json:"reference_audios"`
	Media            []BaiduVODMedia   `json:"media"`
	Content          []json.RawMessage `json:"content"`
	GenerateAudio    *bool             `json:"generate_audio"`
	Watermark        *bool             `json:"watermark"`
	ReturnLastFrame  *bool             `json:"return_last_frame"`
	CallbackURL      string            `json:"callback_url"`
	ServiceTier      string            `json:"service_tier"`
	ExpiresAfter     *int              `json:"execution_expires_after"`
	Draft            *bool             `json:"draft"`
	Frames           *int              `json:"frames"`
	Seed             *int64            `json:"seed"`
	PersonGeneration string            `json:"person_generation"`
	CameraFixed      *bool             `json:"camera_fixed"`
	SafetyID         string            `json:"safety_identifier"`
	Priority         *int              `json:"priority"`
	Tools            []json.RawMessage `json:"tools"`
}

type BaiduVODVeoImage struct {
	ImageURL string `json:"imageUrl"`
}

type BaiduVODVeoGenerationMode string

const (
	BaiduVODVeoModeImage BaiduVODVeoGenerationMode = "image_to_video"
	BaiduVODVeoModeText  BaiduVODVeoGenerationMode = "text_to_video"
)

type BaiduVODVeoTaskInput struct {
	Prompt           string            `json:"prompt"`
	Image            *BaiduVODVeoImage `json:"image,omitempty"`
	LastFrame        *BaiduVODVeoImage `json:"lastFrame,omitempty"`
	ReferenceImages  *BaiduVODVeoImage `json:"referenceImages,omitempty"`
	N                int               `json:"n,omitempty"`
	AspectRatio      string            `json:"aspectRatio,omitempty"`
	DurationSeconds  int               `json:"durationSeconds,omitempty"`
	Resolution       string            `json:"resolution,omitempty"`
	NegativePrompt   string            `json:"negativePrompt,omitempty"`
	GenerateAudio    *bool             `json:"generateAudio,omitempty"`
	PersonGeneration string            `json:"personGeneration,omitempty"`
	Seed             *int64            `json:"seed,omitempty"`
}

type BaiduVODUpstreamRequest struct {
	Provider string `json:"-"`
	Model    string `json:"model"`
	Input    struct {
		Prompt string          `json:"prompt,omitempty"`
		Media  []BaiduVODMedia `json:"media,omitempty"`
	} `json:"input"`
	Parameters struct {
		Resolution string `json:"resolution"`
		Ratio      string `json:"ratio"`
		Duration   int    `json:"duration"`
	} `json:"parameters"`
	Content         []json.RawMessage         `json:"-"`
	Resolution      string                    `json:"-"`
	Ratio           string                    `json:"-"`
	Duration        int                       `json:"-"`
	GenerateAudio   *bool                     `json:"-"`
	Watermark       *bool                     `json:"-"`
	ReturnLastFrame *bool                     `json:"-"`
	CallbackURL     string                    `json:"-"`
	ServiceTier     string                    `json:"-"`
	ExpiresAfter    *int                      `json:"-"`
	Draft           *bool                     `json:"-"`
	Frames          *int                      `json:"-"`
	Seed            *int64                    `json:"-"`
	CameraFixed     *bool                     `json:"-"`
	SafetyID        string                    `json:"-"`
	Priority        *int                      `json:"-"`
	Tools           []json.RawMessage         `json:"-"`
	VeoInput        *BaiduVODVeoTaskInput     `json:"-"`
	VeoMode         BaiduVODVeoGenerationMode `json:"-"`
}

func (r BaiduVODUpstreamRequest) MarshalJSON() ([]byte, error) {
	if r.Provider == BaiduVODProviderVeo {
		type veoRequest struct {
			Model              string                `json:"model"`
			ModelVE31Input     *BaiduVODVeoTaskInput `json:"modelVE31TaskInput,omitempty"`
			ModelVE31FastInput *BaiduVODVeoTaskInput `json:"modelVE31FTaskInput,omitempty"`
			ModelVE31LiteInput *BaiduVODVeoTaskInput `json:"modelVE31LTaskInput,omitempty"`
		}
		payload := veoRequest{Model: r.Model}
		switch r.Model {
		case "VE3.1":
			payload.ModelVE31Input = r.VeoInput
		case "VE3.1F":
			payload.ModelVE31FastInput = r.VeoInput
		case "VE3.1L":
			payload.ModelVE31LiteInput = r.VeoInput
		default:
			return nil, fmt.Errorf("unsupported Baidu VOD Veo upstream model: %s", r.Model)
		}
		return json.Marshal(payload)
	}
	if r.Provider != BaiduVODProviderSeedance {
		type happyHorseRequest struct {
			Model      string `json:"model"`
			Input      any    `json:"input"`
			Parameters any    `json:"parameters"`
		}
		return json.Marshal(happyHorseRequest{Model: r.Model, Input: r.Input, Parameters: r.Parameters})
	}
	type seedanceRequest struct {
		Model           string            `json:"model"`
		Content         []json.RawMessage `json:"content"`
		Resolution      string            `json:"resolution,omitempty"`
		Ratio           string            `json:"ratio,omitempty"`
		Duration        int               `json:"duration,omitempty"`
		GenerateAudio   *bool             `json:"generate_audio,omitempty"`
		Watermark       *bool             `json:"watermark,omitempty"`
		ReturnLastFrame *bool             `json:"return_last_frame,omitempty"`
		CallbackURL     string            `json:"callback_url,omitempty"`
		ServiceTier     string            `json:"service_tier,omitempty"`
		ExpiresAfter    *int              `json:"execution_expires_after,omitempty"`
		Draft           *bool             `json:"draft,omitempty"`
		Frames          *int              `json:"frames,omitempty"`
		Seed            *int64            `json:"seed,omitempty"`
		CameraFixed     *bool             `json:"camera_fixed,omitempty"`
		SafetyID        string            `json:"safety_identifier,omitempty"`
		Priority        *int              `json:"priority,omitempty"`
		Tools           []json.RawMessage `json:"tools,omitempty"`
	}
	return json.Marshal(seedanceRequest{
		Model: r.Model, Content: r.Content, Resolution: strings.ToLower(r.Resolution), Ratio: r.Ratio, Duration: r.Duration,
		GenerateAudio: r.GenerateAudio, Watermark: r.Watermark, ReturnLastFrame: r.ReturnLastFrame,
		CallbackURL: r.CallbackURL, ServiceTier: r.ServiceTier, ExpiresAfter: r.ExpiresAfter, Draft: r.Draft,
		Frames: r.Frames, Seed: r.Seed, CameraFixed: r.CameraFixed, SafetyID: r.SafetyID, Priority: r.Priority, Tools: r.Tools,
	})
}

func ParseBaiduVODVideoRequest(body []byte) (BaiduVODVideoRequest, error) {
	var req BaiduVODVideoRequest
	if len(bytes.TrimSpace(body)) == 0 {
		return req, errors.New("request body is empty")
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, fmt.Errorf("invalid JSON body: %w", err)
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.NegativePrompt = strings.TrimSpace(req.NegativePrompt)
	req.PersonGeneration = strings.TrimSpace(req.PersonGeneration)
	if req.Duration == 0 {
		req.Duration = req.Seconds
	}
	spec, knownModel := BaiduVODModel(req.Model)
	defaultDuration, defaultResolution, defaultRatio := baiduVODDefaultDuration, "720P", "16:9"
	if knownModel {
		defaultDuration, defaultResolution, defaultRatio = spec.DefaultDuration, spec.DefaultResolution, spec.DefaultRatio
	}
	if req.Duration == 0 {
		req.Duration = defaultDuration
	}
	req.Resolution = normalizeBaiduVODResolution(req.Resolution, req.Size, defaultResolution)
	if req.Ratio == "" {
		req.Ratio = ratioFromBaiduVODSize(req.Size)
	}
	if req.Ratio == "" {
		req.Ratio = defaultRatio
	}
	return req, nil
}

func normalizeBaiduVODResolution(resolution, size, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(resolution))
	if strings.HasSuffix(value, "p") {
		value = strings.TrimSuffix(value, "p")
	}
	if value == "480" || value == "720" || value == "1080" {
		return value + "P"
	}
	if value == "4k" || value == "2160" {
		return "4K"
	}
	size = strings.ToLower(strings.TrimSpace(size))
	if strings.Contains(size, "1920x1080") || strings.Contains(size, "1080x1920") {
		return "1080P"
	}
	if strings.Contains(size, "3840x2160") || strings.Contains(size, "2160x3840") {
		return "4K"
	}
	if strings.TrimSpace(fallback) == "" {
		fallback = "720P"
	}
	return strings.ToUpper(strings.TrimSpace(fallback))
}

func ratioFromBaiduVODSize(size string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return ""
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return ""
	}
	if w*9 == h*16 {
		return "16:9"
	}
	if w*1 == h*1 {
		return "1:1"
	}
	if w*4 == h*3 {
		return "4:3"
	}
	return fmt.Sprintf("%d:%d", w/gcd(w, h), h/gcd(w, h))
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func baiduVODURL(raw json.RawMessage) string {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"url", "image_url", "imageUrl", "uri"} {
			if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func (r BaiduVODVideoRequest) mediaFor(spec BaiduVODModelSpec) ([]BaiduVODMedia, error) {
	media := make([]BaiduVODMedia, 0, 4)
	for _, item := range r.Media {
		item.Type, item.URL = strings.TrimSpace(item.Type), strings.TrimSpace(item.URL)
		if item.URL != "" {
			media = append(media, item)
		}
	}
	add := func(raw json.RawMessage, kind string) {
		if value := baiduVODURL(raw); value != "" {
			media = append(media, BaiduVODMedia{Type: kind, URL: value})
		}
	}
	switch spec.Capability {
	case BaiduVODCapabilityI2V:
		add(r.FirstFrame, "first_frame")
		if len(media) == 0 {
			add(r.Image, "first_frame")
		}
	case BaiduVODCapabilityR2V:
		for _, raw := range r.ReferenceImages {
			add(raw, "reference_image")
		}
		if len(media) == 0 {
			for _, raw := range r.Images {
				add(raw, "reference_image")
			}
		}
	case BaiduVODCapabilityEdit:
		add(r.Video, "video")
		add(r.Image, "reference_image")
	}
	if spec.Capability != BaiduVODCapabilityT2V && len(media) == 0 {
		return nil, fmt.Errorf("model %s requires input media", spec.Model)
	}
	return media, nil
}

func appendSeedanceContent(content *[]json.RawMessage, kind, rawURL, role string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	item := map[string]any{"type": kind, kind: map[string]string{"url": rawURL}}
	if role != "" {
		item["role"] = role
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	*content = append(*content, raw)
	return nil
}

func (r BaiduVODVideoRequest) seedanceContentFor(spec BaiduVODModelSpec) ([]json.RawMessage, error) {
	content := append([]json.RawMessage(nil), r.Content...)
	hasText := false
	for _, raw := range content {
		var item struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &item) == nil && strings.EqualFold(strings.TrimSpace(item.Type), "text") {
			hasText = true
		}
	}
	if r.Prompt != "" && !hasText {
		raw, err := json.Marshal(map[string]string{"type": "text", "text": r.Prompt})
		if err != nil {
			return nil, err
		}
		content = append([]json.RawMessage{raw}, content...)
	}
	add := func(raw json.RawMessage, kind, role string) error {
		return appendSeedanceContent(&content, kind, baiduVODURL(raw), role)
	}
	if err := add(r.FirstFrame, "image_url", "first_frame"); err != nil {
		return nil, err
	}
	if err := add(r.LastFrame, "image_url", "last_frame"); err != nil {
		return nil, err
	}
	if baiduVODURL(r.FirstFrame) == "" {
		if err := add(r.Image, "image_url", "first_frame"); err != nil {
			return nil, err
		}
	}
	for _, raw := range append(append([]json.RawMessage(nil), r.ReferenceImages...), r.Images...) {
		if err := add(raw, "image_url", "reference_image"); err != nil {
			return nil, err
		}
	}
	for _, raw := range append(append([]json.RawMessage{r.Video}, r.ReferenceVideos...), r.Videos...) {
		if err := add(raw, "video_url", "reference_video"); err != nil {
			return nil, err
		}
	}
	for _, raw := range append(append([]json.RawMessage{r.Audio}, r.ReferenceAudios...), r.Audios...) {
		if err := add(raw, "audio_url", "reference_audio"); err != nil {
			return nil, err
		}
	}
	for _, item := range r.Media {
		kind, role := seedanceMediaKind(item.Type, item.Role)
		if err := appendSeedanceContent(&content, kind, item.URL, role); err != nil {
			return nil, err
		}
	}
	if err := validateSeedanceContent(spec, content); err != nil {
		return nil, err
	}
	return content, nil
}

func seedanceMediaKind(kind, role string) (string, string) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	role = strings.ToLower(strings.TrimSpace(role))
	switch kind {
	case "first_frame", "last_frame", "reference_image", "image", "image_url":
		if role == "" && kind != "image" && kind != "image_url" {
			role = kind
		}
		if role == "" {
			role = "reference_image"
		}
		return "image_url", role
	case "video", "video_url", "reference_video":
		if role == "" {
			role = "reference_video"
		}
		return "video_url", role
	case "audio", "audio_url", "reference_audio":
		if role == "" {
			role = "reference_audio"
		}
		return "audio_url", role
	default:
		return kind, role
	}
}

func validateSeedanceContent(spec BaiduVODModelSpec, content []json.RawMessage) error {
	counts := map[string]int{}
	for _, raw := range content {
		var item struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &item); err != nil || strings.TrimSpace(item.Type) == "" {
			return errors.New("Seedance content entries require a type")
		}
		counts[strings.ToLower(strings.TrimSpace(item.Type))]++
	}
	mediaCount := counts["image_url"] + counts["video_url"]
	if counts["audio_url"] > 0 && mediaCount == 0 {
		return errors.New("Seedance audio content requires an image or video")
	}
	if len(content) == counts["audio_url"] {
		return errors.New("Seedance requires text, image, or video content")
	}
	if strings.Contains(spec.Model, "seedance-2-0") {
		if counts["image_url"] > 9 || counts["video_url"] > 3 || counts["audio_url"] > 3 {
			return errors.New("Seedance 2.0 supports up to 9 images, 3 videos, and 3 audio files")
		}
		return nil
	}
	if counts["video_url"] > 0 || counts["audio_url"] > 0 {
		return fmt.Errorf("model %s supports text and image content", spec.Model)
	}
	maxImages := 2
	if strings.Contains(spec.Model, "pro-fast") {
		maxImages = 1
	}
	if counts["image_url"] > maxImages {
		return fmt.Errorf("model %s supports up to %d image inputs", spec.Model, maxImages)
	}
	return nil
}

func estimateSeedanceCompletionTokens(req BaiduVODVideoRequest, spec BaiduVODModelSpec) int {
	content, err := req.seedanceContentFor(spec)
	if err != nil {
		return 0
	}
	videoCount := seedanceVideoInputCount(content)
	duration := req.Duration
	if duration == -1 {
		duration = spec.MaxDuration
	}
	if duration <= 0 {
		duration = spec.DefaultDuration
	}
	width, height := seedanceVideoDimensions(req.Resolution, req.Ratio)
	if width <= 0 || height <= 0 {
		return 0
	}
	frames := int64(duration * 24)
	if req.Frames != nil && *req.Frames > 0 {
		frames = int64(*req.Frames)
	}
	frames += int64(videoCount * baiduVODSeedanceMaxMediaSecs * 24)
	tokens := (int64(width)*int64(height)*frames + 1023) / 1024
	if tokens > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(tokens)
}

func seedanceInputContainsVideo(req BaiduVODVideoRequest, spec BaiduVODModelSpec) bool {
	content, err := req.seedanceContentFor(spec)
	return err == nil && seedanceVideoInputCount(content) > 0
}

func seedanceVideoInputCount(content []json.RawMessage) int {
	videoCount := 0
	for _, raw := range content {
		var item struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &item) == nil && strings.EqualFold(strings.TrimSpace(item.Type), "video_url") {
			videoCount++
		}
	}
	return videoCount
}

func seedanceVideoDimensions(resolution, ratio string) (int, int) {
	short, long := 720, 1280
	switch normalizeBaiduVODResolution(resolution, "", "720P") {
	case "480P":
		short, long = 480, 864
	case "1080P":
		short, long = 1080, 1920
	case "4K":
		short, long = 2160, 3840
	}
	switch strings.ToLower(strings.TrimSpace(ratio)) {
	case "9:16":
		return short, long
	case "3:4":
		return short, short * 4 / 3
	case "1:1":
		return short, short
	case "4:3":
		return short * 4 / 3, short
	case "21:9", "adaptive":
		return short * 21 / 9, short
	default:
		return long, short
	}
}

func baiduVODSupportsResolution(spec BaiduVODModelSpec, resolution string) bool {
	for _, supported := range spec.Resolutions {
		if strings.EqualFold(supported, resolution) {
			return true
		}
	}
	return false
}

func validateSeedanceParameters(req BaiduVODVideoRequest, spec BaiduVODModelSpec) error {
	allowedRatios := map[string]bool{"16:9": true, "4:3": true, "1:1": true, "3:4": true, "9:16": true, "21:9": true, "adaptive": true}
	if !allowedRatios[strings.ToLower(strings.TrimSpace(req.Ratio))] {
		return fmt.Errorf("unsupported Seedance ratio: %s", req.Ratio)
	}
	if req.ExpiresAfter != nil && (*req.ExpiresAfter < 3600 || *req.ExpiresAfter > 259200) {
		return errors.New("Seedance execution_expires_after must be between 3600 and 259200 seconds")
	}
	is20 := strings.Contains(spec.Model, "seedance-2-0")
	is15 := strings.Contains(spec.Model, "seedance-1-5")
	if is20 && strings.TrimSpace(req.ServiceTier) != "" && !strings.EqualFold(strings.TrimSpace(req.ServiceTier), "default") {
		return fmt.Errorf("model %s supports the default service tier", spec.Model)
	}
	if req.Frames != nil {
		if is20 || is15 {
			return fmt.Errorf("model %s does not support frames", spec.Model)
		}
		if *req.Frames < 29 || *req.Frames > 289 || (*req.Frames-25)%4 != 0 {
			return errors.New("Seedance frames must be between 29 and 289 and match 25 + 4n")
		}
	}
	if req.GenerateAudio != nil && !is20 && !is15 {
		return fmt.Errorf("model %s does not support generate_audio", spec.Model)
	}
	if req.Priority != nil {
		if !is20 {
			return fmt.Errorf("model %s does not support priority", spec.Model)
		}
		if *req.Priority < 0 || *req.Priority > 9 {
			return errors.New("Seedance priority must be between 0 and 9")
		}
	}
	if is20 && req.Seed != nil {
		return fmt.Errorf("model %s does not support seed", spec.Model)
	}
	if req.Seed != nil && (*req.Seed < -1 || *req.Seed > int64(^uint32(0))) {
		return errors.New("Seedance seed must be between -1 and 4294967295")
	}
	if is20 && req.CameraFixed != nil {
		return fmt.Errorf("model %s does not support camera_fixed", spec.Model)
	}
	if len(req.Tools) > 0 && !is20 {
		return fmt.Errorf("model %s does not support tools", spec.Model)
	}
	if req.Draft != nil && !is15 {
		return fmt.Errorf("model %s does not support draft mode", spec.Model)
	}
	if req.Draft != nil && *req.Draft {
		if !strings.EqualFold(req.Resolution, "480P") {
			return errors.New("Seedance draft mode requires 480P resolution")
		}
		if req.ReturnLastFrame != nil && *req.ReturnLastFrame {
			return errors.New("Seedance draft mode does not support return_last_frame")
		}
		if strings.EqualFold(strings.TrimSpace(req.ServiceTier), "flex") {
			return errors.New("Seedance draft mode does not support flex service tier")
		}
	}
	return nil
}

func veoImage(raw json.RawMessage) *BaiduVODVeoImage {
	imageURL := baiduVODURL(raw)
	if imageURL == "" {
		return nil
	}
	return &BaiduVODVeoImage{ImageURL: imageURL}
}

func veoTaskInputFor(req BaiduVODVideoRequest, spec BaiduVODModelSpec) (*BaiduVODVeoTaskInput, BaiduVODVeoGenerationMode, error) {
	if req.Prompt == "" {
		return nil, "", errors.New("prompt is required")
	}
	if len([]rune(req.Prompt)) > 2000 {
		return nil, "", errors.New("Veo prompt must not exceed 2000 characters")
	}
	if len([]rune(req.NegativePrompt)) > 1000 {
		return nil, "", errors.New("Veo negative_prompt must not exceed 1000 characters")
	}
	if req.Duration != 4 && req.Duration != 6 && req.Duration != 8 {
		return nil, "", fmt.Errorf("model %s duration must be 4, 6, or 8 seconds", spec.Model)
	}
	if req.Ratio != "16:9" && req.Ratio != "9:16" {
		return nil, "", fmt.Errorf("model %s supports 16:9 and 9:16 ratios", spec.Model)
	}
	if req.N != 0 && req.N != 1 {
		return nil, "", errors.New("Veo currently supports n=1 through this API")
	}
	if req.Seed != nil && (*req.Seed < 0 || *req.Seed > int64(^uint32(0))) {
		return nil, "", errors.New("Veo seed must be between 0 and 4294967295")
	}
	if req.PersonGeneration != "" && req.PersonGeneration != "allow_adult" && req.PersonGeneration != "disallow" {
		return nil, "", errors.New("Veo person_generation must be allow_adult or disallow")
	}

	firstFrame := veoImage(req.FirstFrame)
	if firstFrame == nil {
		firstFrame = veoImage(req.Image)
	}
	lastFrame := veoImage(req.LastFrame)
	references := req.ReferenceImages
	if len(references) == 0 {
		references = req.Images
	}
	var referenceImage *BaiduVODVeoImage
	for _, raw := range references {
		if image := veoImage(raw); image != nil {
			if referenceImage != nil {
				return nil, "", errors.New("Veo supports one reference image per request")
			}
			referenceImage = image
		}
	}
	if lastFrame != nil && firstFrame == nil {
		return nil, "", errors.New("Veo last_frame requires first_frame or image")
	}
	if referenceImage != nil && !spec.AllowReferences {
		return nil, "", fmt.Errorf("model %s does not support reference images", spec.Model)
	}
	if referenceImage != nil && (firstFrame != nil || lastFrame != nil) {
		return nil, "", errors.New("Veo reference image mode cannot include first_frame, image, or last_frame")
	}
	mode := BaiduVODVeoModeImage
	if firstFrame == nil && referenceImage == nil {
		if !spec.AllowText {
			return nil, "", fmt.Errorf("model %s requires an image input", spec.Model)
		}
		mode = BaiduVODVeoModeText
	}

	return &BaiduVODVeoTaskInput{
		Prompt: req.Prompt, Image: firstFrame, LastFrame: lastFrame, ReferenceImages: referenceImage,
		N: 1, AspectRatio: req.Ratio, DurationSeconds: req.Duration, Resolution: strings.ToLower(req.Resolution),
		NegativePrompt: req.NegativePrompt, GenerateAudio: req.GenerateAudio,
		PersonGeneration: req.PersonGeneration, Seed: req.Seed,
	}, mode, nil
}

func TranslateBaiduVODVideoRequest(req BaiduVODVideoRequest) (BaiduVODModelSpec, BaiduVODUpstreamRequest, error) {
	spec, ok := BaiduVODModel(req.Model)
	if !ok {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, fmt.Errorf("unsupported Baidu VOD model: %s", req.Model)
	}
	if req.Duration == 0 {
		req.Duration = spec.DefaultDuration
	}
	req.Resolution = normalizeBaiduVODResolution(req.Resolution, req.Size, spec.DefaultResolution)
	if req.Ratio == "" {
		req.Ratio = firstNonEmpty(ratioFromBaiduVODSize(req.Size), spec.DefaultRatio)
	}
	if !baiduVODSupportsResolution(spec, req.Resolution) {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, fmt.Errorf("model %s does not support resolution %s", spec.Model, req.Resolution)
	}
	if spec.MinDuration > 0 && req.Duration != -1 && (req.Duration < spec.MinDuration || req.Duration > spec.MaxDuration) {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, fmt.Errorf("model %s duration must be between %d and %d seconds", spec.Model, spec.MinDuration, spec.MaxDuration)
	}
	if req.Duration == -1 && !spec.AllowAutoDuration {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, fmt.Errorf("model %s does not support automatic duration", spec.Model)
	}
	var out BaiduVODUpstreamRequest
	out.Provider, out.Model = spec.Provider, spec.UpstreamModel
	if spec.Provider == BaiduVODProviderVeo {
		input, mode, err := veoTaskInputFor(req, spec)
		if err != nil {
			return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, err
		}
		out.VeoInput, out.VeoMode = input, mode
		if mode == BaiduVODVeoModeText {
			spec.Capability = BaiduVODCapabilityT2V
			spec.CreatePath = BaiduVODVeoTextCreatePath
		} else if input.ReferenceImages != nil {
			spec.Capability = BaiduVODCapabilityR2V
		}
		return spec, out, nil
	}
	if spec.Provider == BaiduVODProviderSeedance {
		if err := validateSeedanceParameters(req, spec); err != nil {
			return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, err
		}
		content, err := req.seedanceContentFor(spec)
		if err != nil {
			return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, err
		}
		out.Content, out.Resolution, out.Ratio, out.Duration = content, req.Resolution, req.Ratio, req.Duration
		out.GenerateAudio, out.Watermark, out.ReturnLastFrame = req.GenerateAudio, req.Watermark, req.ReturnLastFrame
		out.CallbackURL, out.ServiceTier, out.ExpiresAfter = strings.TrimSpace(req.CallbackURL), strings.TrimSpace(req.ServiceTier), req.ExpiresAfter
		out.Draft, out.Frames, out.Seed, out.CameraFixed = req.Draft, req.Frames, req.Seed, req.CameraFixed
		out.SafetyID, out.Priority, out.Tools = strings.TrimSpace(req.SafetyID), req.Priority, req.Tools
		return spec, out, nil
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, errors.New("prompt is required")
	}
	media, err := req.mediaFor(spec)
	if err != nil {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, err
	}
	out.Input.Prompt = req.Prompt
	out.Input.Media = media
	out.Parameters.Resolution = req.Resolution
	out.Parameters.Ratio = req.Ratio
	out.Parameters.Duration = req.Duration
	return spec, out, nil
}

type BaiduVODCreateResponse struct {
	Output struct {
		TaskStatus string `json:"task_status"`
		TaskID     string `json:"task_id"`
	} `json:"output"`
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type BaiduVODTaskUsage struct {
	Duration            int    `json:"duration"`
	InputVideoDuration  int    `json:"input_video_duration"`
	OutputVideoDuration int    `json:"output_video_duration"`
	VideoCount          int    `json:"video_count"`
	SR                  int    `json:"SR"`
	Ratio               string `json:"ratio"`
	Resolution          string `json:"resolution"`
	CompletionTokens    int    `json:"completion_tokens"`
	TotalTokens         int    `json:"total_tokens"`
}

type BaiduVODTaskResponse struct {
	Provider  string `json:"-"`
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Output    struct {
		TaskID        string `json:"task_id"`
		TaskStatus    string `json:"task_status"`
		Code          string `json:"code"`
		Message       string `json:"message"`
		VideoURL      string `json:"video_url"`
		SubmitTime    string `json:"submit_time"`
		ScheduledTime string `json:"scheduled_time"`
		EndTime       string `json:"end_time"`
	} `json:"output"`
	Usage *BaiduVODTaskUsage `json:"usage"`
}

type baiduVODSeedanceError struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

type baiduVODSeedanceCreateResponse struct {
	ID        string                 `json:"id"`
	RequestID string                 `json:"request_id"`
	Code      json.RawMessage        `json:"code"`
	Message   string                 `json:"message"`
	Error     *baiduVODSeedanceError `json:"error"`
}

type baiduVODSeedanceTaskResponse struct {
	ID        string                 `json:"id"`
	Model     string                 `json:"model"`
	Status    string                 `json:"status"`
	RequestID string                 `json:"request_id"`
	Code      json.RawMessage        `json:"code"`
	Message   string                 `json:"message"`
	Error     *baiduVODSeedanceError `json:"error"`
	Content   struct {
		VideoURL     string `json:"video_url"`
		LastFrameURL string `json:"last_frame_url"`
		FileURL      string `json:"file_url"`
	} `json:"content"`
	Usage *struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Duration   int    `json:"duration"`
	Ratio      string `json:"ratio"`
	Resolution string `json:"resolution"`
}

type baiduVODVeoCreateResponse struct {
	TaskID    string `json:"taskId"`
	RequestID string `json:"requestId"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type baiduVODVeoMediaInfo struct {
	Source struct {
		SourceURL string `json:"sourceUrl"`
	} `json:"source"`
	SourceMetadata struct {
		DurationInSecond int `json:"durationInSecond"`
		Video            struct {
			WidthInPixel  int `json:"widthInPixel"`
			HeightInPixel int `json:"heightInPixel"`
		} `json:"video"`
	} `json:"sourceMetadata"`
}

type baiduVODVeoTaskResponse struct {
	TaskID                string `json:"taskId"`
	Type                  string `json:"type"`
	Status                string `json:"status"`
	RequestID             string `json:"requestId"`
	Code                  string `json:"code"`
	Message               string `json:"message"`
	VideoGenerateTaskInfo struct {
		Status                  string  `json:"status"`
		ErrMsg                  string  `json:"errMsg"`
		UnitPrice               float64 `json:"unitPrice"`
		VideoGenerateTaskOutput struct {
			MediaBasicInfos []baiduVODVeoMediaInfo `json:"mediaBasicInfos"`
		} `json:"videoGenerateTaskOutput"`
	} `json:"videoGenerateTaskInfo"`
}

func baiduVODJSONCode(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == `""` {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	return trimmed
}

func parseBaiduVODTime(value string) *time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, strings.TrimSpace(value), time.Local); err == nil {
			return &parsed
		}
	}
	return nil
}

func ParseBaiduVODVideoURLExpiry(rawURL string) *time.Time {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}
	expires := firstNonEmpty(parsed.Query().Get("Expires"), parsed.Query().Get("expires"))
	if value, err := strconv.ParseInt(expires, 10, 64); err == nil && value > 0 {
		t := time.Unix(value, 0)
		return &t
	}
	return nil
}

// BCEAuthV1 signs a request with Baidu BCE's bce-auth-v1 scheme.
func BCEAuthV1(req *http.Request, accessKey, secretKey string, now time.Time, expiration time.Duration) error {
	if req == nil || strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return errors.New("BCE credentials and request are required")
	}
	if expiration <= 0 {
		expiration = 1800 * time.Second
	}
	timestamp := now.UTC().Format("2006-01-02T15:04:05Z")
	req.Header.Set("x-bce-date", timestamp)
	parsed, err := url.Parse(req.URL.String())
	if err != nil {
		return err
	}
	canonicalURI := canonicalBCEURI(parsed.EscapedPath())
	canonicalQuery := canonicalBCEQuery(parsed.Query())
	signedHeaders := []string{"host", "x-bce-date"}
	canonicalHeaders := "host:" + canonicalBCEValue(req.Host)
	if canonicalHeaders == "host:" {
		canonicalHeaders = "host:" + canonicalBCEValue(req.URL.Host)
	}
	canonicalHeaders += "\nx-bce-date:" + canonicalBCEValue(timestamp)
	canonicalRequest := strings.Join([]string{req.Method, canonicalURI, canonicalQuery, canonicalHeaders}, "\n")
	authPrefix := fmt.Sprintf("bce-auth-v1/%s/%s/%d", accessKey, timestamp, int(expiration/time.Second))
	signingKey := hmacSHA256Hex([]byte(secretKey), authPrefix)
	signature := hmacSHA256Hex([]byte(signingKey), canonicalRequest)
	req.Header.Set("Authorization", authPrefix+"/"+strings.Join(signedHeaders, ";")+"/"+signature)
	return nil
}

func hmacSHA256Hex(key []byte, value string) string {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}

func canonicalBCEURI(path string) string {
	if path == "" {
		return "/"
	}
	decoded, err := url.PathUnescape(path)
	if err != nil {
		decoded = path
	}
	return bceURIEncode(decoded, true)
}

func canonicalBCEValue(value string) string {
	return bceURIEncode(strings.TrimSpace(value), false)
}

func canonicalBCEQuery(query url.Values) string {
	parts := make([]string, 0, len(query))
	for key, queryValues := range query {
		if strings.EqualFold(key, "authorization") {
			continue
		}
		values := append([]string(nil), queryValues...)
		sort.Strings(values)
		for _, value := range values {
			parts = append(parts, canonicalBCEValue(key)+"="+canonicalBCEValue(value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "&")
}

func bceURIEncode(value string, preserveSlash bool) string {
	const hexChars = "0123456789ABCDEF"
	var out strings.Builder
	for _, b := range []byte(value) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '~' || (preserveSlash && b == '/') {
			out.WriteByte(b)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(hexChars[b>>4])
		out.WriteByte(hexChars[b&15])
	}
	return out.String()
}
