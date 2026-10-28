package nativeruntime

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"copypaste.hk/platform/internal/agentworker"
)

type Peer struct {
	PID      int
	UID, GID uint32
}

// host is implemented only by the privileged Linux entry point. Fixture hosts
// are confined to package tests; the CLI has no host override or signing mode.
type host interface {
	Prepare(context.Context, UnitPlan, http.Handler) error
	Run(context.Context, UnitPlan, []byte) error
	Observe(context.Context, UnitPlan) (Observation, error)
	Peer(UnitPlan, Peer) error
	SaveEvidence(UnitPlan, string, []byte) error
	Stop(context.Context, UnitPlan) error
	Clean(UnitPlan) error
}
type activeRun struct {
	input             WorkerInput
	profile           Profile
	plan              UnitPlan
	ctx               context.Context
	cancel            context.CancelFunc
	running, stopping bool
	proofs            int
	requests          chan struct{}
	models            chan struct{}
}
type broker struct {
	config   Config
	key      ed25519.PrivateKey
	modelKey string
	host     host
	upstream http.RoundTripper
	clock    func() time.Time
	mu       sync.Mutex
	runs     map[string]*activeRun
	blocked  bool
}

func (b *broker) active(r *activeRun) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runs[r.plan.RunID] == r && r.running && !r.stopping && r.ctx.Err() == nil
}
func (b *broker) launch(ctx context.Context, in WorkerInput, node, entry string) Reply {
	now := b.clock()
	profile, err := b.config.ValidateRun(in, node, entry, now)
	if err != nil {
		return Reply{Code: "NATIVE_BINDING_DENIED"}
	}
	plan, err := b.config.Plan(in, now)
	if err != nil {
		return Reply{Code: "NATIVE_BINDING_DENIED"}
	}
	b.mu.Lock()
	if b.blocked || len(b.runs) >= b.config.MaxConcurrency || b.runs[in.Run.RunID] != nil {
		b.mu.Unlock()
		return Reply{Code: "NATIVE_CAPACITY_OR_DUPLICATE"}
	}
	runCtx, cancel := context.WithDeadline(ctx, in.Run.ExpiresAt)
	r := &activeRun{input: in, profile: profile, plan: plan, ctx: runCtx, cancel: cancel, requests: make(chan struct{}, 8), models: make(chan struct{}, 2)}
	b.runs[plan.RunID] = r
	b.mu.Unlock()
	defer cancel()
	// Persist the unit identity before anything can be started. Host preparation
	// refuses an existing run directory, including a completed run's tombstone.
	gateway := &gateway{broker: b, run: r}
	if err = b.host.Prepare(runCtx, plan, gateway); err != nil {
		b.mu.Lock()
		delete(b.runs, plan.RunID)
		b.mu.Unlock()
		return Reply{Code: "NATIVE_PREPARATION_FAILED"}
	}
	raw, _ := json.Marshal(in)
	b.mu.Lock()
	r.running = true
	b.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- b.host.Run(runCtx, plan, raw) }()
	var processErr error
	finished := false
	select {
	case processErr = <-done:
		finished = true
	case <-runCtx.Done():
		processErr = runCtx.Err()
	}
	b.mu.Lock()
	r.stopping = true
	r.running = false
	b.mu.Unlock()
	cancel()
	stopCtx, stop := context.WithTimeout(context.Background(), 12*time.Second)
	stopErr := b.host.Stop(stopCtx, plan)
	stop()
	if stopErr == nil {
		// A killed systemd-run client is not itself a teardown receipt. Stop has
		// verified the actual cgroup empty; now reap our own client goroutine.
		if !finished {
			timer := time.NewTimer(4 * time.Second)
			select {
			case <-done:
				finished = true
			case <-timer.C:
			}
			timer.Stop()
		}
		if !finished {
			stopErr = invalid()
		} else {
			stopErr = b.host.Clean(plan)
		}
	}
	b.mu.Lock()
	if stopErr != nil {
		b.blocked = true
	} else {
		delete(b.runs, plan.RunID)
	}
	b.mu.Unlock()
	if stopErr != nil {
		return Reply{Code: "NATIVE_STOP_UNCONFIRMED"}
	}
	if processErr != nil {
		return Reply{Code: "NATIVE_EXECUTION_STOPPED", Stopped: true}
	}
	return Reply{Code: "NATIVE_EXECUTION_FINISHED", Stopped: true}
}
func (b *broker) prove(ctx context.Context, challenge agentworker.ProofChallenge) Reply {
	if !digestPattern.MatchString(challenge.Nonce) || !uuidPattern.MatchString(challenge.RunID) {
		return Reply{Code: "NATIVE_PROOF_DENIED"}
	}
	b.mu.Lock()
	r := b.runs[challenge.RunID]
	b.mu.Unlock()
	if r == nil || !b.active(r) || challenge.AuthorityStamp != r.input.Run.AuthorityStamp || challenge.Profile.ProfileID != r.profile.ID || challenge.Profile.Model != r.profile.Model || challenge.Profile.WorkingDirectory != b.config.WorkDirectory || challenge.Profile.SDKVersion != "0.164.0" || challenge.Profile.ConfigurationSHA256 != r.profile.ConfigurationSHA256 || !sameTools(challenge.Profile.ExpectedTools, r.profile.Tools) {
		return Reply{Code: "NATIVE_PROOF_DENIED"}
	}
	b.mu.Lock()
	if r.proofs >= 16 {
		b.mu.Unlock()
		return Reply{Code: "NATIVE_PROOF_DENIED"}
	}
	r.proofs++
	b.mu.Unlock()
	observation, err := b.host.Observe(ctx, r.plan)
	if err != nil || observation.Validate(b.config, r.plan) != nil || !b.active(r) {
		return Reply{Code: "NATIVE_PROOF_DENIED"}
	}
	raw, _ := json.Marshal(observation)
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	if b.host.SaveEvidence(r.plan, digest, raw) != nil {
		return Reply{Code: "NATIVE_PROOF_DENIED"}
	}
	now := b.clock()
	expiry := now.Add(30 * time.Second)
	if expiry.After(r.plan.ExpiresAt) {
		expiry = r.plan.ExpiresAt
	}
	if !expiry.After(now.Add(5 * time.Second)) {
		return Reply{Code: "NATIVE_PROOF_DENIED"}
	}
	payload, _ := json.Marshal(agentworker.ProofPayload{Challenge: challenge, ExpiresAt: expiry, EvidenceSHA256: digest})
	// Serialize the final liveness check and signing with cancellation. No I/O
	// occurs while this lock is held, and the model never sees the signing key.
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.runs[r.plan.RunID] != r || !r.running || r.stopping || r.ctx.Err() != nil {
		return Reply{Code: "NATIVE_PROOF_DENIED"}
	}
	return Reply{Code: "NATIVE_PROOF_ISSUED", Proof: &agentworker.ProofReply{Payload: payload, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(b.key, payload))}}
}
func sameTools(a, c []string) bool {
	if len(a) != len(c) {
		return false
	}
	a = slices.Clone(a)
	c = slices.Clone(c)
	slices.Sort(a)
	slices.Sort(c)
	return slices.Equal(a, c) && len(slices.Compact(a)) == len(a)
}
func (b *broker) dispatch(ctx context.Context, req Request) Reply {
	switch req.Action {
	case "launch":
		if req.Input == nil || req.Challenge != nil {
			return Reply{Code: "NATIVE_PROTOCOL_DENIED"}
		}
		return b.launch(ctx, *req.Input, req.Node, req.Entry)
	case "prove":
		if req.Input != nil || req.Challenge == nil || req.Node != "" || req.Entry != "" {
			return Reply{Code: "NATIVE_PROTOCOL_DENIED"}
		}
		return b.prove(ctx, *req.Challenge)
	default:
		return Reply{Code: "NATIVE_PROTOCOL_DENIED"}
	}
}
func safeError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New("native boundary operation refused")
}

type peerContextKey struct{}

func peerFrom(r *http.Request) (Peer, bool) {
	p, ok := r.Context().Value(peerContextKey{}).(Peer)
	return p, ok
}
func writeSafe(w http.ResponseWriter, code int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, `{"error":{"code":"NATIVE_GATEWAY_DENIED"}}`)
}
