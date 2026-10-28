package agentworker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
)

type SignedProbe struct {
	Command   string
	PublicKey ed25519.PublicKey
}
type proofChallenge struct {
	Nonce          string        `json:"nonce"`
	RunID          string        `json:"run_id"`
	AuthorityStamp string        `json:"authority_stamp"`
	Profile        NativeProfile `json:"profile"`
}
type proofPayload struct {
	Challenge      proofChallenge `json:"challenge"`
	ExpiresAt      time.Time      `json:"expires_at"`
	EvidenceSHA256 string         `json:"evidence_sha256,omitempty"`
}
type proofReply struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

// Native issuers use the same canonical binding and signed payload as the
// verifier. The optional evidence digest names a private kernel observation.
type ProofChallenge = proofChallenge
type ProofPayload = proofPayload
type ProofReply = proofReply
type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, core.Fail(409, "RUNTIME_CONFINEMENT_UNVERIFIED", "验证输出过大")
	}
	return b.Buffer.Write(p)
}

func (p SignedProbe) Verify(ctx context.Context, lease agent.ExecutionLease, profile NativeProfile) (ConfinementProof, error) {
	fail := func() (ConfinementProof, error) {
		return ConfinementProof{}, core.Fail(409, "RUNTIME_CONFINEMENT_UNVERIFIED", "原生执行边界尚未验证")
	}
	if len(p.PublicKey) != ed25519.PublicKeySize || p.Command == "" {
		return fail()
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return fail()
	}
	challenge := proofChallenge{hex.EncodeToString(nonce), lease.RunID, lease.AuthorityStamp, profile}
	input, _ := json.Marshal(challenge)
	limited, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.Command(p.Command)
	cmd.Env = processEnvironment()
	cmd.Stdin = bytes.NewReader(input)
	output := &boundedOutput{}
	cmd.Stdout = output
	cmd.Stderr = &boundedOutput{}
	if runOwnedProcess(limited, cmd) != nil {
		return fail()
	}
	return verifyProof(output.Bytes(), p.PublicKey, challenge, lease.ExpiresAt, time.Now())
}
func verifyProof(raw []byte, key ed25519.PublicKey, challenge proofChallenge, deadline, now time.Time) (ConfinementProof, error) {
	fail := func() (ConfinementProof, error) {
		return ConfinementProof{}, core.Fail(409, "RUNTIME_CONFINEMENT_UNVERIFIED", "原生执行边界尚未验证")
	}
	if len(key) != ed25519.PublicKeySize {
		return fail()
	}
	var reply proofReply
	if decodeRaw(raw, &reply) != nil {
		return fail()
	}
	sig, err := base64.StdEncoding.DecodeString(reply.Signature)
	if err != nil || !ed25519.Verify(key, reply.Payload, sig) {
		return fail()
	}
	var payload proofPayload
	if decodeRaw(reply.Payload, &payload) != nil {
		return fail()
	}
	if payload.EvidenceSHA256 != "" {
		decoded, err := hex.DecodeString(payload.EvidenceSHA256)
		if err != nil || len(decoded) != 32 || payload.EvidenceSHA256 != hex.EncodeToString(decoded) {
			return fail()
		}
	}
	actual, _ := json.Marshal(payload.Challenge)
	expected, _ := json.Marshal(challenge)
	if !bytes.Equal(actual, expected) || !payload.ExpiresAt.After(now.Add(5*time.Second)) || payload.ExpiresAt.After(now.Add(60*time.Second)) || payload.ExpiresAt.After(deadline) {
		return fail()
	}
	return ConfinementProof{SHA256: core.Digest(raw), ExpiresAt: payload.ExpiresAt}, nil
}
