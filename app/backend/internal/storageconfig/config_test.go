package storageconfig

import (
	"strings"
	"testing"
)

func fixtureSettings(t *testing.T, provider string) {
	t.Helper()
	prefix := "CP_" + strings.ToUpper(provider) + "_"
	endpoint := "https://oss-cn-shenzhen.aliyuncs.com"
	if provider == "r2" {
		endpoint = "https://fixture-account.r2.cloudflarestorage.com"
	}
	t.Setenv("CP_STORAGE_DRIVER", provider)
	t.Setenv(prefix+"ENDPOINT", endpoint)
	t.Setenv(prefix+"REGION", "cn-shenzhen")
	t.Setenv(prefix+"BUCKET", "fixture-bucket")
	for _, role := range []string{"UPLOAD_SIGNER", "UPLOAD_INSPECTOR", "UPLOAD_LIFECYCLE", "UPLOAD_VERIFY", "PROMOTE_SOURCE", "PROMOTE_TARGET", "REPLICA_SOURCE", "REPLICA_TARGET", "DOWNLOAD_SIGNER", "DOWNLOAD_READER"} {
		t.Setenv(prefix+role+"_ACCESS_KEY_ID", "")
		t.Setenv(prefix+role+"_ACCESS_KEY_SECRET", "")
		t.Setenv(prefix+role+"_SESSION_TOKEN", "")
	}
}
func TestRestoreWorkerRequiresIndependentReadAndWriteRoles(t *testing.T) {
	for _, provider := range []string{"oss", "r2"} {
		fixtureSettings(t, provider)
		t.Setenv("CP_STORAGE_DRIVER", provider)
		prefix := "CP_" + strings.ToUpper(provider) + "_"
		t.Setenv(prefix+"RESTORE_SOURCE_ACCESS_KEY_ID", "")
		t.Setenv(prefix+"RESTORE_SOURCE_ACCESS_KEY_SECRET", "")
		t.Setenv(prefix+"RESTORE_TARGET_ACCESS_KEY_ID", "")
		t.Setenv(prefix+"RESTORE_TARGET_ACCESS_KEY_SECRET", "")
		t.Setenv(prefix+"PROMOTE_SOURCE_ACCESS_KEY_ID", "unrelated")
		t.Setenv(prefix+"PROMOTE_SOURCE_ACCESS_KEY_SECRET", "unrelated")
		if _, _, _, err := LoadRestoreWorker(); err == nil {
			t.Fatal("restoration inherited promotion identity")
		}
		t.Setenv(prefix+"RESTORE_SOURCE_ACCESS_KEY_ID", "explicit-reader")
		t.Setenv(prefix+"RESTORE_SOURCE_ACCESS_KEY_SECRET", "explicit-reader")
		if _, _, _, err := LoadRestoreWorker(); err == nil {
			t.Fatal("read identity permitted target writes")
		}
		t.Setenv(prefix+"RESTORE_TARGET_ACCESS_KEY_ID", "explicit-target")
		t.Setenv(prefix+"RESTORE_TARGET_ACCESS_KEY_SECRET", "explicit-target")
		backend, read, write, err := LoadRestoreWorker()
		if err != nil || backend == nil || backend.Provider != provider || backend.Signer != nil || backend.Lister != nil || read == nil || write == nil {
			t.Fatal("explicit restoration roles unavailable", err)
		}
	}
}
func TestDownloadsRequireExplicitIndependentReadAndSigningRoles(t *testing.T) {
	fixtureSettings(t, "oss")
	fixtureSettings(t, "r2")
	t.Setenv("CP_STORAGE_DRIVER", "disabled")
	t.Setenv("CP_DOWNLOAD_PROVIDERS", "disabled")
	if configured, err := LoadDownloads(); err != nil || len(configured) != 0 {
		t.Fatal("disabled downloads constructed a provider", err)
	}
	t.Setenv("CP_DOWNLOAD_PROVIDERS", "oss,r2")
	for _, provider := range []string{"OSS", "R2"} {
		prefix := "CP_" + provider + "_"
		t.Setenv(prefix+"UPLOAD_SIGNER_ACCESS_KEY_ID", "unrelated")
		t.Setenv(prefix+"UPLOAD_SIGNER_ACCESS_KEY_SECRET", "unrelated")
		t.Setenv(prefix+"DOWNLOAD_SIGNER_ACCESS_KEY_ID", "explicit-signer")
		t.Setenv(prefix+"DOWNLOAD_SIGNER_ACCESS_KEY_SECRET", "explicit-signer")
	}
	if _, err := LoadDownloads(); err == nil {
		t.Fatal("download signing role was used as a streaming reader")
	}
	for _, provider := range []string{"OSS", "R2"} {
		t.Setenv("CP_"+provider+"_DOWNLOAD_READER_ACCESS_KEY_ID", "explicit-reader")
		t.Setenv("CP_"+provider+"_DOWNLOAD_READER_ACCESS_KEY_SECRET", "explicit-reader")
	}
	configured, err := LoadDownloads()
	if err != nil || len(configured) != 2 || configured["oss"].Reader == nil || configured["r2"].Signer == nil {
		t.Fatal("independent read roles required an active write provider", err)
	}
	for _, value := range []string{"oss,oss", "oss,", "unknown", "disabled,r2"} {
		t.Setenv("CP_DOWNLOAD_PROVIDERS", value)
		if _, err := LoadDownloads(); err == nil {
			t.Fatal("invalid or ambiguous read provider list", value)
		}
	}
}
func TestReplicaWorkerRequiresSourceAndOtherCloudTargetRoles(t *testing.T) {
	for _, provider := range []string{"oss", "r2"} {
		other := "r2"
		if provider == "r2" {
			other = "oss"
		}
		fixtureSettings(t, provider)
		fixtureSettings(t, other)
		t.Setenv("CP_STORAGE_DRIVER", provider)
		source := "CP_" + strings.ToUpper(provider) + "_"
		target := "CP_" + strings.ToUpper(other) + "_"
		t.Setenv(source+"PROMOTE_SOURCE_ACCESS_KEY_ID", "fixture-unrelated")
		t.Setenv(source+"PROMOTE_SOURCE_ACCESS_KEY_SECRET", "fixture-unrelated")
		if _, _, _, _, err := LoadReplicaWorker(); err == nil {
			t.Fatal("replication inherited promotion credentials")
		}
		t.Setenv(source+"REPLICA_SOURCE_ACCESS_KEY_ID", "fixture-read")
		t.Setenv(source+"REPLICA_SOURCE_ACCESS_KEY_SECRET", "fixture-read")
		if _, _, _, _, err := LoadReplicaWorker(); err == nil {
			t.Fatal("read role allowed writes without other cloud target")
		}
		t.Setenv(target+"REPLICA_TARGET_ACCESS_KEY_ID", "fixture-write")
		t.Setenv(target+"REPLICA_TARGET_ACCESS_KEY_SECRET", "fixture-write")
		from, to, read, write, err := LoadReplicaWorker()
		if err != nil || from == nil || to == nil || from.Provider != provider || to.Provider != other || read == nil || write == nil {
			t.Fatal("explicit direction failed", err)
		}
	}
}
func TestCloudProcessRolesNeverFallBackToOtherCredentials(t *testing.T) {
	for _, provider := range []string{"oss", "r2"} {
		t.Run(provider, func(t *testing.T) {
			fixtureSettings(t, provider)
			prefix := "CP_" + strings.ToUpper(provider) + "_"
			t.Setenv("AWS_ACCESS_KEY_ID", "fixture-shared-key")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-shared-secret")
			t.Setenv(prefix+"UPLOAD_LIFECYCLE_ACCESS_KEY_ID", "fixture-worker-key")
			t.Setenv(prefix+"UPLOAD_LIFECYCLE_ACCESS_KEY_SECRET", "fixture-worker-secret")
			if _, err := LoadAPI(); err == nil {
				t.Fatal("API fell back to worker or shared SDK credentials")
			}
			info, store, err := LoadUploadWorker()
			if err != nil || info == nil || store == nil || info.Signer != nil || info.Lister != nil {
				t.Fatal("worker required or exposed API identities", err)
			}
			if _, _, err := LoadVerificationWorker(); err == nil {
				t.Fatal("verification inherited lifecycle credentials")
			}
			t.Setenv(prefix+"UPLOAD_VERIFY_ACCESS_KEY_ID", "fixture-verify-key")
			t.Setenv(prefix+"UPLOAD_VERIFY_ACCESS_KEY_SECRET", "fixture-verify-secret")
			info, store, err = LoadVerificationWorker()
			if err != nil || info == nil || store == nil || info.Signer != nil || info.Lister != nil {
				t.Fatal("verification required unrelated cloud identities", err)
			}
			if _, _, _, err := LoadPromotionWorker(); err == nil {
				t.Fatal("promotion inherited another worker identity")
			}
			t.Setenv(prefix+"PROMOTE_SOURCE_ACCESS_KEY_ID", "fixture-promotion-read")
			t.Setenv(prefix+"PROMOTE_SOURCE_ACCESS_KEY_SECRET", "fixture-promotion-read-secret")
			if _, _, _, err := LoadPromotionWorker(); err == nil {
				t.Fatal("promotion read identity used for writes")
			}
			t.Setenv(prefix+"PROMOTE_TARGET_ACCESS_KEY_ID", "fixture-promotion-write")
			t.Setenv(prefix+"PROMOTE_TARGET_ACCESS_KEY_SECRET", "fixture-promotion-write-secret")
			if info, read, write, err := LoadPromotionWorker(); err != nil || info == nil || read == nil || write == nil {
				t.Fatal("explicit promotion identities rejected", err)
			}
			t.Setenv(prefix+"UPLOAD_SIGNER_ACCESS_KEY_ID", "fixture-signer-key")
			t.Setenv(prefix+"UPLOAD_SIGNER_ACCESS_KEY_SECRET", "fixture-signer-secret")
			if _, err = LoadAPI(); err == nil {
				t.Fatal("API silently used signing credentials for inspection")
			}
			t.Setenv(prefix+"UPLOAD_INSPECTOR_ACCESS_KEY_ID", "fixture-inspector-key")
			t.Setenv(prefix+"UPLOAD_INSPECTOR_ACCESS_KEY_SECRET", "fixture-inspector-secret")
			api, err := LoadAPI()
			if err != nil || api.Signer == nil || api.Lister == nil {
				t.Fatal("explicit API roles failed", err)
			}
			t.Setenv(prefix+"ENDPOINT", "http://fixture.invalid")
			if _, err = LoadAPI(); err == nil || strings.Contains(err.Error(), "fixture-worker-secret") {
				t.Fatal("unsafe endpoint accepted or secret disclosed")
			}
		})
	}
}
func TestDisabledCloudNeverConstructsAnImplicitWorker(t *testing.T) {
	t.Setenv("CP_STORAGE_DRIVER", "disabled")
	backend, err := LoadAPI()
	if err != nil || backend != nil {
		t.Fatal("disabled API unexpectedly enabled storage", err)
	}
	if _, _, err = LoadUploadWorker(); err == nil {
		t.Fatal("disabled cloud started a worker")
	}
	if _, _, err = LoadVerificationWorker(); err == nil {
		t.Fatal("disabled cloud started a verification worker")
	}
	t.Setenv("CP_STORAGE_DRIVER", "unknown")
	if _, err = LoadAPI(); err == nil {
		t.Fatal("unknown cloud driver accepted")
	}
}
