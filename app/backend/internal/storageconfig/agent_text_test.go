package storageconfig

import "testing"

func TestAgentDraftWriterRequiresIndependentIdentity(t *testing.T) {
	fixtureSettings(t, "oss")
	t.Setenv("CP_AGENT_DRAFT_DRIVER", "disabled")
	if writer, err := LoadAgentDraftWriter(); err != nil || writer != nil {
		t.Fatal("disabled AI draft role constructed a writer")
	}
	t.Setenv("CP_AGENT_DRAFT_DRIVER", "oss")
	t.Setenv("CP_OSS_AGENT_DRAFT_WRITER_ACCESS_KEY_ID", "")
	t.Setenv("CP_OSS_AGENT_DRAFT_WRITER_ACCESS_KEY_SECRET", "")
	for _, role := range []string{"AGENT_FILE_READER", "UPLOAD_SIGNER", "UPLOAD_LIFECYCLE", "PROMOTE_TARGET"} {
		t.Setenv("CP_OSS_"+role+"_ACCESS_KEY_ID", "unrelated-fixture-key")
		t.Setenv("CP_OSS_"+role+"_ACCESS_KEY_SECRET", "unrelated-fixture-secret")
	}
	if _, err := LoadAgentDraftWriter(); err == nil {
		t.Fatal("another storage identity enabled AI draft writing")
	}
	t.Setenv("CP_OSS_AGENT_DRAFT_WRITER_ACCESS_KEY_ID", "dedicated-fixture-key")
	t.Setenv("CP_OSS_AGENT_DRAFT_WRITER_ACCESS_KEY_SECRET", "dedicated-fixture-secret")
	writer, err := LoadAgentDraftWriter()
	if err != nil || writer == nil || writer.Writer == nil || writer.Provider != "oss" || writer.Bucket != "fixture-bucket" {
		t.Fatal("explicit independent writer not constructed")
	}
	for _, provider := range []string{"oss,r2", "other"} {
		t.Setenv("CP_AGENT_DRAFT_DRIVER", provider)
		if _, err = LoadAgentDraftWriter(); err == nil {
			t.Fatal("ambiguous draft provider admitted")
		}
	}
}

func TestAgentFileReaderRequiresIndependentIdentity(t *testing.T) {
	fixtureSettings(t, "oss")
	t.Setenv("CP_AGENT_FILE_PROVIDERS", "disabled")
	if readers, err := LoadAgentTextReaders(); err != nil || len(readers) != 0 {
		t.Fatal("disabled AI file role constructed a reader")
	}
	t.Setenv("CP_AGENT_FILE_PROVIDERS", "oss")
	t.Setenv("CP_OSS_AGENT_FILE_READER_ACCESS_KEY_ID", "")
	t.Setenv("CP_OSS_AGENT_FILE_READER_ACCESS_KEY_SECRET", "")
	for _, role := range []string{"DOWNLOAD_READER", "DOWNLOAD_SIGNER", "UPLOAD_INSPECTOR", "UPLOAD_SIGNER"} {
		t.Setenv("CP_OSS_"+role+"_ACCESS_KEY_ID", "unrelated-fixture-key")
		t.Setenv("CP_OSS_"+role+"_ACCESS_KEY_SECRET", "unrelated-fixture-secret")
	}
	if _, err := LoadAgentTextReaders(); err == nil {
		t.Fatal("ordinary download/upload identity enabled AI file processing")
	}
	t.Setenv("CP_OSS_AGENT_FILE_READER_ACCESS_KEY_ID", "dedicated-fixture-key")
	t.Setenv("CP_OSS_AGENT_FILE_READER_ACCESS_KEY_SECRET", "dedicated-fixture-secret")
	readers, err := LoadAgentTextReaders()
	if err != nil || len(readers) != 1 || readers["oss"].Reader == nil || readers["oss"].Bucket != "fixture-bucket" {
		t.Fatal("explicit independent reader not constructed")
	}
	for _, list := range []string{"oss,oss", "oss,other", "oss,"} {
		t.Setenv("CP_AGENT_FILE_PROVIDERS", list)
		if _, err = LoadAgentTextReaders(); err == nil {
			t.Fatal("ambiguous AI reader providers admitted")
		}
	}
}
