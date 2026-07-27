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
	BaiduVODDefaultBaseURL = "https://vod.bj.baidubce.com"
	BaiduVODCreatePath     = "/api/v1/services/aigc/video-generation/video-synthesis"
	BaiduVODTaskPath       = "/api/v1/tasks/"
	BaiduVODTaskStatusPath = "/tasks/"
)

type BaiduVODVideoCapability string

const (
	BaiduVODCapabilityT2V  BaiduVODVideoCapability = "t2v"
	BaiduVODCapabilityI2V  BaiduVODVideoCapability = "i2v"
	BaiduVODCapabilityR2V  BaiduVODVideoCapability = "r2v"
	BaiduVODCapabilityEdit BaiduVODVideoCapability = "video_edit"
	BaiduVODProvider       string                  = "happyhorse"
	BaiduVODAuthModeAPIKey string                  = "apikey"
	BaiduVODAuthModeAKSK   string                  = "aksk"
)

type BaiduVODModelSpec struct {
	Model         string
	UpstreamModel string
	Capability    BaiduVODVideoCapability
}

var baiduVODModelRegistry = map[string]BaiduVODModelSpec{
	"happyhorse-1.0-t2v":        {Model: "happyhorse-1.0-t2v", UpstreamModel: "happyhorse-1.0-t2v", Capability: BaiduVODCapabilityT2V},
	"happyhorse-1.0-i2v":        {Model: "happyhorse-1.0-i2v", UpstreamModel: "happyhorse-1.0-i2v", Capability: BaiduVODCapabilityI2V},
	"happyhorse-1.0-r2v":        {Model: "happyhorse-1.0-r2v", UpstreamModel: "happyhorse-1.0-r2v", Capability: BaiduVODCapabilityR2V},
	"happyhorse-1.0-video-edit": {Model: "happyhorse-1.0-video-edit", UpstreamModel: "happyhorse-1.0-video-edit", Capability: BaiduVODCapabilityEdit},
	"happyhorse-1.1-t2v":        {Model: "happyhorse-1.1-t2v", UpstreamModel: "happyhorse-1.1-t2v", Capability: BaiduVODCapabilityT2V},
	"happyhorse-1.1-i2v":        {Model: "happyhorse-1.1-i2v", UpstreamModel: "happyhorse-1.1-i2v", Capability: BaiduVODCapabilityI2V},
	"happyhorse-1.1-r2v":        {Model: "happyhorse-1.1-r2v", UpstreamModel: "happyhorse-1.1-r2v", Capability: BaiduVODCapabilityR2V},
	"happyhorse-1.1-video-edit": {Model: "happyhorse-1.1-video-edit", UpstreamModel: "happyhorse-1.1-video-edit", Capability: BaiduVODCapabilityEdit},
}

func BaiduVODModel(model string) (BaiduVODModelSpec, bool) {
	spec, ok := baiduVODModelRegistry[strings.ToLower(strings.TrimSpace(model))]
	return spec, ok
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
}

type BaiduVODVideoRequest struct {
	Model           string            `json:"model"`
	Prompt          string            `json:"prompt"`
	Seconds         int               `json:"seconds"`
	Duration        int               `json:"duration"`
	Size            string            `json:"size"`
	Resolution      string            `json:"resolution"`
	Ratio           string            `json:"ratio"`
	Image           json.RawMessage   `json:"image"`
	Images          []json.RawMessage `json:"images"`
	FirstFrame      json.RawMessage   `json:"first_frame"`
	ReferenceImages []json.RawMessage `json:"reference_images"`
	Video           json.RawMessage   `json:"video"`
	Media           []BaiduVODMedia   `json:"media"`
}

type BaiduVODUpstreamRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt string          `json:"prompt,omitempty"`
		Media  []BaiduVODMedia `json:"media,omitempty"`
	} `json:"input"`
	Parameters struct {
		Resolution string `json:"resolution"`
		Ratio      string `json:"ratio"`
		Duration   int    `json:"duration"`
	} `json:"parameters"`
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
	if req.Duration <= 0 {
		req.Duration = req.Seconds
	}
	if req.Duration <= 0 {
		req.Duration = 5
	}
	req.Resolution = normalizeBaiduVODResolution(req.Resolution, req.Size)
	if req.Ratio == "" {
		req.Ratio = ratioFromBaiduVODSize(req.Size)
	}
	if req.Ratio == "" {
		req.Ratio = "16:9"
	}
	return req, nil
}

func normalizeBaiduVODResolution(resolution, size string) string {
	value := strings.ToLower(strings.TrimSpace(resolution))
	if strings.HasSuffix(value, "p") {
		value = strings.TrimSuffix(value, "p")
	}
	if value == "480" || value == "720" || value == "1080" {
		return value + "P"
	}
	size = strings.ToLower(strings.TrimSpace(size))
	if strings.Contains(size, "1920x1080") || strings.Contains(size, "1080x1920") {
		return "1080P"
	}
	return "720P"
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
		for _, key := range []string{"url", "image_url", "uri"} {
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

func TranslateBaiduVODVideoRequest(req BaiduVODVideoRequest) (BaiduVODModelSpec, BaiduVODUpstreamRequest, error) {
	spec, ok := BaiduVODModel(req.Model)
	if !ok {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, fmt.Errorf("unsupported HappyHorse model: %s", req.Model)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, errors.New("prompt is required")
	}
	if req.Resolution != "720P" && req.Resolution != "1080P" {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, fmt.Errorf("unsupported HappyHorse resolution: %s", req.Resolution)
	}
	media, err := req.mediaFor(spec)
	if err != nil {
		return BaiduVODModelSpec{}, BaiduVODUpstreamRequest{}, err
	}
	var out BaiduVODUpstreamRequest
	out.Model = spec.UpstreamModel
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

type BaiduVODTaskResponse struct {
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
	Usage *struct {
		Duration            int    `json:"duration"`
		InputVideoDuration  int    `json:"input_video_duration"`
		OutputVideoDuration int    `json:"output_video_duration"`
		VideoCount          int    `json:"video_count"`
		SR                  int    `json:"SR"`
		Ratio               string `json:"ratio"`
	} `json:"usage"`
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
