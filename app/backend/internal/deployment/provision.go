// Package deployment is used only by the private native operator CLI. No HTTP
// handler or ordinary application identity can install profiles or storage.
package deployment

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type StorageInput struct {
	OrganizationID    string `json:"organization_id"`
	ActorID           string `json:"actor_id"`
	Reason            string `json:"reason"`
	Primary           string `json:"primary"`
	OSSBucket         string `json:"oss_bucket"`
	R2Bucket          string `json:"r2_bucket"`
	LogicalLimitBytes string `json:"logical_limit_bytes"`
	Reviewed          bool   `json:"reviewed"`
}
type InspectionInput struct {
	SkillID          string `json:"skill_id"`
	ProfileID        string `json:"profile_id"`
	Model            string `json:"model"`
	WorkingDirectory string `json:"working_directory"`
	MCPEndpoint      string `json:"mcp_endpoint"`
	APIBaseURL       string `json:"api_base_url,omitempty"`
}
type EvidenceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type AgentInput struct {
	OrganizationID              string                `json:"organization_id"`
	ActorID                     string                `json:"actor_id"`
	Reason                      string                `json:"reason"`
	Reviewed                    bool                  `json:"reviewed"`
	NodePath                    string                `json:"node_path"`
	DescribeScript              EvidenceFile          `json:"describe_script"`
	Inspection                  InspectionInput       `json:"inspection"`
	ExpectedSkillSHA256         string                `json:"expected_skill_sha256"`
	ExpectedConfigurationSHA256 string                `json:"expected_configuration_sha256"`
	MaxInputTokens              int                   `json:"max_input_tokens"`
	MaxOutputTokens             int                   `json:"max_output_tokens"`
	Evaluation                  agent.SkillEvaluation `json:"evaluation"`
	Dataset                     EvidenceFile          `json:"dataset"`
	Results                     EvidenceFile          `json:"results"`
	NativeIsolation             EvidenceFile          `json:"native_isolation"`
	ModelContract               EvidenceFile          `json:"model_contract"`
}
type Inspection struct {
	Profile struct {
		ID                  string  `json:"id"`
		SkillID             string  `json:"skill_id"`
		SkillVersion        string  `json:"skill_version"`
		SkillSHA256         string  `json:"skill_sha256"`
		RuntimeVersion      string  `json:"runtime_version"`
		ConfigurationSHA256 string  `json:"configuration_sha256"`
		Telegram            *string `json:"telegram_configuration_sha256"`
		TelegramWrite       *string `json:"telegram_write_configuration_sha256"`
	} `json:"profile"`
	SkillState   string   `json:"skill_state"`
	MissingTools []string `json:"missing_tools"`
	Ready        bool     `json:"ready_for_controlled_installation"`
	Limits       struct {
		MaxToolCalls   int `json:"max_tool_calls"`
		TimeoutSeconds int `json:"timeout_seconds"`
	} `json:"limits"`
	ActualModelInvoked bool `json:"actual_model_invoked"`
	DatabaseModified   bool `json:"database_modified"`
}

var ErrProvision = errors.New("deployment configuration rejected; inspect private reviewed inputs")

func validSHA(v string) bool { return len(v) == 64 && strings.Trim(v, "0123456789abcdef") == "" }
func privateFile(path string, limit int64) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrProvision
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() < 1 || st.Size() > limit || runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
		return nil, ErrProvision
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrProvision
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrProvision
	}
	return raw, nil
}
func evidence(e EvidenceFile) error {
	if !validSHA(e.SHA256) || !filepath.IsAbs(e.Path) {
		return ErrProvision
	}
	st, err := os.Lstat(e.Path)
	if err != nil || !st.Mode().IsRegular() || st.Size() < 1 || st.Size() > 32<<20 {
		return ErrProvision
	}
	f, err := os.Open(e.Path)
	if err != nil {
		return ErrProvision
	}
	defer f.Close()
	// Bounded external prerequisite files; only their digest is stored in PG.
	raw, err := io.ReadAll(io.LimitReader(f, 32<<20+1))
	if err != nil || len(raw) > 32<<20 || core.Digest(raw) != e.SHA256 {
		return ErrProvision
	}
	return nil
}
func decode(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return ErrProvision
	}
	return nil
}
func base(org, actor, reason string, reviewed bool) error {
	if !core.ValidID(org) || !core.ValidID(actor) || core.Name(reason) != nil || !reviewed {
		return ErrProvision
	}
	return nil
}
func operator(ctx context.Context, tx pgx.Tx, org, actor string) error {
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT current_user<>'copypaste_app' AND has_table_privilege(current_user,'deployment_configuration_receipts','INSERT')`).Scan(&ok); err != nil || !ok {
		return ErrProvision
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1 FOR UPDATE)`, org).Scan(&ok); err != nil || !ok {
		return ErrProvision
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE org_id=$1 AND id=$2 AND status='active' FOR SHARE)`, org, actor).Scan(&ok); err != nil || !ok {
		return ErrProvision
	}
	return nil
}
func prior(ctx context.Context, tx pgx.Tx, org, kind, hash string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, "SELECT id FROM deployment_configuration_receipts WHERE org_id=$1 AND kind=$2 AND input_sha256=$3", org, kind, hash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}
func receipt(ctx context.Context, tx pgx.Tx, id, org, kind, hash, actor, reason string, subject *string, proof any) error {
	_, err := tx.Exec(ctx, `INSERT INTO deployment_configuration_receipts(id,org_id,kind,input_sha256,actor_id,reason,subject_id,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, org, kind, hash, actor, reason, subject, proof)
	return err
}

func ProvisionStorage(ctx context.Context, db *pgxpool.Pool, in StorageInput) (string, error) {
	if base(in.OrganizationID, in.ActorID, in.Reason, in.Reviewed) != nil || in.Primary != "oss" && in.Primary != "r2" {
		return "", ErrProvision
	}
	var limit int64
	if _, err := fmt.Sscan(in.LogicalLimitBytes, &limit); err != nil || limit < 1 || fmt.Sprint(limit) != in.LogicalLimitBytes {
		return "", ErrProvision
	}
	for _, bucket := range []string{in.OSSBucket, in.R2Bucket} {
		if len(bucket) < 3 || len(bucket) > 100 || strings.Trim(bucket, "abcdefghijklmnopqrstuvwxyz0123456789.-") != "" {
			return "", ErrProvision
		}
	}
	raw, _ := json.Marshal(in)
	hash := core.Digest(raw)
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err = operator(ctx, tx, in.OrganizationID, in.ActorID); err != nil {
		return "", err
	}
	if id, e := prior(ctx, tx, in.OrganizationID, "initial_storage", hash); e != nil || id != "" {
		return id, e
	}
	var used bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM storage_epochs WHERE org_id=$1) OR EXISTS(SELECT 1 FROM storage_quotas WHERE org_id=$1) OR EXISTS(SELECT 1 FROM storage_targets WHERE org_id=$1) OR EXISTS(SELECT 1 FROM upload_sessions WHERE org_id=$1) OR EXISTS(SELECT 1 FROM files WHERE org_id=$1)`, in.OrganizationID).Scan(&used); err != nil {
		return "", err
	}
	if used {
		return "", ErrProvision
	}
	id := core.NewID()
	if _, err = tx.Exec(ctx, `INSERT INTO storage_quotas(org_id,logical_limit_bytes) VALUES($1,$2)`, in.OrganizationID, limit); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,1,$2,$3,$4)`, in.OrganizationID, in.Primary, in.ActorID, "initial-empty-storage:"+id); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO storage_targets(org_id,provider,bucket,enabled) VALUES($1,'oss',$2,true),($1,'r2',$3,true)`, in.OrganizationID, in.OSSBucket, in.R2Bucket); err != nil {
		return "", err
	}
	if err = receipt(ctx, tx, id, in.OrganizationID, "initial_storage", hash, in.ActorID, in.Reason, nil, map[string]any{"configuration": in, "cloud_verified": false, "only_initial_empty_storage": true}); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func InspectProfile(ctx context.Context, in AgentInput) (Inspection, error) {
	var out Inspection
	if base(in.OrganizationID, in.ActorID, in.Reason, in.Reviewed) != nil || !core.ValidID(in.Inspection.ProfileID) || !validSHA(in.ExpectedSkillSHA256) || !validSHA(in.ExpectedConfigurationSHA256) || in.MaxInputTokens < 1 || in.MaxInputTokens > 200000 || in.MaxOutputTokens < 1 || in.MaxOutputTokens > 32000 || in.Evaluation.Validate(true) != nil {
		return out, ErrProvision
	}
	if in.Dataset.SHA256 != in.Evaluation.SetSHA256 || in.Results.SHA256 != in.Evaluation.ResultsSHA256 {
		return out, ErrProvision
	}
	for _, e := range []EvidenceFile{in.DescribeScript, in.Dataset, in.Results, in.NativeIsolation, in.ModelContract} {
		if evidence(e) != nil {
			return out, ErrProvision
		}
	}
	if !filepath.IsAbs(in.NodePath) {
		return out, ErrProvision
	}
	st, err := os.Lstat(in.NodePath)
	if err != nil || !st.Mode().IsRegular() {
		return out, ErrProvision
	}
	input, _ := json.Marshal(in.Inspection)
	runctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runctx, in.NodePath, in.DescribeScript.Path)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = io.Discard
	// No inherited provider, DB, signing or task secrets enter static inspection.
	cmd.Env = []string{"PATH=" + filepath.Dir(in.NodePath)}
	if runtime.GOOS == "windows" {
		cmd.Env = append(cmd.Env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	var output bytes.Buffer
	cmd.Stdout = &boundedBuffer{Buffer: &output, Limit: 1 << 20}
	if cmd.Run() != nil {
		return out, ErrProvision
	}
	if json.Unmarshal(output.Bytes(), &out) != nil || !out.Ready || out.SkillState != "published" || len(out.MissingTools) != 0 || out.ActualModelInvoked || out.DatabaseModified || out.Profile.ID != in.Inspection.ProfileID || out.Profile.SkillID != in.Inspection.SkillID || out.Profile.SkillSHA256 != in.ExpectedSkillSHA256 || out.Profile.ConfigurationSHA256 != in.ExpectedConfigurationSHA256 || out.Profile.RuntimeVersion != "codex-sdk-0.164.0" || out.Limits.MaxToolCalls < 1 || out.Limits.MaxToolCalls > 100 || out.Limits.TimeoutSeconds < 30 || out.Limits.TimeoutSeconds > 1800 {
		return out, ErrProvision
	}
	return out, nil
}

type boundedBuffer struct {
	*bytes.Buffer
	Limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.Limit {
		return 0, ErrProvision
	}
	return b.Buffer.Write(p)
}
func InstallProfile(ctx context.Context, db *pgxpool.Pool, in AgentInput, inspection Inspection) (string, error) {
	// Reinspect real package bytes immediately before installation. A caller cannot
	// substitute a hand-authored readiness report for the controlled producer.
	current, err := InspectProfile(ctx, in)
	if err != nil {
		return "", err
	}
	a, _ := json.Marshal(current)
	b, _ := json.Marshal(inspection)
	if !bytes.Equal(a, b) {
		return "", ErrProvision
	}
	raw, _ := json.Marshal(in)
	hash := core.Digest(raw)
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err = operator(ctx, tx, in.OrganizationID, in.ActorID); err != nil {
		return "", err
	}
	if id, e := prior(ctx, tx, in.OrganizationID, "agent_profile", hash); e != nil || id != "" {
		return id, e
	}
	p := current.Profile
	id := core.NewID()
	_, err = tx.Exec(ctx, `INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason,telegram_configuration_sha256,telegram_write_configuration_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,'active',$8,$9,$10,$11,$12,$13,$14,$15)`, p.ID, in.OrganizationID, p.SkillID, p.SkillVersion, p.SkillSHA256, p.RuntimeVersion, p.ConfigurationSHA256, in.MaxInputTokens, in.MaxOutputTokens, current.Limits.MaxToolCalls, current.Limits.TimeoutSeconds, in.ActorID, in.Reason, p.Telegram, p.TelegramWrite)
	if err != nil {
		return "", err
	}
	proof := map[string]any{"profile": p, "limits": current.Limits, "evaluation": in.Evaluation, "inspection_script_sha256": in.DescribeScript.SHA256, "native_isolation_sha256": in.NativeIsolation.SHA256, "model_contract_sha256": in.ModelContract.SHA256, "prerequisites": "operator-attested external evidence; hashes verified, not rerun", "actual_model_invoked": false}
	if err = receipt(ctx, tx, id, in.OrganizationID, "agent_profile", hash, in.ActorID, in.Reason, &p.ID, proof); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

// Run reads a private bounded JSON file; errors never echo DSNs, paths or input.
func Run(ctx context.Context, command, path string) (string, error) {
	raw, err := privateFile(path, 65536)
	if err != nil {
		return "", err
	}
	connection := os.Getenv("CP_MIGRATION_DATABASE_URL")
	cfg, err := pgxpool.ParseConfig(connection)
	if connection == "" || err != nil || cfg.ConnConfig.User == "copypaste_app" {
		return "", ErrProvision
	}
	if os.Getenv("CP_ENV") == "production" && (cfg.ConnConfig.TLSConfig == nil || cfg.ConnConfig.TLSConfig.InsecureSkipVerify || cfg.ConnConfig.TLSConfig.ServerName == "" || len(cfg.ConnConfig.Fallbacks) > 0) {
		return "", ErrProvision
	}
	db, err := platform.OpenDatabase(ctx, connection)
	if err != nil {
		return "", ErrProvision
	}
	defer db.Close()
	if command == "provision-storage" {
		var in StorageInput
		if decode(raw, &in) != nil {
			return "", ErrProvision
		}
		if in.OSSBucket != os.Getenv("CP_OSS_BUCKET") || in.R2Bucket != os.Getenv("CP_R2_BUCKET") || in.Primary != os.Getenv("CP_STORAGE_DRIVER") {
			return "", ErrProvision
		}
		return ProvisionStorage(ctx, db, in)
	}
	if command == "install-agent-profile" {
		var in AgentInput
		if decode(raw, &in) != nil {
			return "", ErrProvision
		}
		inspection, e := InspectProfile(ctx, in)
		if e != nil {
			return "", e
		}
		return InstallProfile(ctx, db, in, inspection)
	}
	return "", ErrProvision
}
