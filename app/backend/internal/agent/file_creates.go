package agent

import (
	"context"
	"encoding/json"
	"slices"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"github.com/jackc/pgx/v5"
)

type FileCreateInput struct {
	FolderID              string `json:"folder_id"`
	ExpectedFolderVersion int64  `json:"expected_folder_version"`
	Name                  string `json:"name"`
	MediaType             string `json:"media_type"`
}

type FileCreateView struct {
	RunID        string            `json:"run_id"`
	IntentSHA256 string            `json:"intent_sha256"`
	Target       fileUpdateTarget  `json:"target"`
	Status       string            `json:"status"`
	Drafts       []FileDraftView   `json:"drafts"`
	OperationID  *string           `json:"operation_id"`
	Result       json.RawMessage   `json:"result"`
	Recovery     *FileRecoveryView `json:"recovery,omitempty"`
}

func fileCreateView(view FileUpdateView) FileCreateView {
	return FileCreateView{RunID: view.RunID, IntentSHA256: view.IntentSHA256, Target: view.Target, Status: view.Status, Drafts: view.Drafts, OperationID: view.OperationID, Result: view.Result, Recovery: view.Recovery}
}

func assignFileIntent(run *Run, i *fileIntentRecord, view FileUpdateView) {
	if i.Mode == "create" {
		v := fileCreateView(view)
		run.FileCreate = &v
	} else {
		run.FileUpdate = &view
	}
}

func fileIntentSourceType(i *fileIntentRecord) string {
	if i.Mode == "create" {
		return "agent_file_create"
	}
	return "agent_file_update"
}

// The digest fixes the target and its ancestry; no cloud credential or object
// identifier enters the model context. Every original reader must admit AI.
func createTargetDigest(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, target fileUpdateTarget, members []string, collision bool) (string, error) {
	f, err := files.AgentCreateTarget(ctx, tx, a, target.FolderID, target.FolderVersion, target.Name, target.MediaType, collision)
	if err != nil {
		return "", err
	}
	if !sameProject(c.ProjectID, f.ProjectID) || classRank(f.Classification) == 0 || classRank(f.Classification) > classRank(c.Classification) || target.Classification != f.Classification {
		return "", core.Missing()
	}
	path := []files.OrganizationEntry{}
	seen := map[string]bool{}
	id := &target.FolderID
	for id != nil {
		if len(path) >= 32 || seen[*id] {
			return "", core.Missing()
		}
		var entry files.OrganizationEntry
		for _, user := range members {
			reader := a
			reader.UserID = user
			entry, err = files.OrganizationMetadata(ctx, tx, reader, "folder", *id, "folder.read")
			if err != nil {
				return "", err
			}
		}
		seen[entry.ID] = true
		path = append(path, entry)
		id = entry.ParentID
	}
	raw, _ := json.Marshal(path)
	return core.Digest(raw), nil
}

func prepareFileCreateIntent(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in FileCreateInput, members []string, request string) (*fileIntentRecord, error) {
	ai := a
	ai.RunID = runID
	f, err := files.AgentCreateTarget(ctx, tx, ai, in.FolderID, in.ExpectedFolderVersion, in.Name, in.MediaType, true)
	if err != nil {
		return nil, err
	}
	target := fileUpdateTarget{Name: in.Name, FolderID: f.ID, FolderVersion: f.Version, Classification: f.Classification, MediaType: in.MediaType}
	digest, err := createTargetDigest(ctx, tx, ai, c, target, members, false)
	if err != nil {
		return nil, err
	}
	intent, _ := json.Marshal(map[string]any{"mode": "create", "run_id": runID, "file_id": "", "base_version_id": "", "file_version": 0,
		"actor_id": a.UserID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "conversation_id": c.ID,
		"conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "target": target, "base_sha256": digest, "request": request})
	return &fileIntentRecord{Mode: "create", FileUpdateView: FileUpdateView{RunID: runID, IntentSHA256: core.Digest(intent), Target: target, Status: "authorized", Drafts: []FileDraftView{}}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, BaseSHA256: digest, Intent: string(intent)}, nil
}

func fileCreateSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, i *fileIntentRecord, members, audience []string) (SourceSnapshot, error) {
	var out SourceSnapshot
	if !slices.Contains(audience, a.UserID) {
		return out, core.Missing()
	}
	for _, user := range members {
		if !slices.Contains(audience, user) {
			return out, core.Missing()
		}
	}
	// Historical access needs current reading rights, not the sender's current
	// write grant or the original directory revision. Execution checks both.
	for _, user := range members {
		reader := a
		reader.UserID = user
		current, err := files.OrganizationMetadata(ctx, tx, reader, "folder", i.Target.FolderID, "folder.read")
		if err != nil {
			return out, err
		}
		if !sameProject(c.ProjectID, current.ProjectID) || classRank(current.Classification) == 0 || classRank(current.Classification) > classRank(c.Classification) {
			return out, core.Missing()
		}
	}
	if i.OperationID != nil {
		var result files.CommitResult
		if json.Unmarshal(i.Result, &result) != nil || !result.Committed || result.Version.BaseVersionID != nil {
			return out, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
		if _, err := contextFile(ctx, tx, a, c, ContextSource{Type: "file", ID: result.File.ID, VersionID: &result.Version.ID}, members); err != nil {
			return out, err
		}
	}
	if core.Digest([]byte(i.Intent)) != i.IntentSHA256 {
		return out, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return fileUpdateSource(i), nil
}
