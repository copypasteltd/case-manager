CREATE TABLE project_templates (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    key text NOT NULL, name text NOT NULL, revision bigint NOT NULL,
    state text NOT NULL CHECK(state IN ('draft','published','archived')),
    task_and_folder_schema jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,key,revision)
);
CREATE SEQUENCE project_number_seq;
CREATE TABLE projects (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    project_no text NOT NULL, name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    type text NOT NULL DEFAULT 'general', scope text NOT NULL CHECK(length(scope)<=20000),
    owner_id uuid NOT NULL, start_date date, due_date date,
    state text NOT NULL CHECK(state IN ('draft','active','on_hold','closing','archived','cancelled')),
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    template_id uuid, template_version bigint,
    initialization_state text NOT NULL DEFAULT 'complete' CHECK(initialization_state IN ('pending','complete','partial_failed')),
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,project_no),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,template_id) REFERENCES project_templates(org_id,id),
    CHECK(start_date IS NULL OR due_date IS NULL OR due_date>=start_date)
);
CREATE INDEX projects_list ON projects(org_id,state,updated_at DESC,id DESC);
CREATE TABLE project_members (
    org_id uuid NOT NULL, project_id uuid NOT NULL, user_id uuid NOT NULL,
    project_role text NOT NULL CHECK(project_role IN ('owner','manager','member','viewer')),
    authorization_version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(project_id,user_id),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id), FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
CREATE TABLE milestones (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, title text NOT NULL,
    due_at timestamptz, owner_id uuid, state text NOT NULL CHECK(state IN ('open','achieved','missed','cancelled')),
    version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,project_id,id),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id), FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id)
);
CREATE TABLE tasks (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL,
    parent_id uuid, milestone_id uuid, title text NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
    body text NOT NULL DEFAULT '' CHECK(length(body)<=20000), assignee_id uuid,
    priority text NOT NULL CHECK(priority IN ('low','normal','high','urgent')),
    state text NOT NULL CHECK(state IN ('todo','in_progress','in_review','done','blocked','cancelled')),
    resume_state text, blocked_reason text, start_at timestamptz, due_at timestamptz, completed_at timestamptz,
    review_required boolean NOT NULL, sort_order bigint NOT NULL DEFAULT 0,
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,project_id,id),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,assignee_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,project_id,parent_id) REFERENCES tasks(org_id,project_id,id),
    FOREIGN KEY(org_id,project_id,milestone_id) REFERENCES milestones(org_id,project_id,id),
    CHECK(parent_id IS NULL OR parent_id<>id),
    CHECK((state='done' AND completed_at IS NOT NULL) OR (state<>'done' AND completed_at IS NULL)),
    CHECK(state<>'blocked' OR (resume_state IS NOT NULL AND length(trim(blocked_reason))>0)),
    CHECK(start_at IS NULL OR due_at IS NULL OR due_at>=start_at)
);
CREATE INDEX tasks_list ON tasks(org_id,project_id,state,updated_at DESC,id DESC);
CREATE INDEX tasks_assigned ON tasks(org_id,assignee_id,state,due_at);
CREATE TABLE task_dependencies (
    org_id uuid NOT NULL, project_id uuid NOT NULL, predecessor_id uuid NOT NULL, successor_id uuid NOT NULL,
    kind text NOT NULL DEFAULT 'finish_to_start' CHECK(kind='finish_to_start'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(predecessor_id,successor_id),
    FOREIGN KEY(org_id,project_id,predecessor_id) REFERENCES tasks(org_id,project_id,id),
    FOREIGN KEY(org_id,project_id,successor_id) REFERENCES tasks(org_id,project_id,id),
    CHECK(predecessor_id<>successor_id)
);
CREATE TABLE deliverables (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, name text NOT NULL,
    state text NOT NULL CHECK(state IN ('draft','submitted','accepted','changes_requested')),
    acceptance_criteria text NOT NULL, current_revision_id uuid, owner_id uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id)
);
CREATE TABLE deliverable_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, deliverable_id uuid NOT NULL, seq bigint NOT NULL,
    file_version_ids uuid[] NOT NULL CHECK(cardinality(file_version_ids)>0), description text NOT NULL,
    submitted_by uuid NOT NULL, submitted_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,deliverable_id,id), UNIQUE(deliverable_id,seq),
    FOREIGN KEY(org_id,deliverable_id) REFERENCES deliverables(org_id,id),
    FOREIGN KEY(org_id,submitted_by) REFERENCES users(org_id,id)
);
ALTER TABLE deliverables ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES deliverable_revisions(org_id,deliverable_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER deliverable_revision_immutable BEFORE UPDATE OR DELETE ON deliverable_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE acceptance_decisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, revision_id uuid NOT NULL,
    decided_by uuid NOT NULL, decision text NOT NULL CHECK(decision IN ('accept','return')),
    reason text NOT NULL, decided_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(revision_id), FOREIGN KEY(org_id,revision_id) REFERENCES deliverable_revisions(org_id,id),
    FOREIGN KEY(org_id,decided_by) REFERENCES users(org_id,id)
);
CREATE TRIGGER acceptance_immutable BEFORE UPDATE OR DELETE ON acceptance_decisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE risks (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, owner_id uuid NOT NULL,
    title text NOT NULL, description text NOT NULL, severity text NOT NULL CHECK(severity IN ('low','medium','high','critical')),
    probability text, mitigation text NOT NULL DEFAULT '', state text NOT NULL CHECK(state IN ('open','mitigating','closed','realized')),
    due_at timestamptz, issue_id uuid, version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id)
);
CREATE TABLE issues (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, owner_id uuid NOT NULL,
    title text NOT NULL, description text NOT NULL, severity text NOT NULL CHECK(severity IN ('low','medium','high','critical')),
    resolution text NOT NULL DEFAULT '', state text NOT NULL CHECK(state IN ('open','investigating','resolved','closed')),
    due_at timestamptz, version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id)
);
ALTER TABLE risks ADD FOREIGN KEY(org_id,issue_id) REFERENCES issues(org_id,id);
CREATE TABLE change_requests (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, title text NOT NULL,
    state text NOT NULL CHECK(state IN ('draft','in_review','approved','rejected','withdrawn','implemented')),
    scope_delta text NOT NULL, budget_delta numeric(20,6), currency text REFERENCES currency_definitions(code),
    schedule_delta integer, base_project_version bigint NOT NULL, approval_instance_id uuid,
    version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id)
);
CREATE TABLE project_budgets (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, project_id uuid NOT NULL, revision bigint NOT NULL,
    state text NOT NULL CHECK(state IN ('draft','approved','superseded')),
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(org_id,id), UNIQUE(project_id,revision),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id)
);
CREATE TABLE budget_lines (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, budget_id uuid NOT NULL, category text NOT NULL,
    currency text NOT NULL REFERENCES currency_definitions(code), amount numeric(20,6) NOT NULL CHECK(amount>=0),
    FOREIGN KEY(org_id,budget_id) REFERENCES project_budgets(org_id,id)
);
CREATE TABLE project_links (
    org_id uuid NOT NULL, project_id uuid NOT NULL, resource_type text NOT NULL, resource_id uuid NOT NULL, relation text NOT NULL,
    PRIMARY KEY(project_id,resource_type,resource_id,relation), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id)
);
CREATE TABLE posts (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), project_id uuid, author_id uuid NOT NULL,
    type text NOT NULL CHECK(type IN ('project_log','meeting_note','sop','article')),
    title text NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    state text NOT NULL CHECK(state IN ('draft','published','archived')),
    current_revision_id uuid, published_revision_id uuid,
    version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id)
);
CREATE TABLE post_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, post_id uuid NOT NULL, seq bigint NOT NULL, base_revision_id uuid,
    body text NOT NULL CHECK(length(body)<=20000), summary text NOT NULL DEFAULT '', author_id uuid NOT NULL,
    source_refs jsonb NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,post_id,id), UNIQUE(post_id,seq),
    FOREIGN KEY(org_id,post_id) REFERENCES posts(org_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,post_id,base_revision_id) REFERENCES post_revisions(org_id,post_id,id)
);
ALTER TABLE posts ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES post_revisions(org_id,post_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE posts ADD FOREIGN KEY(org_id,id,published_revision_id) REFERENCES post_revisions(org_id,post_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER post_revision_immutable BEFORE UPDATE OR DELETE ON post_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE comments (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    resource_type text NOT NULL, resource_id uuid NOT NULL, author_id uuid NOT NULL,
    body text NOT NULL CHECK(length(trim(body)) BETWEEN 1 AND 20000), revision bigint NOT NULL,
    attachment_versions uuid[] NOT NULL DEFAULT '{}', replaces_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,replaces_id) REFERENCES comments(org_id,id)
);
CREATE TRIGGER comment_revision_immutable BEFORE UPDATE OR DELETE ON comments FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE tags (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    name_key text NOT NULL, display_name text NOT NULL,
    UNIQUE(org_id,id), UNIQUE(org_id,name_key)
);
CREATE TABLE resource_tags (
    org_id uuid NOT NULL, tag_id uuid NOT NULL, resource_type text NOT NULL, resource_id uuid NOT NULL,
    PRIMARY KEY(tag_id,resource_type,resource_id), FOREIGN KEY(org_id,tag_id) REFERENCES tags(org_id,id)
);
