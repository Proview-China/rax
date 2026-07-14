package operation

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"strings"
)

var knownKinds = map[Kind]struct{}{
	EmbeddingCreate: {}, RerankCreate: {}, ModerationCreate: {}, ImageGenerate: {}, ImageEdit: {}, ImageVariation: {},
	VideoGenerate: {}, VideoEdit: {}, VideoExtend: {}, VideoRemix: {}, AudioTranscribe: {}, AudioTranslate: {},
	VideoGet: {}, VideoDelete: {}, VideoContent: {},
	SpeechGenerate: {}, MusicGenerate: {}, TokenCount: {}, BatchCreate: {}, BatchList: {}, BatchGet: {}, BatchCancel: {}, BatchDelete: {}, BatchResults: {},
	FileCreate: {}, FileList: {}, FileGet: {}, FileDelete: {}, FileContent: {}, StoreCreate: {}, StoreList: {}, StoreGet: {},
	StoreDelete: {}, StoreSearch: {},
}

func (r Request) Validate() error {
	if strings.TrimSpace(string(r.Provider)) == "" {
		return fmt.Errorf("operation provider is required")
	}
	if _, ok := knownKinds[r.Kind]; !ok {
		return fmt.Errorf("operation kind %q is unknown", r.Kind)
	}
	if r.Budget.Timeout < 0 || r.Budget.MaxResponseBytes < 0 {
		return fmt.Errorf("operation budget values must not be negative")
	}
	if r.ContentType != "" {
		if _, _, err := mime.ParseMediaType(r.ContentType); err != nil {
			return fmt.Errorf("operation content type is invalid")
		}
	}
	if strings.ContainsAny(r.ResourceID+r.ParentID+r.IdempotencyKey, "\r\n\x00") {
		return fmt.Errorf("operation identifiers must be single-line")
	}
	for key, values := range r.Query {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n\x00") {
			return fmt.Errorf("operation query key is invalid")
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("operation query value is invalid")
			}
		}
	}
	for key := range r.Metadata {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("operation metadata keys must be non-empty")
		}
	}
	for provider, raw := range r.ProviderOptions {
		if strings.TrimSpace(string(provider)) == "" || !json.Valid(raw) {
			return fmt.Errorf("operation provider options must use a provider ID and valid JSON")
		}
	}
	if r.Body.Len() > 0 && strings.HasPrefix(strings.ToLower(r.ContentType), "application/json") && !json.Valid(r.Body.Bytes()) {
		return fmt.Errorf("operation JSON body is invalid")
	}
	return nil
}

func ValidateArtifact(a Artifact) error {
	if a.Kind == "" {
		return fmt.Errorf("artifact kind is required")
	}
	sources := 0
	if len(a.Data) > 0 {
		sources++
	}
	if a.URL != "" {
		sources++
	}
	if a.ResourceID != "" {
		sources++
	}
	if sources > 1 {
		return fmt.Errorf("artifact must use at most one content source")
	}
	if a.URL != "" {
		u, err := url.Parse(a.URL)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return fmt.Errorf("artifact URL is invalid")
		}
	}
	if a.SizeBytes < 0 {
		return fmt.Errorf("artifact size must not be negative")
	}
	return nil
}
