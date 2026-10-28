package storageconfig

import "testing"

func TestMultipartCleanupSeparatesAPIMetadataAndWorkerIdentity(t *testing.T) {
	for _, provider := range []string{"oss", "r2"} {
		t.Run(provider, func(t *testing.T) {
			fixtureSettings(t, provider)
			t.Setenv("CP_STORAGE_DRIVER", "disabled")
			t.Setenv("CP_MULTIPART_CLEANUP_PROVIDERS", provider)
			targets, e := LoadMultipartCleanupTargets()
			if e != nil || len(targets) != 1 || targets[provider].Cleaner != nil || len(targets[provider].BindingSHA256) != 64 {
				t.Fatal("API acquired cleaner or metadata failed", e)
			}
			prefix := "CP_OSS_"
			if provider == "r2" {
				prefix = "CP_R2_"
			}
			t.Setenv(prefix+"MULTIPART_CLEANER_ACCESS_KEY_ID", "")
			t.Setenv(prefix+"MULTIPART_CLEANER_ACCESS_KEY_SECRET", "")
			t.Setenv(prefix+"UPLOAD_LIFECYCLE_ACCESS_KEY_ID", "lifecycle")
			t.Setenv(prefix+"UPLOAD_LIFECYCLE_ACCESS_KEY_SECRET", "lifecycle")
			if _, e = LoadMultipartCleaners(); e == nil {
				t.Fatal("worker borrowed upload identity")
			}
			t.Setenv(prefix+"MULTIPART_CLEANER_ACCESS_KEY_ID", "explicit-cleaner")
			t.Setenv(prefix+"MULTIPART_CLEANER_ACCESS_KEY_SECRET", "explicit-cleaner-secret")
			cleaners, e := LoadMultipartCleaners()
			if e != nil || cleaners[provider].Cleaner == nil || cleaners[provider].BindingSHA256 != targets[provider].BindingSHA256 {
				t.Fatal("independent cleaner missing", e)
			}
		})
	}
	for _, value := range []string{"oss,oss", "oss,", "disabled,r2", "other"} {
		t.Setenv("CP_MULTIPART_CLEANUP_PROVIDERS", value)
		if _, e := LoadMultipartCleanupTargets(); e == nil {
			t.Fatal("ambiguous providers accepted", value)
		}
	}
	t.Setenv("CP_MULTIPART_CLEANUP_PROVIDERS", "disabled")
	targets, e := LoadMultipartCleaners()
	if e != nil || len(targets) != 0 {
		t.Fatal("disabled cleanup enabled cloud clients", e)
	}
}
