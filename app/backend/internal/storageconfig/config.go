// Package storageconfig binds explicit process roles to cloud identities.
// It never falls back to shared SDK credentials or a different role's key.
package storageconfig

import (
	"errors"
	"os"
	"strings"

	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
)

func driver() (string, error) {
	value := os.Getenv("CP_STORAGE_DRIVER")
	if value == "" || value == "disabled" {
		return "", nil
	}
	if value != "oss" && value != "r2" {
		return "", errors.New("CP_STORAGE_DRIVER must be disabled, oss or r2")
	}
	return value, nil
}
func roleStore(provider, role string) (objectstore.Store, objectstore.Settings, error) {
	prefix := "CP_" + strings.ToUpper(provider) + "_"
	credentials := prefix + role + "_"
	settings := objectstore.Settings{Endpoint: os.Getenv(prefix + "ENDPOINT"), Region: os.Getenv(prefix + "REGION"), Bucket: os.Getenv(prefix + "BUCKET"),
		AccessKeyID: os.Getenv(credentials + "ACCESS_KEY_ID"), AccessKeySecret: os.Getenv(credentials + "ACCESS_KEY_SECRET"), SessionToken: os.Getenv(credentials + "SESSION_TOKEN")}
	for _, value := range []string{settings.AccessKeyID, settings.AccessKeySecret} {
		if value == "" || strings.HasPrefix(value, "CHANGE_") || strings.HasPrefix(value, "GENERATE_") {
			return nil, objectstore.Settings{}, errors.New("explicit cloud credentials for the selected process role are required")
		}
	}
	var store objectstore.Store
	var err error
	if provider == "oss" {
		store, err = objectstore.NewOSS(settings)
	} else {
		store, err = objectstore.NewR2(settings)
	}
	if err != nil {
		return nil, objectstore.Settings{}, errors.New("invalid cloud endpoint, region, bucket or credentials")
	}
	return store, settings, nil
}
func LoadAPI() (*files.UploadBackend, error) {
	provider, err := driver()
	if err != nil || provider == "" {
		return nil, err
	}
	signer, settings, err := roleStore(provider, "UPLOAD_SIGNER")
	if err != nil {
		return nil, err
	}
	inspector, _, err := roleStore(provider, "UPLOAD_INSPECTOR")
	if err != nil {
		return nil, err
	}
	return &files.UploadBackend{Provider: provider, Bucket: settings.Bucket, Signer: signer, Lister: inspector}, nil
}

// Downloads are independent of the active write provider. Only explicitly
// listed providers are readable; signing and streaming identities are separate.
func LoadDownloads() (map[string]*files.DownloadBackend, error) {
	out := map[string]*files.DownloadBackend{}
	list := os.Getenv("CP_DOWNLOAD_PROVIDERS")
	if list == "" || list == "disabled" {
		return out, nil
	}
	for _, provider := range strings.Split(list, ",") {
		provider = strings.TrimSpace(provider)
		if (provider != "oss" && provider != "r2") || out[provider] != nil {
			return nil, errors.New("CP_DOWNLOAD_PROVIDERS must be disabled or a unique comma separated list of oss,r2")
		}
		signer, settings, err := roleStore(provider, "DOWNLOAD_SIGNER")
		if err != nil {
			return nil, err
		}
		reader, _, err := roleStore(provider, "DOWNLOAD_READER")
		if err != nil {
			return nil, err
		}
		downloadSigner, ok := signer.(objectstore.DownloadSigner)
		if !ok {
			return nil, errors.New("selected provider cannot sign fixed source reads")
		}
		out[provider] = &files.DownloadBackend{Provider: provider, Bucket: settings.Bucket, Signer: downloadSigner, Reader: reader}
	}
	return out, nil
}

// AI reads are deliberately separate from ordinary human downloads. A download
// signer, upload credential or ambient SDK identity never enables this role.
func LoadAgentTextReaders() (map[string]*files.AgentTextBackend, error) {
	out := map[string]*files.AgentTextBackend{}
	list := os.Getenv("CP_AGENT_FILE_PROVIDERS")
	if list == "" || list == "disabled" {
		return out, nil
	}
	for _, provider := range strings.Split(list, ",") {
		provider = strings.TrimSpace(provider)
		if (provider != "oss" && provider != "r2") || out[provider] != nil {
			return nil, errors.New("CP_AGENT_FILE_PROVIDERS must be disabled or a unique comma separated list of oss,r2")
		}
		reader, settings, err := roleStore(provider, "AGENT_FILE_READER")
		if err != nil {
			return nil, err
		}
		out[provider] = &files.AgentTextBackend{Provider: provider, Bucket: settings.Bucket, Reader: reader}
	}
	return out, nil
}

// File indexing never borrows a human, model or upload identity.
func LoadTextIndexReaders() (map[string]*files.AgentTextBackend, error) {
	out := map[string]*files.AgentTextBackend{}
	list := os.Getenv("CP_TEXT_INDEX_PROVIDERS")
	if list == "" || list == "disabled" {
		return out, nil
	}
	for _, provider := range strings.Split(list, ",") {
		provider = strings.TrimSpace(provider)
		if (provider != "oss" && provider != "r2") || out[provider] != nil {
			return nil, errors.New("CP_TEXT_INDEX_PROVIDERS must be disabled or a unique comma separated list of oss,r2")
		}
		reader, settings, err := roleStore(provider, "TEXT_INDEX_READER")
		if err != nil {
			return nil, err
		}
		out[provider] = &files.AgentTextBackend{Provider: provider, Bucket: settings.Bucket, Reader: reader}
	}
	return out, nil
}

func LoadAgentDraftWriter() (*files.AgentDraftBackend, error) {
	provider := os.Getenv("CP_AGENT_DRAFT_DRIVER")
	if provider == "" || provider == "disabled" {
		return nil, nil
	}
	if provider != "oss" && provider != "r2" {
		return nil, errors.New("CP_AGENT_DRAFT_DRIVER must be disabled, oss or r2")
	}
	writer, settings, err := roleStore(provider, "AGENT_DRAFT_WRITER")
	if err != nil {
		return nil, err
	}
	return &files.AgentDraftBackend{Provider: provider, Bucket: settings.Bucket, Writer: writer}, nil
}
func LoadUploadWorker() (*files.UploadBackend, objectstore.Store, error) {
	provider, err := driver()
	if err != nil {
		return nil, nil, err
	}
	if provider == "" {
		return nil, nil, errors.New("upload worker requires an explicitly enabled storage driver")
	}
	store, settings, err := roleStore(provider, "UPLOAD_LIFECYCLE")
	if err != nil {
		return nil, nil, err
	}
	return &files.UploadBackend{Provider: provider, Bucket: settings.Bucket}, store, nil
}
func LoadVerificationWorker() (*files.UploadBackend, objectstore.Store, error) {
	provider, err := driver()
	if err != nil {
		return nil, nil, err
	}
	if provider == "" {
		return nil, nil, errors.New("verification worker requires an explicitly enabled storage driver")
	}
	store, settings, err := roleStore(provider, "UPLOAD_VERIFY")
	if err != nil {
		return nil, nil, err
	}
	return &files.UploadBackend{Provider: provider, Bucket: settings.Bucket}, store, nil
}
func LoadPromotionWorker() (*files.UploadBackend, objectstore.Store, objectstore.Store, error) {
	provider, err := driver()
	if err != nil {
		return nil, nil, nil, err
	}
	if provider == "" {
		return nil, nil, nil, errors.New("promotion requires an explicitly enabled storage driver")
	}
	source, settings, err := roleStore(provider, "PROMOTE_SOURCE")
	if err != nil {
		return nil, nil, nil, err
	}
	target, _, err := roleStore(provider, "PROMOTE_TARGET")
	if err != nil {
		return nil, nil, nil, err
	}
	return &files.UploadBackend{Provider: provider, Bucket: settings.Bucket}, source, target, nil
}
func LoadReplicaWorker() (*files.UploadBackend, *files.UploadBackend, objectstore.Store, objectstore.Store, error) {
	provider, err := driver()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if provider == "" {
		return nil, nil, nil, nil, errors.New("replication requires an explicitly enabled storage driver")
	}
	other := "r2"
	if provider == "r2" {
		other = "oss"
	}
	source, from, err := roleStore(provider, "REPLICA_SOURCE")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	target, to, err := roleStore(other, "REPLICA_TARGET")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return &files.UploadBackend{Provider: provider, Bucket: from.Bucket}, &files.UploadBackend{Provider: other, Bucket: to.Bucket}, source, target, nil
}

// Restorations read retained committed bytes and write new candidate keys.
// Neither upload, promotion, replication nor download identities are reused.
func LoadRestoreWorker() (*files.UploadBackend, objectstore.Store, objectstore.Store, error) {
	provider, err := driver()
	if err != nil {
		return nil, nil, nil, err
	}
	if provider == "" {
		return nil, nil, nil, errors.New("restoration requires an explicitly enabled storage driver")
	}
	source, settings, err := roleStore(provider, "RESTORE_SOURCE")
	if err != nil {
		return nil, nil, nil, err
	}
	target, _, err := roleStore(provider, "RESTORE_TARGET")
	if err != nil {
		return nil, nil, nil, err
	}
	return &files.UploadBackend{Provider: provider, Bucket: settings.Bucket}, source, target, nil
}
