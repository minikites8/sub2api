package requestmodel

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/tidwall/gjson"
)

var ErrAmbiguousModel = errors.New("model and session fields must be unique and use canonical names")

// ValidateBodyModels requires unique, canonical routing fields. Even identical
// duplicates become ambiguous when a route rewrites only one occurrence.
func ValidateBodyModels(contentType string, body []byte) error {
	if isMultipartContentType(contentType) {
		return validateMultipartModels(contentType, body)
	}
	return ValidateJSONModels(body)
}

// ValidateJSONModels checks routing fields at the root and in session objects.
// Other payload objects can contain application data with their own model keys.
// Invalid JSON remains the responsibility of the protocol handler.
func ValidateJSONModels(body []byte) error {
	topCount, sessionModelCount := 0, 0
	sessionCount := 0
	conflict := false
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		switch {
		case strings.EqualFold(key.String(), "model"):
			topCount++
			conflict = conflict || key.String() != "model"
		case strings.EqualFold(key.String(), "session"):
			sessionCount++
			conflict = conflict || key.String() != "session"
			value.ForEach(func(key, value gjson.Result) bool {
				if strings.EqualFold(key.String(), "model") {
					sessionModelCount++
					conflict = conflict || key.String() != "model"
				}
				return true
			})
		}
		return true
	})
	if conflict || topCount > 1 || sessionModelCount > 1 || sessionCount > 1 {
		return ErrAmbiguousModel
	}
	return nil
}

func validateMultipartModels(contentType string, body []byte) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		return nil // the handler validates malformed multipart input
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	modelCount, sessionCount := 0, 0
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil
		}
		name := part.FormName()
		if part.FileName() != "" || (!strings.EqualFold(name, "model") && !strings.EqualFold(name, "session")) {
			continue
		}
		if name != "model" && name != "session" {
			return ErrAmbiguousModel
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return nil
		}
		if name == "model" {
			modelCount++
			if modelCount > 1 {
				return ErrAmbiguousModel
			}
		} else {
			sessionCount++
			if sessionCount > 1 {
				return ErrAmbiguousModel
			}
			if err := ValidateJSONModels(data); err != nil {
				return err
			}
		}
	}
}
