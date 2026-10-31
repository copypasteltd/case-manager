package storageconfig

import (
	"errors"
	"os"
	"strings"

	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
)

func LoadOriginalCleanupTargets() (map[string]*files.OriginalCleanupBackend, error) {
	out := map[string]*files.OriginalCleanupBackend{}
	raw := os.Getenv("CP_ORIGINAL_CLEANUP_PROVIDERS")
	if raw == "" || raw == "disabled" {
		return out, nil
	}
	exclusive := os.Getenv("CP_R2_CLEANUP_EXCLUSIVE_NAMESPACE")
	if exclusive != "" && exclusive != "true" && exclusive != "false" {
		return nil, errors.New("CP_R2_CLEANUP_EXCLUSIVE_NAMESPACE must be explicit true/false")
	}
	for _, provider := range strings.Split(raw, ",") {
		provider = strings.TrimSpace(provider)
		if provider != "oss" && provider != "r2" || out[provider] != nil {
			return nil, errors.New("CP_ORIGINAL_CLEANUP_PROVIDERS requires unique oss,r2")
		}
		prefix := "CP_" + strings.ToUpper(provider) + "_"
		settings := objectstore.Settings{Endpoint: os.Getenv(prefix + "ENDPOINT"), Region: os.Getenv(prefix + "REGION"), Bucket: os.Getenv(prefix + "BUCKET")}
		binding, e := objectstore.LocationFingerprint(provider, settings)
		if e != nil {
			return nil, errors.New("original cleanup requires explicit original storage location")
		}
		out[provider] = &files.OriginalCleanupBackend{Provider: provider, Bucket: settings.Bucket, BindingSHA256: binding, ExclusiveNamespace: provider == "r2" && exclusive == "true"}
	}
	return out, nil
}
func LoadOriginalCleaners() (map[string]*files.OriginalCleanupBackend, error) {
	out, e := LoadOriginalCleanupTargets()
	if e != nil {
		return nil, e
	}
	for provider, b := range out {
		if provider == "r2" && !b.ExclusiveNamespace {
			return nil, errors.New("R2 cleanup requires attested exclusive immutable managed namespace; no external writers or automatic lifecycle deletion")
		}
		store, settings, e := roleStore(provider, "ORIGINAL_CLEANER")
		if e != nil {
			return nil, e
		}
		cleaner, ok := store.(files.OriginalCleaner)
		if !ok {
			return nil, errors.New("known-original cleanup unavailable")
		}
		binding, e := objectstore.LocationFingerprint(provider, settings)
		if e != nil || binding != b.BindingSHA256 {
			return nil, errors.New("original cleanup location changed")
		}
		b.Cleaner = cleaner
	}
	return out, nil
}
