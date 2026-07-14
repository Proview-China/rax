package realtime

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Validate checks the provider-neutral session intent before any transport is
// selected. Provider implementations may add stricter native constraints.
func (request Request) Validate() error {
	if strings.TrimSpace(string(request.Provider)) == "" {
		return fmt.Errorf("realtime provider is required")
	}
	if request.Model == "" || request.Model != strings.TrimSpace(request.Model) || len(request.Model) > 512 || strings.ContainsAny(request.Model, "\r\n\x00") {
		return fmt.Errorf("realtime model is invalid")
	}
	if request.Timeout < 0 {
		return fmt.Errorf("realtime timeout must not be negative")
	}
	if request.Configuration.Len() > 0 && !json.Valid(request.Configuration.Bytes()) {
		return fmt.Errorf("realtime configuration must be valid JSON")
	}
	seen := make(map[Modality]struct{}, len(request.Modalities))
	for _, modality := range request.Modalities {
		switch modality {
		case Text, Audio, Video:
		default:
			return fmt.Errorf("realtime modality %q is invalid", modality)
		}
		if _, exists := seen[modality]; exists {
			return fmt.Errorf("realtime modality %q is duplicated", modality)
		}
		seen[modality] = struct{}{}
	}
	for key := range request.Metadata {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("realtime metadata keys must be non-empty")
		}
	}
	for provider, raw := range request.ProviderOptions {
		if provider != request.Provider || !json.Valid(raw) {
			return fmt.Errorf("realtime provider options must use the selected provider ID and valid JSON")
		}
	}
	return nil
}

func validateClientEvent(event ClientEvent) error {
	sources := 0
	if len(event.Binary) > 0 {
		sources++
	}
	if event.Raw.Len() > 0 {
		sources++
		if !json.Valid(event.Raw.Bytes()) {
			return fmt.Errorf("realtime raw event must be valid JSON")
		}
	}
	if event.Type != "" || event.Text != "" {
		sources++
	}
	if sources == 0 {
		return fmt.Errorf("realtime client event is empty")
	}
	if sources > 1 {
		return fmt.Errorf("realtime client event payload forms are mutually exclusive")
	}
	if strings.ContainsAny(event.Type, "\r\n\x00") {
		return fmt.Errorf("realtime client event type is invalid")
	}
	return nil
}
