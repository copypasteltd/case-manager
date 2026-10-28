package storageconfig

import (
	"errors"
	"os"
	"strings"

	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
)

func LoadMultipartCleanupTargets() (map[string]*files.CleanupBackend, error) {
	out := map[string]*files.CleanupBackend{}
	list := os.Getenv("CP_MULTIPART_CLEANUP_PROVIDERS")
	if list == "" || list == "disabled" {
		return out, nil
	}
	for _, provider := range strings.Split(list, ",") {
		provider = strings.TrimSpace(provider)
		if (provider != "oss" && provider != "r2") || out[provider] != nil {
			return nil, errors.New("CP_MULTIPART_CLEANUP_PROVIDERS requires unique oss,r2 targets")
		}
		prefix := "CP_" + strings.ToUpper(provider) + "_"
		settings := objectstore.Settings{Endpoint: os.Getenv(prefix + "ENDPOINT"), Region: os.Getenv(prefix + "REGION"), Bucket: os.Getenv(prefix + "BUCKET")}
		binding, e := objectstore.LocationFingerprint(provider, settings)
		if e != nil {
			return nil, errors.New("multipart cleanup target requires an explicit valid original location")
		}
		out[provider] = &files.CleanupBackend{Provider: provider, Bucket: settings.Bucket, BindingSHA256: binding}
	}
	return out, nil
}
func LoadMultipartCleaners() (map[string]*files.CleanupBackend, error) {
	out, e := LoadMultipartCleanupTargets()
	if e != nil {
		return nil, e
	}
	for provider, b := range out {
		store, settings, e := roleStore(provider, "MULTIPART_CLEANER")
		if e != nil {
			return nil, e
		}
		cleaner, ok := store.(files.MultipartCleaner)
		if !ok {
			return nil, errors.New("multipart inspection is unavailable")
		}
		binding, e := objectstore.LocationFingerprint(provider, settings)
		if e != nil || binding != b.BindingSHA256 {
			return nil, errors.New("multipart cleaner location changed")
		}
		b.Cleaner = cleaner
	}
	return out, nil
}
