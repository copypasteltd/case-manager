CREATE TABLE folders (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), project_id uuid, parent_id uuid,
    root_scope text NOT NULL, name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    name_key text NOT NULL, classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    state text NOT NULL CHECK(state IN ('active','trashed')), version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,parent_id) REFERENCES folders(org_id,id), CHECK(parent_id IS NULL OR parent_id<>id)
);
CREATE UNIQUE INDEX folders_names ON folders(org_id,root_scope,coalesce(parent_id,'00000000-0000-0000-0000-000000000000'),name_key) WHERE state='active';
CREATE TABLE files (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), folder_id uuid NOT NULL,
    name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200), name_key text NOT NULL,
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    current_version_id uuid, state text NOT NULL CHECK(state IN ('active','trashed')),
    trashed_at timestamptz, legal_hold boolean NOT NULL DEFAULT false, version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,folder_id) REFERENCES folders(org_id,id)
);
CREATE UNIQUE INDEX files_names ON files(org_id,folder_id,name_key) WHERE state='active';
CREATE TABLE file_versions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, file_id uuid NOT NULL, seq bigint NOT NULL,
    base_version_id uuid, object_key text NOT NULL, provider_source_id text NOT NULL, storage_epoch bigint NOT NULL,
    size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 0 AND 120000000000),
    sha256 text NOT NULL CHECK(sha256~'^[0-9a-f]{64}$'), media_type text NOT NULL,
    source text NOT NULL, author_id uuid NOT NULL, executor_id uuid NOT NULL,
    scan_state text NOT NULL CHECK(scan_state IN ('pending','clean','quarantined','unscannable','waived')),
    scan_waiver_ref uuid, availability text NOT NULL CHECK(availability IN ('available','quarantined','missing')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,file_id,id), UNIQUE(file_id,seq), UNIQUE(org_id,object_key),
    FOREIGN KEY(org_id,file_id) REFERENCES files(org_id,id),
    FOREIGN KEY(org_id,file_id,base_version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,executor_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,storage_epoch) REFERENCES storage_epochs(org_id,epoch),
    CHECK(availability<>'available' OR scan_state IN ('clean','waived'))
);
ALTER TABLE files ADD FOREIGN KEY(org_id,id,current_version_id) REFERENCES file_versions(org_id,file_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER file_version_immutable BEFORE UPDATE OF file_id,seq,object_key,provider_source_id,storage_epoch,size_bytes,sha256,media_type,source,author_id,executor_id OR DELETE ON file_versions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE upload_sessions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, owner_id uuid NOT NULL, target_folder_id uuid NOT NULL,
    file_id uuid, base_version_id uuid, name text NOT NULL, media_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 120000000000),
    source_fingerprint text NOT NULL, part_size bigint NOT NULL CHECK(part_size BETWEEN 5242880 AND 536870912),
    part_count integer NOT NULL CHECK(part_count BETWEEN 1 AND 10000),
    provider_upload_id text, draft_key text NOT NULL, frozen_source_id text,
    candidate_key text, candidate_source_id text, committed_version_id uuid,
    storage_epoch bigint NOT NULL,
    state text NOT NULL CHECK(state IN ('created','uploading','finalizing','verifying','ready','committed','aborted','expired','failed')),
    final_digest text CHECK(final_digest IS NULL OR final_digest~'^[0-9a-f]{64}$'),
    last_error text, expires_at timestamptz NOT NULL, version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,draft_key),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,target_folder_id) REFERENCES folders(org_id,id),
    FOREIGN KEY(org_id,file_id) REFERENCES files(org_id,id),
    FOREIGN KEY(org_id,file_id,base_version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,committed_version_id) REFERENCES file_versions(org_id,id),
    FOREIGN KEY(org_id,storage_epoch) REFERENCES storage_epochs(org_id,epoch),
    CHECK((file_id IS NULL AND base_version_id IS NULL) OR (file_id IS NOT NULL AND base_version_id IS NOT NULL))
);
CREATE TABLE upload_parts (
    session_id uuid NOT NULL REFERENCES upload_sessions(id), part_no integer NOT NULL CHECK(part_no>0),
    size_bytes bigint NOT NULL CHECK(size_bytes>0), provider_etag text NOT NULL, acknowledged_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(session_id,part_no)
);
CREATE TABLE file_replicas (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, version_id uuid NOT NULL,
    provider text NOT NULL CHECK(provider IN ('oss','r2')), bucket text NOT NULL, object_key text NOT NULL, source_id text,
    state text NOT NULL CHECK(state IN ('queued','copying','verifying','verified','retry_wait','failed','missing')),
    expected_sha256 text NOT NULL CHECK(expected_sha256~'^[0-9a-f]{64}$'),
    actual_sha256 text, verified_at timestamptz, last_error text,
    version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(version_id,provider,bucket),
    FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id),
    CHECK(state<>'verified' OR (actual_sha256=expected_sha256 AND verified_at IS NOT NULL))
);
CREATE TABLE replica_parts (
    replica_id uuid NOT NULL REFERENCES file_replicas(id), upload_id text NOT NULL,
    part_no integer NOT NULL, source_offset bigint NOT NULL, size_bytes bigint NOT NULL, etag text NOT NULL,
    fencing_token bigint NOT NULL, PRIMARY KEY(replica_id,part_no)
);
CREATE TABLE retention_holds (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), file_id uuid NOT NULL, version_id uuid,
    kind text NOT NULL, reason text NOT NULL, owner_id uuid NOT NULL, valid_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(org_id,file_id) REFERENCES files(org_id,id),
    FOREIGN KEY(org_id,file_id,version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id)
);
CREATE TABLE derived_artifacts (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, version_id uuid NOT NULL, kind text NOT NULL,
    generator_version text NOT NULL, state text NOT NULL, output_key text, text_ref uuid,
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(version_id,kind,generator_version),
    FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TABLE search_documents (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    resource_type text NOT NULL, resource_id uuid NOT NULL, version_id uuid,
    title text NOT NULL, text text NOT NULL, tags text[] NOT NULL DEFAULT '{}', project_id uuid,
    owner_id uuid, classification text NOT NULL, index_version bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,resource_type,resource_id,version_id),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id)
);
CREATE TABLE post_file_links (
    org_id uuid NOT NULL, post_revision_id uuid NOT NULL, file_id uuid NOT NULL, pinned_version_id uuid,
    follow_current boolean NOT NULL, PRIMARY KEY(post_revision_id,file_id),
    FOREIGN KEY(org_id,post_revision_id) REFERENCES post_revisions(org_id,id),
    FOREIGN KEY(org_id,file_id) REFERENCES files(org_id,id),
    FOREIGN KEY(org_id,file_id,pinned_version_id) REFERENCES file_versions(org_id,file_id,id),
    CHECK((follow_current AND pinned_version_id IS NULL) OR (NOT follow_current AND pinned_version_id IS NOT NULL))
);
