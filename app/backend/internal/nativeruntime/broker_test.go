package nativeruntime

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agentworker"
)

// This fixture represents an explicitly synthetic kernel observation. Production
// uses linuxHost only; none of these checks claim that a Linux unit ran here.
func fixtureObservation(c Config, p UnitPlan) Observation {
	properties := map[string]string{}
	for _, prop := range p.Properties {
		k, v, _ := strings.Cut(prop, "=")
		properties[k] = v
	}
	properties["ActiveState"] = "active"
	properties["SubState"] = "running"
	properties["Delegate"] = "no"
	properties["SystemCallFilter"] = "~" + strings.Join(criticalDeniedSyscalls, " ")
	o := Observation{Unit: p.Unit, PID: 4200, UID: 62001, GID: 62001, StartTicks: 123456,
		Cgroup: "/system.slice/" + p.Unit, ProcessCgroup: "/system.slice/" + p.Unit,
		MountNamespace: 11, HostMountNamespace: 10, NetworkNamespace: 21, HostNetworkNamespace: 20,
		RootDevice: 7, RootInode: 100, ExpectedRootDevice: 7, ExpectedRootInode: 100,
		CapEffective: "0000000000000000", CapPermitted: "0000000000000000", CapBounding: "0000000000000000", NoNewPrivileges: 1, Seccomp: 2,
		MemoryMax: c.Limits.MemoryBytes, SwapMax: 0, TasksMax: int64(c.Limits.Tasks), CPUQuota: 100000, CPUPeriod: 100000, Properties: properties, ReleaseSHA256: c.ReleaseSHA256, InspectedAt: time.Now()}
	for _, target := range append([]string{c.Release, c.Node.Path, c.Codex.Path, c.Broker.Path}, c.LibraryMounts...) {
		o.Mounts = append(o.Mounts, Mount{Target: target, Type: "ext4", Options: []string{"ro"}})
	}
	o.Mounts = append(o.Mounts, Mount{Target: c.WorkDirectory, Type: "tmpfs", Options: []string{"rw", "nosuid", "nodev", "noexec"}, SuperOptions: []string{"size=65536k"}}, Mount{Target: "/tmp", Type: "tmpfs", Options: []string{"rw", "nosuid", "nodev", "noexec"}, SuperOptions: []string{"size=16384k"}})
	return o
}

func TestNativeObservationRequiresActualIndependentBoundaryFacts(t *testing.T) {
	c := fixtureConfig()
	now := time.Now()
	p, _ := c.Plan(fixtureInput(c, now), now)
	good := fixtureObservation(c, p)
	if good.Validate(c, p) != nil {
		t.Fatal("synthetic complete observation refused")
	}
	cases := map[string]func(*Observation){
		"root process": func(o *Observation) { o.UID = 0 }, "worker identity": func(o *Observation) { o.UID = c.WorkerUID }, "different cgroup": func(o *Observation) { o.ProcessCgroup += "/child" }, "unit prefix collision": func(o *Observation) { o.Cgroup += "-other" },
		"host mount namespace": func(o *Observation) { o.MountNamespace = o.HostMountNamespace }, "host network": func(o *Observation) { o.NetworkNamespace = o.HostNetworkNamespace }, "host root": func(o *Observation) { o.RootInode++ }, "pid reuse marker": func(o *Observation) { o.StartTicks = 0 },
		"effective capability": func(o *Observation) { o.CapEffective = "0000000000000001" }, "permitted capability": func(o *Observation) { o.CapPermitted = "1" }, "bounding capability": func(o *Observation) { o.CapBounding = "1" }, "seccomp absent": func(o *Observation) { o.Seccomp = 0 }, "new privileges": func(o *Observation) { o.NoNewPrivileges = 0 },
		"memory uncapped": func(o *Observation) { o.MemoryMax = -1 }, "swap enabled": func(o *Observation) { o.SwapMax = 1 }, "fork limit missing": func(o *Observation) { o.TasksMax = -1 }, "cpu excessive": func(o *Observation) { o.CPUQuota = 200000 }, "cpu overflow": func(o *Observation) { o.CPUQuota = 1 << 62 },
		"unpublished bundle": func(o *Observation) { o.ReleaseSHA256 = strings.Repeat("f", 64) }, "writable bundle": func(o *Observation) { o.Mounts[0].Options = []string{"rw"} }, "executable work": func(o *Observation) { o.Mounts[len(o.Mounts)-2].Options = []string{"rw", "nosuid", "nodev"} }, "uncapped tmp": func(o *Observation) { o.Mounts[len(o.Mounts)-1].SuperOptions = []string{"size=1g"} },
		"syscall policy missing": func(o *Observation) { o.Properties["SystemCallFilter"] = "" }, "broad exec": func(o *Observation) { o.Properties["ExecPaths"] += " /bin" }, "broader AF": func(o *Observation) { o.Properties["RestrictAddressFamilies"] += " AF_INET6" }, "namespaces enabled": func(o *Observation) { o.Properties["RestrictNamespaces"] = "no" }, "not running": func(o *Observation) { o.Properties["SubState"] = "exited" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			o := clone(good)
			mutate(&o)
			if o.Validate(c, p) == nil {
				t.Fatal("incomplete boundary received an attestation")
			}
		})
	}
}

type fixtureHost struct {
	config                           Config
	started                          chan UnitPlan
	release                          chan struct{}
	mu                               sync.Mutex
	stopErr, prepareErr, evidenceErr error
	stops, cleans                    int
	evidence                         map[string][]byte
	observation                      func(Observation) Observation
	peerDenied                       bool
}

func (h *fixtureHost) Prepare(context.Context, UnitPlan, http.Handler) error { return h.prepareErr }
func (h *fixtureHost) Run(ctx context.Context, p UnitPlan, _ []byte) error {
	h.started <- p
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.release:
		return nil
	}
}
func (h *fixtureHost) Observe(_ context.Context, p UnitPlan) (Observation, error) {
	o := fixtureObservation(h.config, p)
	if h.observation != nil {
		o = h.observation(o)
	}
	return o, nil
}
func (h *fixtureHost) Peer(_ UnitPlan, p Peer) error {
	if h.peerDenied || p.PID != 4200 || p.UID != 62001 {
		return invalid()
	}
	return nil
}
func (h *fixtureHost) SaveEvidence(_ UnitPlan, digest string, raw []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.evidenceErr != nil {
		return h.evidenceErr
	}
	h.evidence[digest] = append([]byte(nil), raw...)
	return nil
}
func (h *fixtureHost) Stop(context.Context, UnitPlan) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stops++
	return h.stopErr
}
func (h *fixtureHost) Clean(UnitPlan) error { h.mu.Lock(); defer h.mu.Unlock(); h.cleans++; return nil }
func fixtureBroker(t *testing.T) (*broker, *fixtureHost, WorkerInput) {
	t.Helper()
	c := fixtureConfig()
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	h := &fixtureHost{config: c, started: make(chan UnitPlan, 4), release: make(chan struct{}), evidence: map[string][]byte{}}
	b := &broker{config: c, key: key, modelKey: "synthetic-host-only-provider-secret", host: h, clock: time.Now, runs: map[string]*activeRun{}}
	return b, h, fixtureInput(c, time.Now())
}
func startFixture(t *testing.T, b *broker, h *fixtureHost, in WorkerInput) (UnitPlan, context.CancelFunc, <-chan Reply) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Reply, 1)
	go func() { done <- b.launch(ctx, in, b.config.Node.Path, b.config.Entry) }()
	t.Cleanup(cancel)
	select {
	case p := <-h.started:
		return p, cancel, done
	case r := <-done:
		t.Fatalf("launch refused: %s", r.Code)
	case <-time.After(2 * time.Second):
		t.Fatal("fixture start timeout")
	}
	return UnitPlan{}, cancel, done
}
func fixtureChallenge(b *broker, in WorkerInput) agentworker.ProofChallenge {
	return agentworker.ProofChallenge{Nonce: strings.Repeat("c", 64), RunID: in.Run.RunID, AuthorityStamp: in.Run.AuthorityStamp, Profile: agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: in.Settings.ProfileID, Model: in.Settings.Model, WorkingDirectory: in.Settings.WorkingDirectory, ConfigurationSHA256: in.Run.Profile.ConfigurationSHA256, ExpectedTools: b.config.Profiles[0].Tools}}
}
func receive(t *testing.T, done <-chan Reply) Reply {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("fixture stop timeout")
	}
	return Reply{}
}

func TestNativeBrokerSignsFreshEvidenceAndStopsOnDisconnect(t *testing.T) {
	b, h, in := fixtureBroker(t)
	_, cancel, done := startFixture(t, b, h, in)
	challenge := fixtureChallenge(b, in)
	reply := b.prove(context.Background(), challenge)
	if reply.Proof == nil || reply.Code != "NATIVE_PROOF_ISSUED" {
		t.Fatal(reply.Code)
	}
	sig, e := base64.StdEncoding.DecodeString(reply.Proof.Signature)
	if e != nil || !ed25519.Verify(b.key.Public().(ed25519.PublicKey), reply.Proof.Payload, sig) {
		t.Fatal("invalid real Ed25519 signature")
	}
	var payload agentworker.ProofPayload
	if json.Unmarshal(reply.Proof.Payload, &payload) != nil || payload.Challenge.Nonce != challenge.Nonce || payload.Challenge.RunID != in.Run.RunID || !payload.ExpiresAt.Before(time.Now().Add(31*time.Second)) {
		t.Fatal("proof not freshly run-bound")
	}
	raw := h.evidence[payload.EvidenceSHA256]
	sum := sha256.Sum256(raw)
	if len(raw) == 0 || hex.EncodeToString(sum[:]) != payload.EvidenceSHA256 {
		t.Fatal("proof does not identify retained observation")
	}
	challenge.Nonce = strings.Repeat("d", 64)
	second := b.prove(context.Background(), challenge)
	if second.Proof == nil || string(second.Proof.Payload) == string(reply.Proof.Payload) {
		t.Fatal("new challenge replayed old proof")
	}
	bad := challenge
	bad.AuthorityStamp = "revoked-authority"
	if b.prove(context.Background(), bad).Proof != nil {
		t.Fatal("wrong authority signed")
	}
	bad = challenge
	bad.Profile.ExpectedTools = append(clone(bad.Profile.ExpectedTools), "company/create_project")
	if b.prove(context.Background(), bad).Proof != nil {
		t.Fatal("wider tool set signed")
	}
	if b.launch(context.Background(), in, b.config.Node.Path, b.config.Entry).Code != "NATIVE_CAPACITY_OR_DUPLICATE" {
		t.Fatal("duplicate active run admitted")
	}
	cancel()
	r := receive(t, done)
	if !r.Stopped || r.Code != "NATIVE_EXECUTION_STOPPED" || h.stops != 1 || h.cleans != 1 {
		t.Fatal("disconnect did not stop and clean owned run")
	}
	if b.prove(context.Background(), challenge).Proof != nil {
		t.Fatal("stopped run can still attest")
	}
}
func TestNativeBrokerDoesNotSignInvalidOrUnrecordedObservation(t *testing.T) {
	for _, mode := range []string{"kernel mismatch", "record failed"} {
		t.Run(mode, func(t *testing.T) {
			b, h, in := fixtureBroker(t)
			if mode == "kernel mismatch" {
				h.observation = func(o Observation) Observation { o.NetworkNamespace = o.HostNetworkNamespace; return o }
			} else {
				h.evidenceErr = invalid()
			}
			_, cancel, done := startFixture(t, b, h, in)
			if b.prove(context.Background(), fixtureChallenge(b, in)).Proof != nil {
				t.Fatal("unverified evidence signed")
			}
			cancel()
			receive(t, done)
		})
	}
}
func TestNativeBrokerUnknownTeardownBlocksNewWorkAndKeepsMarker(t *testing.T) {
	b, h, in := fixtureBroker(t)
	h.stopErr = invalid()
	_, cancel, done := startFixture(t, b, h, in)
	cancel()
	r := receive(t, done)
	if r.Stopped || r.Code != "NATIVE_STOP_UNCONFIRMED" || h.cleans != 0 {
		t.Fatal("unknown teardown treated as clean")
	}
	other := clone(in)
	other.Run.RunID = "abcd1234-1234-4234-8234-123456789abc"
	if b.launch(context.Background(), other, b.config.Node.Path, b.config.Entry).Code != "NATIVE_CAPACITY_OR_DUPLICATE" {
		t.Fatal("work admitted after uncertain stop")
	}
}
func TestNativeBrokerPreparationRefusalDoesNotStartOrSign(t *testing.T) {
	b, h, in := fixtureBroker(t)
	h.prepareErr = invalid()
	if b.launch(context.Background(), in, b.config.Node.Path, b.config.Entry).Code != "NATIVE_PREPARATION_FAILED" || h.stops != 0 || len(b.runs) != 0 {
		t.Fatal("failed preparation retained active execution")
	}
	if b.prove(context.Background(), fixtureChallenge(b, in)).Proof != nil {
		t.Fatal("unstarted run signed")
	}
}

func TestNativeBrokerProofStorageIsBoundedPerRun(t *testing.T) {
	b, h, in := fixtureBroker(t)
	_, cancel, done := startFixture(t, b, h, in)
	defer finishGateway(t, cancel, done)
	for i := 0; i < 16; i++ {
		if b.prove(context.Background(), fixtureChallenge(b, in)).Proof == nil {
			t.Fatal("bounded proof failed")
		}
	}
	if b.prove(context.Background(), fixtureChallenge(b, in)).Proof != nil || len(h.evidence) > 16 {
		t.Fatal("unbounded kernel evidence writes admitted")
	}
}
