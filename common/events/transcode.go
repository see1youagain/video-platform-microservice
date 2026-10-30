package events

import (
	"encoding/hex"
	"fmt"
)

func ValidTaskID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// ValidateTranscodeRequest restricts object names and the supported encoding profiles.
// Duplicate profiles are removed while preserving the requested order.
func ValidateTranscodeRequest(fileHash string, resolutions []string) ([]string, error) {
	if len(fileHash) != 32 && len(fileHash) != 64 {
		return nil, fmt.Errorf("file_hash must be an MD5 or SHA256 hex digest")
	}
	if _, err := hex.DecodeString(fileHash); err != nil {
		return nil, fmt.Errorf("invalid file_hash: %w", err)
	}
	if len(resolutions) == 0 || len(resolutions) > 16 {
		return nil, fmt.Errorf("resolutions must contain 1 to 16 profiles")
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(resolutions))
	for _, name := range resolutions {
		switch name {
		case "360p", "480p", "720p", "1080p":
		default:
			return nil, fmt.Errorf("unsupported resolution: %q", name)
		}
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result, nil
}
