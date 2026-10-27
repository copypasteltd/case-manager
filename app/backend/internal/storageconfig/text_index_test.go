package storageconfig

import "testing"

func TestTextIndexRequiresIndependentReader(t *testing.T) {
	fixtureSettings(t, "oss")
	t.Setenv("CP_TEXT_INDEX_PROVIDERS", "disabled")
	if readers, err := LoadTextIndexReaders(); err != nil || len(readers) != 0 {
		t.Fatal("disabled reader created")
	}
	t.Setenv("CP_TEXT_INDEX_PROVIDERS", "oss")
	for _, suffix := range []string{"ACCESS_KEY_ID", "ACCESS_KEY_SECRET"} {
		t.Setenv("CP_OSS_TEXT_INDEX_READER_"+suffix, "")
	}
	for _, role := range []string{"AGENT_FILE_READER", "DOWNLOAD_READER", "UPLOAD_VERIFY", "PROMOTE_SOURCE"} {
		t.Setenv("CP_OSS_"+role+"_ACCESS_KEY_ID", "unrelated-fixture-key")
		t.Setenv("CP_OSS_"+role+"_ACCESS_KEY_SECRET", "unrelated-fixture-secret")
	}
	if _, err := LoadTextIndexReaders(); err == nil {
		t.Fatal("another identity enabled text indexing")
	}
	t.Setenv("CP_OSS_TEXT_INDEX_READER_ACCESS_KEY_ID", "dedicated-fixture-key")
	t.Setenv("CP_OSS_TEXT_INDEX_READER_ACCESS_KEY_SECRET", "dedicated-fixture-secret")
	readers, err := LoadTextIndexReaders()
	if err != nil || len(readers) != 1 || readers["oss"].Reader == nil || readers["oss"].Bucket != "fixture-bucket" {
		t.Fatal("independent reader not constructed")
	}
	for _, list := range []string{"oss,oss", "oss,other", "oss,", "oss,r2,oss"} {
		t.Setenv("CP_TEXT_INDEX_PROVIDERS", list)
		if _, err = LoadTextIndexReaders(); err == nil {
			t.Fatal("ambiguous role admitted")
		}
	}
}
