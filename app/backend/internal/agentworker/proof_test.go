package agentworker

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSignedNativeProofBinding(t *testing.T) {
	key, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	challenge := proofChallenge{Nonce: "test-nonce", RunID: "current-run", AuthorityStamp: "current-authority", Profile: NativeProfile{SDKVersion: "0.164.0", ProfileID: "current-profile", ConfigurationSHA256: strings.Repeat("a", 64), ExpectedTools: []string{"company/read_resource"}}}
	sign := func(p proofPayload) []byte {
		raw, _ := json.Marshal(p)
		reply, _ := json.Marshal(proofReply{Payload: raw, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(secret, raw))})
		return reply
	}
	payload := proofPayload{Challenge: challenge, ExpiresAt: now.Add(30 * time.Second)}
	good := sign(payload)
	proof, err := verifyProof(good, key, challenge, now.Add(time.Minute), now)
	if err != nil || len(proof.SHA256) != 64 {
		t.Fatal("valid signature refused", err)
	}
	t.Run("different run authority configuration or tool scope cannot replay a proof", func(t *testing.T) {
		variants := []proofChallenge{challenge, challenge, challenge, challenge}
		variants[0].Nonce = "another-nonce"
		variants[1].AuthorityStamp = "another-authority"
		variants[2].Profile.ConfigurationSHA256 = strings.Repeat("b", 64)
		variants[3].Profile.ExpectedTools = []string{"company/create_project"}
		for _, want := range variants {
			if _, err := verifyProof(good, key, want, now.Add(time.Minute), now); err == nil {
				t.Fatal("replayed proof admitted")
			}
		}
	})
	t.Run("unsigned forged expired or long lived proofs cannot enable native execution", func(t *testing.T) {
		for _, expiry := range []time.Time{now, now.Add(2 * time.Second), now.Add(2 * time.Minute)} {
			p := payload
			p.ExpiresAt = expiry
			if _, err := verifyProof(sign(p), key, challenge, now.Add(time.Minute), now); err == nil {
				t.Fatal("invalid proof lifetime admitted")
			}
		}
		var forged proofReply
		_ = json.Unmarshal(good, &forged)
		forged.Payload = json.RawMessage(`{"verified":true}`)
		raw, _ := json.Marshal(forged)
		if _, err := verifyProof(raw, key, challenge, now.Add(time.Minute), now); err == nil {
			t.Fatal("forged proof admitted")
		}
		if _, err := verifyProof([]byte(`{"verified":true}`), key, challenge, now.Add(time.Minute), now); err == nil {
			t.Fatal("boolean bypass admitted")
		}
	})
}

func TestNativeProcessEnvironmentAndCancellation(t *testing.T) {
	if os.Getenv("CP_AGENT_TEST_HELPER") == "wait" {
		<-time.After(time.Minute)
		os.Exit(0)
	}
	t.Setenv("CP_DATABASE_URL", "synthetic-database-credential")
	t.Setenv("CP_MIGRATION_DATABASE_URL", "synthetic-owner-credential")
	t.Setenv("OPENAI_API_KEY", "synthetic-model-credential")
	t.Setenv("CP_AGENT_TASK_CREDENTIAL", "synthetic-task-credential")
	t.Setenv("CP_OSS_AGENT_FILE_READER_ACCESS_KEY_SECRET", "synthetic-private-file-reader-credential")
	t.Setenv("CP_OSS_AGENT_DRAFT_WRITER_ACCESS_KEY_SECRET", "synthetic-private-draft-writer-credential")
	for _, entry := range processEnvironment() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "CP_") || strings.EqualFold(name, "OPENAI_API_KEY") {
			t.Fatal("credential inherited by native environment")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestNativeProcessEnvironmentAndCancellation$")
	cmd.Env = append(processEnvironment(), "CP_AGENT_TEST_HELPER=wait")
	started := time.Now()
	if err := runOwnedProcess(ctx, cmd); err == nil {
		t.Fatal("blocked executor was not cancelled")
	}
	if time.Since(started) > 8*time.Second {
		t.Fatal("owned native cancellation exceeded bound")
	}
	if cmd.ProcessState == nil {
		t.Fatal("native process was not reaped")
	}
}
