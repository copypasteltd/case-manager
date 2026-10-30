"""Generate the versioned API contract and the backend's embedded schema copy."""
import json
import re
from pathlib import Path


SCHEMAS = {}
PATHS = {}


def ref(name):
    return {"$ref": "#/components/schemas/" + name}


def obj(properties, required=()):
    value = {"type": "object", "properties": properties, "additionalProperties": False}
    if required:
        value["required"] = list(required)
    return value


def array(items, **kwargs):
    return {"type": "array", "items": items, **kwargs}


def nullable(value):
    return {"anyOf": [value, {"type": "null"}]}


def enum(*values):
    return {"type": "string", "enum": list(values)}


def add(name, fields, required=()):
    SCHEMAS[name] = obj(fields, required)
    return ref(name)


ID = {"type": "string", "format": "uuid"}
TEXT = {"type": "string", "maxLength": 20000}
NAME = {"type": "string", "minLength": 1, "maxLength": 200}
VERSION = {"type": "integer", "minimum": 1, "maximum": 9007199254740991}
CURSOR = {"type": "string", "minLength": 1, "maxLength": 2048}
DATE = {"type": "string", "format": "date"}
TIME = {"type": "string", "format": "date-time"}
HASH = {"type": "string", "pattern": "^[a-f0-9]{64}$"}
MONEY = {"type": "string", "pattern": r"^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$"}
BYTES = {"type": "string", "pattern": r"^(0|[1-9][0-9]*)$"}
CURRENCY = {"type": "string", "pattern": "^[A-Z]{3}$"}
CLASS = enum("internal", "project_restricted", "confidential")
EXPECTED = {"expected_version": VERSION}
RECORD = {"id": ID, "version": VERSION, "created_at": TIME, "updated_at": TIME}

add("Meta", {"request_id": NAME, "server_time": TIME}, ("request_id", "server_time"))
add("Error", {"error": obj({"code": NAME, "message": TEXT,
    "details": array(obj({"field": NAME, "code": NAME, "safe_message": TEXT})),
    "retryable": {"type": "boolean"}}, ("code", "message", "retryable")),
    "meta": ref("Meta")}, ("error", "meta"))
RETENTION_KIND = enum("formal_history", "trash", "draft", "multipart")
POLICY_LEVEL = enum("internal", "project_restricted", "confidential")
POLICY_CHANNEL = enum("web", "wechat", "douyin", "android", "telegram")
add("ClassificationPolicy", {"id": ID, "level": POLICY_LEVEL, "version": VERSION, "ai_allowed": {"type": "boolean"},
    "allowed_channels": {**array(POLICY_CHANNEL), "maxItems": 5, "uniqueItems": True}, "published_by": nullable(ID),
    "base_policy_id": nullable(ID), "reason": nullable(NAME), "created_at": TIME},
    ("id", "level", "version", "ai_allowed", "allowed_channels", "published_by", "base_policy_id", "reason", "created_at"))
add("ClassificationPolicyInput", {"level": POLICY_LEVEL, "expected_version": VERSION, "base_policy_id": ID,
    "ai_allowed": {"type": "boolean"}, "allowed_channels": {**array(enum("web", "wechat", "douyin", "android")), "maxItems": 4, "uniqueItems": True},
    "reason": NAME}, ("level", "expected_version", "base_policy_id", "ai_allowed", "allowed_channels", "reason"))
RETENTION_SECONDS = {"type": "integer", "minimum": 86400, "maximum": 315360000}
OBSERVATION_SECONDS = {"type": "integer", "minimum": 86400, "maximum": 31536000}
retention_policy = {"id": ID, "version": VERSION, "kind": RETENTION_KIND, "enabled": {"type": "boolean"},
    "retain_seconds": RETENTION_SECONDS, "observation_seconds": OBSERVATION_SECONDS}
add("RetentionRule", retention_policy, tuple(retention_policy))
add("RetentionPolicyInput", {"expected_version": {"type": "integer", "minimum": 0},
    **{k: v for k, v in retention_policy.items() if k not in ("id", "version")}, "reason": NAME},
    ("expected_version", "kind", "enabled", "retain_seconds", "observation_seconds", "reason"))
add("RetentionPolicy", {**retention_policy, "published_by": ID, "reason": NAME, "created_at": TIME},
    (*retention_policy, "published_by", "reason", "created_at"))
add("RetentionOverview", {"file_id": ID, "legal_hold": {"type": "boolean"}, "active_hold_count": {"type": "integer", "minimum": 0},
    "can_manage": {"type": "boolean"}, "execution_available": {"type": "boolean"}},
    ("file_id", "legal_hold", "active_hold_count", "can_manage", "execution_available"))
add("RetentionHoldCreate", {"expected_version": VERSION, "version_id": ID, "kind": enum("legal", "business", "investigation"),
    "reason": NAME, "valid_until": TIME}, ("expected_version", "kind", "reason"))
add("RetentionHoldRelease", {"reason": NAME}, ("reason",))
hold = {"id": ID, "file_id": ID, "version_id": nullable(ID), "kind": NAME, "reason": NAME, "owner_id": ID,
    "valid_until": nullable(TIME), "created_at": TIME, "released_at": nullable(TIME), "released_by": nullable(ID),
    "release_reason": nullable(NAME), "active": {"type": "boolean"}}
add("RetentionHold", hold, tuple(hold))
add("RetentionAssessmentInput", {"expected_file_version": VERSION, "reason": NAME}, ("expected_file_version", "reason"))
reference_counts = obj({kind: {"type": "integer", "minimum": 0} for kind in
    ("knowledge_revision", "comment", "approval_snapshot", "contract_revision", "contract_signature", "purchase_revision", "purchase_order", "purchase_receipt", "claim_revision", "cost_revision", "payment_revision", "subscription_revision", "subscription_period_event", "contract_finance_event", "contract_closure", "purchase_variance_revision", "purchase_completion", "deliverable", "project_change_revision", "recovery_set")})
snapshot = {"file_id": ID, "file_version": VERSION, "version_id": ID, "seq": VERSION, "size_bytes": BYTES, "checked_at": TIME,
    "policy": nullable(ref("RetentionRule")), "trash_policy": nullable(ref("RetentionRule")), "retain_until": nullable(TIME), "legal_hold": {"type": "boolean"},
    "reference_counts": reference_counts, "eligible_for_review": {"type": "boolean"}, "execution_available": {"type": "boolean"},
    "blocking_reasons": array(NAME), **{key: {"type": "integer", "minimum": 0} for key in
        ("active_hold_count", "active_restore_count", "active_upload_count", "unexpired_download_count", "incomplete_replica_count", "derived_object_count")}}
add("RetentionSnapshot", snapshot, tuple(snapshot))
add("RetentionAssessment", {"id": ID, "snapshot": ref("RetentionSnapshot"), "snapshot_sha256": HASH, "reason": NAME, "created_at": TIME},
    ("id", "snapshot", "snapshot_sha256", "reason", "created_at"))
cleanup_candidate = {"upload_id": ID, "version": VERSION, "name": TEXT, "size_bytes": BYTES, "state": NAME,
    "created_at": TIME, "expires_at": TIME, "retain_until": nullable(TIME), "policy": nullable(ref("RetentionRule")),
    "source_sha256": HASH, "backend_sha256": {"type":"string","pattern":"^([a-f0-9]{64})?$"},
    "eligible": {"type":"boolean"}, "blocking_reasons": array(NAME)}
add("CleanupCandidate", cleanup_candidate, tuple(cleanup_candidate))
cleanup_selection = obj({"upload_id":ID,"expected_version":VERSION},("upload_id","expected_version"))
add("CleanupCreate", {"items":array(cleanup_selection,minItems=1,maxItems=50),"reason":NAME},("items","reason"))
add("CleanupDecision", {**EXPECTED,"snapshot_sha256":HASH,"reason":NAME},("expected_version","snapshot_sha256","reason"))
add("CleanupSnapshot", {"folder_id":ID,"folder_version":VERSION,"items":array(ref("CleanupCandidate"),minItems=1,maxItems=50)},("folder_id","folder_version","items"))
cleanup_item = {"id":ID,"upload_id":ID,"version":VERSION,"generation":VERSION,
    "state":enum("observing","queued","running","retry_wait","succeeded","failed","blocked"),
    "attempts":{"type":"integer","minimum":0},"last_error":nullable(NAME),"confirmed_at":nullable(TIME),"updated_at":TIME}
add("CleanupItem",cleanup_item,tuple(cleanup_item))
cleanup_plan = {"id":ID,"version":VERSION,"state":enum("observing","approved","cancelled"),"snapshot":ref("CleanupSnapshot"),
    "snapshot_sha256":HASH,"reason":NAME,"created_at":TIME,"ready_after":TIME,"expires_at":TIME,
    "approved_at":nullable(TIME),"approval_reason":nullable(NAME),"cancelled_at":nullable(TIME),"cancellation_reason":nullable(NAME),
    "items":array(ref("CleanupItem"),minItems=1,maxItems=50)}
add("CleanupPlan",cleanup_plan,tuple(cleanup_plan))
cleanup_receipt = {"id":ID,"generation":VERSION,"attempt":VERSION,"outcome":enum("started","confirmed_absent","retry_scheduled","failed","blocked"),
    "error_code":nullable(NAME),"created_at":TIME}
add("CleanupReceipt",cleanup_receipt,tuple(cleanup_receipt))
add("ResourceRef", {"type": NAME, "id": ID, "version_id": nullable(ID)}, ("type", "id"))
add("CommandResult", {"operation_id": ID, "resource": ref("ResourceRef"),
    "committed": {"type": "boolean"}, "state": NAME, "job_id": nullable(ID)},
    ("operation_id", "committed", "state"))
add("Transition", {**EXPECTED, "action": NAME, "reason": NAME},
    ("expected_version", "action", "reason"))
add("User", {**RECORD, "display_name": NAME, "status": enum("invited", "active", "disabled")},
    ("id", "version", "display_name", "status"))
add("UserInvite", {"login_key": NAME, "display_name": NAME}, ("login_key", "display_name"))
TOKEN = {"type": "string", "minLength": 40, "maxLength": 100}
add("LoginInput", {"login": NAME, "password": {"type": "string", "minLength": 1, "maxLength": 1024},
    "device_type": enum("web", "wechat", "douyin", "android")}, ("login", "password", "device_type"))
add("LoginResult", {"step": enum("authenticated", "mfa", "mfa_enroll"), "challenge": TOKEN,
    "access_token": TOKEN, "refresh_token": TOKEN, "csrf_token": TOKEN, "expires_at": TIME,
    "mfa_secret": NAME, "mfa_uri": {"type": "string", "maxLength": 1000},
    "recovery_codes": array(NAME)}, ("step",))
add("MFAStepUpInput", {"code": {"type": "string", "pattern": "^[0-9]{6}$"}}, ("code",))
add("MFAStepUpResult", {"mfa_verified_at": TIME}, ("mfa_verified_at",))
add("MFAInput", {"challenge": TOKEN, "code": {"type": "string", "pattern": "^[0-9]{6}$"}}, ("challenge", "code"))
add("MFARecoveryInput", {"challenge": TOKEN, "code": {"type": "string", "pattern": "^[A-Za-z0-9_-]{20}$"}}, ("challenge", "code"))
add("MFAEnrollInput", {"challenge": TOKEN}, ("challenge",))
add("RefreshInput", {"refresh_token": TOKEN})
add("EmptyInput", {})
add("LogoutResult", {"logged_out": {"type": "boolean"}}, ("logged_out",))
add("InvitationReceipt", {"user": ref("User"), "invitation_id": ID, "expires_at": TIME, "activation_token": TOKEN},
    ("user", "invitation_id", "expires_at"))
add("ActivateInput", {"token": TOKEN, "password": {"type": "string", "minLength": 12, "maxLength": 1024}}, ("token", "password"))
add("ActivationResult", {"activated": {"type": "boolean"}}, ("activated",))
device_session = {"id": ID, "device_type": enum("web", "wechat", "douyin", "android"),
    "started_at": TIME, "rotated_at": TIME, "access_expires_at": TIME, "refresh_expires_at": TIME,
    "revoked_at": nullable(TIME), "status": enum("active", "expired", "revoked"), "is_current": {"type": "boolean"}}
TG_NUMBER = {"type":"string","pattern":r"^[1-9][0-9]{0,15}$"}
TG_CHAT = {"type":"string","pattern":r"^-[1-9][0-9]{0,15}$"}
TG_SKILLS = array(enum("project-briefing","incident-summary"),minItems=1,maxItems=2,uniqueItems=True)
TG_TOPIC = {"type":"integer","minimum":0,"maximum":2147483647}
add("TelegramBot",{"bot_id":TG_NUMBER,"username":NAME},("bot_id","username"))
tg_challenge={"id":ID,"version":VERSION,"bot_id":TG_NUMBER,"telegram_user_id":nullable(TG_NUMBER),"expires_at":TIME,
    "claimed_at":nullable(TIME),"confirmed_at":nullable(TIME),"cancelled_at":nullable(TIME),"created_at":TIME}
add("TelegramChallenge",{**tg_challenge,"start_token":{"type":"string","pattern":r"^[A-Za-z0-9_-]{43}$"},
    "start_url":{"type":"string","maxLength":300,"pattern":r"^https://t\.me/[A-Za-z][A-Za-z0-9_]{4,31}\?start=[A-Za-z0-9_-]{43}$"}},tuple(tg_challenge))
tg_binding={"id":ID,"version":VERSION,"bot_id":TG_NUMBER,"telegram_user_id":TG_NUMBER,"state":enum("active","revoked"),"created_at":TIME,"revoked_at":nullable(TIME)}
add("TelegramBinding",tg_binding,tuple(tg_binding))
tg_status={"ingress_configured":{"type":"boolean"},"execution_available":{"const":False},"delivery_available":{"type":"boolean"},
    "bots":array(ref("TelegramBot")),"bindings":array(ref("TelegramBinding")),"challenges":array(ref("TelegramChallenge"))}
add("TelegramStatus",tg_status,tuple(tg_status))
add("TelegramChallengeCreate",{"bot_id":TG_NUMBER,"reason":NAME},("bot_id","reason"))
add("TelegramChallengeConfirm",{**EXPECTED,"telegram_user_id":TG_NUMBER,"reason":NAME},("expected_version","telegram_user_id","reason"))
add("TelegramReasonInput",{**EXPECTED,"reason":NAME},("expected_version","reason"))
tg_channel={**RECORD,"bot_id":TG_NUMBER,"chat_id":TG_CHAT,"topic_id":TG_TOPIC,"label":NAME,"project_id":nullable(ID),
    "allowed_skill_ids":TG_SKILLS,"default_skill_id":nullable(enum("project-briefing","incident-summary")),"disclosure_mode":{"const":"portal_only"},"delivery_enabled":{"type":"boolean"},"state":enum("active","paused")}
add("TelegramChannel",tg_channel,tuple(tg_channel))
tg_channel_input={"bot_id":TG_NUMBER,"chat_id":TG_CHAT,"topic_id":TG_TOPIC,"label":NAME,"project_id":nullable(ID),"allowed_skill_ids":TG_SKILLS,"default_skill_id":nullable(enum("project-briefing","incident-summary")),"reason":NAME}
tg_channel_input["delivery_enabled"]={"type":"boolean","default":False}
add("TelegramChannelCreate",tg_channel_input,tuple(k for k in tg_channel_input if k not in ("default_skill_id","delivery_enabled")))
tg_patch={**EXPECTED,"label":NAME,"project_id":nullable(ID),"allowed_skill_ids":TG_SKILLS,"default_skill_id":nullable(enum("project-briefing","incident-summary")),"state":enum("active","paused"),"reason":NAME}
tg_patch["delivery_enabled"]={"type":"boolean","default":False}
add("TelegramChannelPatch",tg_patch,tuple(k for k in tg_patch if k not in ("default_skill_id","delivery_enabled")))
tg_request={"id":ID,"bot_id":TG_NUMBER,"channel_id":ID,"channel_version":VERSION,"state":enum("awaiting_executor","dispatched","withdrawn"),"requested_skill_id":nullable(enum("project-briefing","incident-summary")),"conversation_id":nullable(ID),"run_id":nullable(ID),"run_state":nullable(enum("queued","running","awaiting_approval","committing","succeeded","failed","cancelled","interrupted")),"reason_code":NAME,"text":{"type":"string","minLength":1,"maxLength":4000},"received_at":TIME}
add("TelegramRequest",tg_request,tuple(tg_request))
TG_DELIVERY_STATE=enum("pending","sending","retry_wait","sent","delivery_unknown","confirmed_sent","failed","superseded","suppressed","abandoned")
tg_delivery={**RECORD,"request_id":ID,"run_id":nullable(ID),"phase":enum("accepted","progress","result"),"state":TG_DELIVERY_STATE,"attempts":{"type":"integer","minimum":0,"maximum":5},"reason_code":NAME,"next_attempt_at":TIME,"message_id":nullable({"type":"integer","minimum":1,"maximum":9007199254740991})}
add("TelegramDelivery",tg_delivery,tuple(tg_delivery))
tg_delivery_event={"id":ID,"delivery_id":ID,"version":VERSION,"state":TG_DELIVERY_STATE,"attempts":{"type":"integer","minimum":0,"maximum":5},"reason_code":NAME,"message_id":tg_delivery["message_id"],"actor_id":nullable(ID),"reason":nullable(NAME),"created_at":TIME}
add("TelegramDeliveryEvent",tg_delivery_event,tuple(tg_delivery_event))
add("TelegramDeliveryDetails",{"deliveries":array(ref("TelegramDelivery")),"events":array(ref("TelegramDeliveryEvent"))},("deliveries","events"))
add("TelegramDeliveryReconcile",{**EXPECTED,"action":enum("confirm_sent","retry_confirmed_not_sent","abandon"),"message_id":tg_delivery["message_id"],"reason":NAME},("expected_version","action","message_id","reason"))
tg_revision={"id":ID,"channel_id":ID,"version":VERSION,"source":enum("portal","membership","migration"),"reason":NAME,"snapshot":ref("TelegramChannel"),"created_at":TIME}
add("TelegramChannelRevision",tg_revision,tuple(tg_revision))
add("TelegramWebhookAck",{"ok":{"const":True}},("ok",))

add("DeviceSession", device_session, tuple(device_session))
add("SessionRevokeInput", {"reason": NAME}, ("reason",))
add("SessionRevocation", {"id": ID, "revoked_at": TIME}, ("id", "revoked_at"))
PASSWORD = {"type": "string", "minLength": 12, "maxLength": 1024}
add("PasswordChangeInput", {"current_password": {"type": "string", "minLength": 1, "maxLength": 1024}, "password": PASSWORD}, ("current_password", "password"))
add("PasswordChanged", {"changed": {"type": "boolean"}}, ("changed",))
add("PasswordResetInput", {**EXPECTED, "reason": NAME}, ("expected_version", "reason"))
add("PasswordResetReceipt", {"id": ID, "user_id": ID, "expires_at": TIME, "reset_token": TOKEN}, ("id", "user_id", "expires_at"))
add("CompletePasswordResetInput", {"token": TOKEN, "password": PASSWORD}, ("token", "password"))
add("Role", {**RECORD, "key": NAME, "name": NAME, "requires_mfa": {"type": "boolean"}, "actions": array(NAME)},
    ("id", "version", "key", "name", "requires_mfa", "actions"))
add("RoleBindingCreate", {"user_id": ID, "role_id": ID, "scope_type": enum("org", "project", "resource", "own"),
    "scope_id": nullable(ID), "classification_ceiling": CLASS, "valid_until": TIME, "reason": NAME},
    ("user_id", "role_id", "scope_type", "classification_ceiling", "reason"))
add("RoleBinding", {"id": ID, "user_id": ID, "role_id": ID, "scope_type": NAME, "scope_id": nullable(ID), "revoked_at": nullable(TIME),
    "classification_ceiling": CLASS, "valid_from": TIME, "valid_until": nullable(TIME), "version": VERSION},
    ("id", "user_id", "role_id", "scope_type", "scope_id", "classification_ceiling", "valid_from", "valid_until", "version"))
add("RoleBindingRevoke", {"expected_version": VERSION, "reason": NAME}, ("expected_version", "reason"))
add("RevocationResult", {"revoked": {"type": "boolean"}}, ("revoked",))
add("Me", {"user": ref("User"), "actions": array(NAME), "authorization_version": VERSION, "session_expires_at": TIME, "mfa_verified_at": nullable(TIME)},
    ("user", "actions", "authorization_version", "session_expires_at", "mfa_verified_at"))
add("Capability", {"platform": enum("web", "weixin", "douyin", "android"),
    "max_upload_bytes": BYTES, "agent_enabled": {"type": "boolean"}},
    ("platform", "max_upload_bytes", "agent_enabled"))

template_task = obj({"title": NAME, "body": TEXT, "priority": enum("low", "normal", "high", "urgent"), "review_required": {"type": "boolean"}}, ("title", "body", "priority", "review_required"))
add("ProjectTemplateSchema", {"folders": array(NAME, maxItems=100), "tasks": array(template_task, maxItems=200), "milestones": array(obj({"title": NAME}, ("title",)), maxItems=100)}, ("folders", "tasks", "milestones"))
template_content = {"name": NAME, "description": {"type": "string", "maxLength": 2000}, "schema": ref("ProjectTemplateSchema")}
template_key = {"type": "string", "pattern": "^[a-z][a-z0-9-]{0,63}$"}
add("ProjectTemplateCreate", {**template_content, "key": template_key, "reason": NAME}, ("name", "description", "schema", "key", "reason"))
add("ProjectTemplatePatch", {**template_content, **EXPECTED, "reason": NAME}, ("name", "description", "schema", "expected_version", "reason"))
add("ProjectTemplateAction", {**EXPECTED, "content_hash": HASH, "reason": NAME}, ("expected_version", "content_hash", "reason"))
add("ProjectTemplate", {**RECORD, **template_content, "key": template_key, "revision": VERSION, "state": enum("draft", "published", "archived"), "content_hash": HASH,
    "creator_id": nullable(ID), "based_on_id": nullable(ID), "published_at": nullable(TIME), "published_by": nullable(ID), "retired_at": nullable(TIME), "retired_by": nullable(ID)},
    ("id", "version", "created_at", "updated_at", "name", "description", "schema", "key", "revision", "state", "content_hash", "creator_id", "based_on_id", "published_at", "published_by", "retired_at", "retired_by"))
add("ProjectTemplateEvent", {"id": ID, "template_id": ID, "version": VERSION, "kind": enum("created", "revised", "updated", "published", "retired"), "actor_id": ID, "reason": NAME, "content_hash": HASH, "snapshot": ref("ProjectTemplate"), "created_at": TIME},
    ("id", "template_id", "version", "kind", "actor_id", "reason", "content_hash", "snapshot", "created_at"))

project_fields = {"name": NAME, "scope": TEXT, "type": NAME, "owner_id": ID,
    "member_ids": array(ID, uniqueItems=True), "start_date": DATE, "due_date": DATE,
    "classification": CLASS, "template_id": ID}
add("ProjectCreate", project_fields, ("name", "scope", "owner_id", "classification"))
add("ProjectPatch", {**EXPECTED, **{k: v for k, v in project_fields.items()
    if k not in ("member_ids", "template_id")}}, ("expected_version",))
SCHEMAS["ProjectPatch"]["properties"].update({"start_date": nullable(DATE), "due_date": nullable(DATE)})
add("Project", {**RECORD, **project_fields, "creator_id": ID, "project_no": NAME, "template_version": VERSION,
    "state": enum("draft", "active", "on_hold", "closing", "archived", "cancelled"),
    "initialization_state": enum("pending", "complete", "partial_failed")},
    ("id", "version", "name", "scope", "owner_id", "state", "project_no"))
task_fields = {"title": NAME, "body": TEXT, "assignee_id": nullable(ID),
    "parent_id": nullable(ID), "milestone_id": nullable(ID), "start_at": nullable(TIME), "due_at": nullable(TIME),
    "priority": enum("low", "normal", "high", "urgent"), "review_required": {"type": "boolean"}}
add("TaskCreate", task_fields, ("title", "priority", "review_required"))
add("TaskPatch", {**EXPECTED, **task_fields}, ("expected_version",))
add("Task", {**RECORD, **task_fields, "creator_id": ID, "project_id": ID,
    "state": enum("todo", "in_progress", "in_review", "done", "blocked", "cancelled")},
    ("id", "version", "project_id", "title", "state"))
add("TaskDependency", {**EXPECTED, "predecessor_id": ID, "action": enum("add", "remove")},
    ("expected_version", "predecessor_id", "action"))
add("TaskDependencies", {"items": array(obj({"predecessor_id": ID, "title": NAME, "state": NAME},
    ("predecessor_id", "title", "state")))}, ("items",))
add("ProjectMembers", {"items": array(ref("User")), "memberships": array(obj({"user_id": ID,
    "project_role": enum("owner", "manager", "member", "viewer"), "authorization_version": VERSION},
    ("user_id", "project_role", "authorization_version")))}, ("items",))
add("ProjectMemberChange", {**EXPECTED, "user_id": ID, "project_role": enum("manager", "member", "viewer"),
    "action": enum("set", "remove"), "reason": NAME}, ("expected_version", "user_id", "action", "reason"))
COUNT = {"type": "integer", "minimum": 0}
add("Dashboard", {"as_of": TIME, "active_projects": COUNT, "my_open_tasks": COUNT, "my_overdue_tasks": COUNT,
    "tasks": array(ref("Task"))}, ("as_of", "active_projects", "my_open_tasks", "my_overdue_tasks", "tasks"))
post_fields = {"project_id": nullable(ID), "type": enum("project_log", "meeting_note", "sop", "article"),
    "title": NAME, "body": TEXT, "summary": TEXT, "tags": array({"type": "string", "minLength": 1, "maxLength": 50}, maxItems=20, uniqueItems=True),
    "classification": CLASS, "source_refs": array(ref("ResourceRef"), maxItems=100)}
add("PostCreate", post_fields, ("type", "title", "body", "classification"))
add("PostPatch", {**EXPECTED, **post_fields, "base_revision_id": ID}, ("expected_version", "base_revision_id"))
add("Post", {**RECORD, **post_fields, "author_id": ID, "current_revision_id": ID, "content_hash": HASH,
    "published_revision_id": nullable(ID), "has_unpublished_changes": {"type": "boolean"},
    "can_edit": {"type": "boolean"}, "can_read_history": {"type": "boolean"}, "can_publish": {"type": "boolean"}, "can_comment": {"type": "boolean"},
    "state": enum("draft", "published", "archived")}, ("id", "version", "title", "state", "current_revision_id"))
add("PostTransition", {**EXPECTED, "base_revision_id": ID, "action": enum("publish", "archive", "draft"), "reason": NAME},
    ("expected_version", "base_revision_id", "action", "reason"))
add("PostRestore", {**EXPECTED, "base_revision_id": ID, "revision_id": ID, "reason": NAME},
    ("expected_version", "base_revision_id", "revision_id", "reason"))
add("PostRevision", {"id": ID, "post_id": ID, "seq": VERSION, "base_revision_id": nullable(ID),
    "title": {"type": "string", "maxLength": 200}, "body": TEXT, "summary": TEXT, "tags": array(NAME),
    "author_id": ID, "created_at": TIME, "source_refs": array(ref("ResourceRef"))},
    ("id", "post_id", "seq", "title", "body", "author_id", "created_at"))
add("Comment", {"id": ID, "resource_type": enum("post", "task"), "resource_id": ID, "author_id": ID, "author_display_name": NAME,
    "body": TEXT, "revision": VERSION, "attachment_versions": array(ID), "replaces_id": nullable(ID),
    "context_revision_id": nullable(ID), "mentioned_user_ids": array(ID, maxItems=20, uniqueItems=True), "mentions_filtered": {"type": "boolean"}, "created_at": TIME},
    ("id", "resource_type", "resource_id", "author_id", "author_display_name", "body", "revision", "attachment_versions", "created_at"))
add("MentionCandidate", {"id": ID, "display_name": NAME}, ("id", "display_name"))
add("MentionCandidates", {"items": array(ref("MentionCandidate")), "next_cursor": nullable(CURSOR)}, ("items", "next_cursor"))
add("Comments", {"items": array(ref("Comment")), "next_cursor": nullable(CURSOR)}, ("items", "next_cursor"))
add("CommentCreate", {"body": {"type": "string", "minLength": 1, "maxLength": 20000},
    "attachment_versions": array(ID, maxItems=20, uniqueItems=True), "mentioned_user_ids": array(ID, maxItems=20, uniqueItems=True), "expected_context_revision_id": ID}, ("body",))
add("PostCommentCreate", SCHEMAS["CommentCreate"]["properties"], ("body", "expected_context_revision_id"))
add("CommentPatch", {**SCHEMAS["CommentCreate"]["properties"], "expected_revision": VERSION}, ("body", "expected_revision"))

add("FolderCreate", {"parent_id": nullable(ID), "project_id": nullable(ID), "name": NAME,
    "classification": CLASS}, ("name", "classification"))
add("Folder", {**RECORD, **SCHEMAS["FolderCreate"]["properties"], "creator_id": nullable(ID),
    "state": enum("active", "trashed"), "can_create": {"type": "boolean"}, "can_update": {"type": "boolean"}, "can_upload": {"type": "boolean"}},
    (*RECORD, "parent_id", "project_id", "creator_id", "name", "classification", "state", "can_create", "can_update", "can_upload"))
add("Move", {**EXPECTED, "target_folder_id": ID, "new_name": NAME, "plan_id": ID,
    "plan_hash": HASH, "reason": NAME}, ("expected_version", "reason"))
SCHEMAS["Move"]["anyOf"] = [{"required": ["target_folder_id"]}, {"required": ["new_name"]}]
SCHEMAS["Move"]["dependentRequired"] = {"plan_id": ["plan_hash"], "plan_hash": ["plan_id"]}
add("File", {**RECORD, "folder_id": ID, "project_id": nullable(ID), "creator_id": nullable(ID), "name": NAME, "classification": CLASS,
    "current_version_id": nullable(ID), "state": enum("active", "trashed"), "legal_hold": {"type": "boolean"},
    **{name: {"type": "boolean"} for name in ("can_update", "can_move", "can_trash", "can_restore", "can_restore_history", "can_view_history_restores")}},
    (*RECORD, "folder_id", "project_id", "creator_id", "name", "classification", "current_version_id", "state", "legal_hold",
        "can_update", "can_move", "can_trash", "can_restore", "can_restore_history", "can_view_history_restores"))
REPLICA_STATE = enum("queued", "copying", "verifying", "verified", "retry_wait", "failed", "missing")
add("FileReplica", {"id": ID, "version": VERSION, "provider": enum("oss", "r2"), "state": REPLICA_STATE, "actual_sha256": nullable(HASH),
    "verified_at": nullable(TIME), "updated_at": TIME, "error_code": nullable(NAME)},
    ("id", "version", "provider", "state", "actual_sha256", "verified_at", "updated_at", "error_code"))
add("ReplicaRetry", {**EXPECTED, "reason": NAME}, ("expected_version", "reason"))
add("FileVersion", {"id": ID, "file_id": ID, "seq": VERSION, "base_version_id": nullable(ID),
    "restored_from_version_id": nullable(ID), "original_name": nullable(NAME), "size_bytes": BYTES, "sha256": HASH, "media_type": NAME, "created_at": TIME,
    "source": NAME, "author_id": ID, "executor_id": ID, "scan_state": enum("pending", "clean", "blocked", "unscannable", "waived"),
    "scan_waiver_ref": nullable(ID),
    "availability": enum("available", "missing", "quarantined"),
    "replica_state": nullable(REPLICA_STATE), "replicas": array(ref("FileReplica"))},
    ("id", "file_id", "seq", "base_version_id", "original_name", "size_bytes", "sha256", "media_type", "created_at",
        "source", "author_id", "executor_id", "scan_state", "scan_waiver_ref", "availability", "replica_state", "replicas", "restored_from_version_id"))
add("HistoryRestoreCreate", {**EXPECTED,"source_version_id":ID,"reason":NAME}, ("expected_version","source_version_id","reason"))
add("HistoryRestoreCommit", {**EXPECTED,"base_version_id":ID,"sha256":HASH}, ("expected_version","base_version_id","sha256"))
add("HistoryRestoreCancel", {**EXPECTED,"reason":NAME}, ("expected_version","reason"))
add("HistoryRestore", {**RECORD,"file_id":ID,"source_seq":VERSION,"base_seq":VERSION,"source_version_id":ID,"base_version_id":ID,"result_version_id":nullable(ID),
    "state":enum("queued","copying","verifying","retry_wait","ready","committed","cancelled","failed"),"size_bytes":BYTES,
    "sha256":HASH,"copied_bytes":BYTES,"verified_bytes":BYTES,"error_code":nullable(NAME),"reason":NAME,"expires_at":TIME},
    (*RECORD,"file_id","source_seq","base_seq","source_version_id","base_version_id","result_version_id","state","size_bytes","sha256","copied_bytes",
        "verified_bytes","error_code","reason","expires_at"))
add("UploadCreate", {"folder_id": ID, "name": NAME, "size_bytes": BYTES, "media_type": NAME,
    "file_id": ID, "base_version_id": ID, "source_fingerprint": NAME},
    ("folder_id", "name", "size_bytes", "media_type", "source_fingerprint"))
SCHEMAS["UploadCreate"]["description"] = "file_id and base_version_id must both be present for an update, both absent for a new file. Max bytes=120000000000."
SCHEMAS["UploadCreate"]["dependentRequired"] = {"file_id": ["base_version_id"], "base_version_id": ["file_id"]}
add("Upload", {**RECORD, "folder_id": ID, "file_id": nullable(ID), "base_version_id": nullable(ID),
    "name": NAME, "media_type": NAME, "source_fingerprint": NAME, "error_code": nullable(NAME),
    "verified_bytes": BYTES, "scan_state": enum("pending", "clean", "blocked", "unscannable", "waived"), "scan_result_id": nullable(ID), "scan_waiver_ref": nullable(ID),
    "size_bytes": BYTES, "part_bytes": {"type": "integer", "minimum": 5242880, "maximum": 536870912},
    "part_count": {"type": "integer", "minimum": 1, "maximum": 10000},
    "confirmed_parts": array({"type": "integer", "minimum": 1, "maximum": 10000}, uniqueItems=True),
    "state": enum("created", "uploading", "paused", "finalizing", "verifying", "blocked", "ready", "committed", "expired", "aborted", "failed"),
    "storage_epoch": VERSION, "expires_at": TIME, "sha256": nullable(HASH), "result_version_id": nullable(ID), "result_file_id": nullable(ID)},
    (*RECORD, "folder_id", "file_id", "base_version_id", "name", "media_type", "source_fingerprint",
        "size_bytes", "part_bytes", "part_count", "state", "storage_epoch", "confirmed_parts",
        "expires_at", "sha256", "result_version_id", "result_file_id", "error_code", "verified_bytes", "scan_state", "scan_result_id", "scan_waiver_ref"))
add("UploadCapabilities", {"control_ready": {"type": "boolean"}, "reason_code": nullable(NAME), "max_bytes": BYTES,
    "session_hours": {"const": 48}}, ("control_ready", "reason_code", "max_bytes", "session_hours"))
add("UploadScanReport", {"id": ID, "upload_id": ID, "sha256": HASH, "size_bytes": BYTES,
    "state": enum("clean", "blocked", "unscannable"), "code": NAME, "engine": nullable(NAME),
    "engine_version": nullable(NAME), "signature_revision": nullable(NAME), "policy_sha256": nullable(HASH),
    "streamed_bytes": BYTES, "created_at": TIME},
    ("id", "upload_id", "sha256", "size_bytes", "state", "code", "engine", "engine_version", "signature_revision",
        "policy_sha256", "streamed_bytes", "created_at"))
add("SignParts", {**EXPECTED, "part_numbers": array({"type": "integer", "minimum": 1, "maximum": 10000},
    minItems=1, maxItems=20, uniqueItems=True)}, ("expected_version", "part_numbers"))
add("SignedParts", {"items": array(obj({"part_number": {"type": "integer"}, "method": enum("PUT"),
    "url": {"type": "string", "format": "uri"}, "headers": array(obj({"name": NAME, "value": TEXT}, ("name", "value"))),
    "expires_at": TIME}, ("part_number", "method", "url", "expires_at")))}, ("items",))
add("AckParts", {**EXPECTED, "parts": array(obj({"part_number": {"type": "integer", "minimum": 1, "maximum": 10000},
    "etag": NAME, "size_bytes": BYTES}, ("part_number", "etag", "size_bytes")), minItems=1, maxItems=100)},
    ("expected_version", "parts"))
add("UploadFinalize", EXPECTED, ("expected_version",))
waiver_policy_fields = {"classification": enum("internal", "project_restricted"), "enabled": {"type": "boolean"},
    "media_types": array(NAME, minItems=1, maxItems=20, uniqueItems=True),
    "allowed_codes": array(enum("SCANNER_NOT_CONFIGURED", "SCAN_SIZE_LIMIT", "SCAN_ENGINE_SIZE_LIMIT", "SCAN_CONTENT_LIMIT"), minItems=1, maxItems=4, uniqueItems=True),
    "max_bytes": BYTES, "reviewer_id": ID, "reason": NAME}
add("ScanWaiverPolicyInput", {"expected_version": {"type": "integer", "minimum": 0, "maximum": 9007199254740991},
    **waiver_policy_fields}, ("expected_version", *waiver_policy_fields))
add("ScanWaiverPolicy", {"id": ID, "version": VERSION, "created_at": TIME, "published_by": ID,
    **waiver_policy_fields}, ("id", "version", "created_at", "published_by", *waiver_policy_fields))
add("ScanWaiverCreate", {**EXPECTED, "scan_report_id": ID, "sha256": HASH, "reason": NAME, "acknowledged_risk": {"const": True}},
    ("expected_version", "scan_report_id", "sha256", "reason", "acknowledged_risk"))
waiver_fields = {**RECORD, "upload_id": ID, "scan_report_id": ID, "policy_id": ID, "policy_version": VERSION,
    "upload_version": VERSION, "owner_id": ID, "reviewer_id": ID, "classification": enum("internal", "project_restricted"),
    "name": NAME, "media_type": NAME, "size_bytes": BYTES, "sha256": HASH, "scan_code": NAME, "reason": NAME,
    "state": enum("pending", "approved", "rejected", "cancelled"), "decision_reason": nullable(NAME), "expires_at": TIME}
add("ScanWaiver", waiver_fields, tuple(waiver_fields))
add("ScanWaiverDecision", {**EXPECTED, "upload_version": VERSION, "scan_report_id": ID, "sha256": HASH,
    "decision": enum("approve", "reject"), "reason": NAME, "acknowledged_risk": {"type": "boolean"}},
    ("expected_version", "upload_version", "scan_report_id", "sha256", "decision", "reason"))
SCHEMAS["ScanWaiverDecision"]["allOf"] = [{"if": {"properties": {"decision": {"const": "approve"}}},
    "then": {"required": ["acknowledged_risk"], "properties": {"acknowledged_risk": {"const": True}}}}]
add("ResumeUpload", {**EXPECTED, "source_fingerprint": NAME}, ("expected_version", "source_fingerprint"))
add("UploadCommit", {**EXPECTED, "base_version_id": ID, "sha256": HASH}, ("expected_version", "sha256"))
add("FileCommitResult", {"file": ref("File"), "file_version": ref("FileVersion"),
    "committed": {"const": True}}, ("file", "file_version", "committed"))
add("RestoreFile", {**EXPECTED, "mode": enum("untrash", "new_version_from_history"), "source_version_id": ID,
    "reason": NAME}, ("expected_version", "mode", "reason"))
SCHEMAS["RestoreFile"]["allOf"] = [
    {"if": {"properties": {"mode": {"const": "new_version_from_history"}}},
     "then": {"required": ["source_version_id"]}},
    {"if": {"properties": {"mode": {"const": "untrash"}}},
     "then": {"not": {"required": ["source_version_id"]}}}]
add("DownloadRequest", {"disposition": enum("inline", "attachment")}, ("disposition",))
add("DownloadResult", {"id": ID, "mode": enum("signed_url", "authenticated_proxy"), "url": {"type": "string", "format": "uri"},
    "method": {"const": "GET"}, "required_headers": array(obj({"name": NAME, "value": TEXT}, ("name", "value")), maxItems=10),
    "expires_at": TIME, "permit_expires_at": TIME, "file_version_id": ID, "filename": NAME, "size_bytes": BYTES,
    "sha256": HASH, "scan_state": enum("clean", "waived"), "disposition": enum("inline", "attachment"),
    "max_range_bytes": BYTES, "warnings": array(enum("SCAN_WAIVED_NOT_CLEAN", "SIGNED_URL_REVOCATION_DELAY"), maxItems=2)},
    ("id", "mode", "url", "method", "required_headers", "expires_at", "permit_expires_at", "file_version_id", "filename",
    "size_bytes", "sha256", "scan_state", "disposition", "max_range_bytes", "warnings"))
text_states=enum("pending","ready","unsupported","blocked","failed")
file_text={"file_id":ID,"version_id":ID,"extractor":enum("text-v1","office-v1","docx-v2"),"state":text_states,
    "source_sha256":HASH,"text_sha256":nullable(HASH),"indexed_at":nullable(TIME),"error_code":nullable(NAME),
    "total_characters":nullable({"type":"integer","minimum":0}),"offset":{"type":"integer","minimum":0},
    "text":{"type":"string","maxLength":8000},"next_offset":nullable({"type":"integer","minimum":1})}
add("FileText",file_text,tuple(file_text))
comparison_version = {"version_id": ID, "extractor":enum("text-v1","office-v1","docx-v2"), "seq": VERSION, "media_type": NAME, "size_bytes": BYTES,
    "source_sha256": HASH, "text_sha256": HASH, "text_characters": {"type":"integer","minimum":0,"maximum":2097152}, "indexed_at": TIME}
add("FileTextComparisonVersion", comparison_version, tuple(comparison_version))
comparison_line = {"type":"integer","minimum":1,"maximum":40000}
comparison_row = {"kind":enum("equal","delete","insert","skip"), "base_line":nullable(comparison_line),
    "target_line":nullable(comparison_line), "text":{"type":"string","maxLength":2097152},
    "newline":{"type":"boolean"},"count":comparison_line}
add("FileTextComparisonRow", comparison_row, tuple(comparison_row))
comparison_count = {"type":"integer","minimum":0,"maximum":80000}
comparison = {"file_id":ID,"file_version":VERSION,"extractor":enum("text-v1","office-v1","docx-v2","mixed"),
    "base":ref("FileTextComparisonVersion"),"target":ref("FileTextComparisonVersion"),
    "source_equal":{"type":"boolean"},"text_equal":{"type":"boolean"},
    "algorithm":enum("myers-v1","bounded-replacement-v1"),"complete":{"const":True},"view":enum("changes","all"),
    "base_lines":comparison_count,"target_lines":comparison_count,"added_lines":comparison_count,
    "removed_lines":comparison_count,"unchanged_lines":comparison_count,"comparison_sha256":HASH,
    "offset":comparison_count,"base_offset":comparison_count,"target_offset":comparison_count,"total_rows":comparison_count,"rows":array(ref("FileTextComparisonRow"),maxItems=100),
    "next_offset":nullable(comparison_count),"as_of":TIME}
add("FileTextComparison",comparison,tuple(comparison))
search_hit={"resource": ref("ResourceRef"), "title": NAME, "snippet": {"type":"string","maxLength":240}, "updated_at": TIME,
    "project_id":nullable(ID),"match_types":array(enum("name","body","tags"),minItems=1,maxItems=3,uniqueItems=True),
    "text_state":nullable(text_states),"indexed_at":nullable(TIME)}
add("SearchHit",search_hit,tuple(search_hit))
download_chunk = {"download_id": ID, "file_version_id": ID, "offset": BYTES, "length": BYTES,
    "total_size_bytes": BYTES, "sha256": HASH, "chunk_sha256": HASH,
    "data_base64": {"type": "string", "maxLength": 1398104, "pattern": "^[A-Za-z0-9+/]*={0,2}$"}}
add("DownloadChunk", download_chunk, tuple(download_chunk))

party_content = {"name": NAME, "kind": enum("organization", "person"),
    "relationships": array(enum("supplier", "customer", "other"), minItems=1, maxItems=3, uniqueItems=True),
    "contact_name": {"type": "string", "maxLength": 200}, "email": {"type": "string", "maxLength": 254},
    "phone": {"type": "string", "maxLength": 100}, "notes": {"type": "string", "maxLength": 10000}}
add("PartyCreate", {**party_content, "project_id": nullable(ID), "classification": CLASS, "reason": NAME},
    ("name", "kind", "relationships", "classification", "reason"))
add("PartyPatch", {**party_content, **EXPECTED, "base_revision_id": ID, "reason": NAME},
    (*party_content, "expected_version", "base_revision_id", "reason"))
add("Party", {**RECORD, **party_content, "project_id": nullable(ID), "creator_id": ID, "classification": CLASS,
    "state": enum("active", "archived"), "current_revision_id": ID, "can_edit": {"type": "boolean"},
    "can_archive": {"type": "boolean"}, "can_read_history": {"type": "boolean"}},
    (*RECORD, *party_content, "project_id", "creator_id", "classification", "state", "current_revision_id", "can_edit", "can_archive", "can_read_history"))
add("PartyRevision", {**party_content, "id": ID, "party_id": ID, "seq": VERSION, "base_revision_id": nullable(ID),
    "author_id": ID, "reason": NAME, "created_at": TIME},
    (*party_content, "id", "party_id", "seq", "base_revision_id", "author_id", "reason", "created_at"))
add("PartyStateInput", {**EXPECTED, "action": enum("archive", "restore"), "reason": NAME},
    ("expected_version", "action", "reason"))

request_kinds = enum("contract", "purchase", "claim", "change", "purchase_variance")
workflow_step = {"title": NAME, "assigned_user_id": ID, "requires_mfa": {"type": "boolean"}}
add("WorkflowStepInput", workflow_step, tuple(workflow_step))
add("WorkflowStep", {**workflow_step, "seq": {"type": "integer", "minimum": 1, "maximum": 10},
    "assigned_user_name": NAME, "assigned_user_active": {"type": "boolean"}},
    (*workflow_step, "seq", "assigned_user_name", "assigned_user_active"))
workflow_fields = {"key": {"type": "string", "pattern": "^[a-z][a-z0-9_.-]{2,63}$"}, "name": NAME, "kind": request_kinds,
    "category": {"type": "string", "minLength": 1, "maxLength": 50}, "currency": CURRENCY, "min_amount": MONEY,
    "max_amount": nullable(MONEY), "enabled": {"type": "boolean"}, "self_approval_policy": enum("deny", "allow"),
    "withdrawal_allowed": {"type": "boolean"}, "steps": array(ref("WorkflowStepInput"), minItems=1, maxItems=10), "reason": NAME}
add("WorkflowDefinitionInput", {"expected_version": {"type": "integer", "minimum": 0}, **workflow_fields},
    ("expected_version", *workflow_fields))
add("WorkflowDefinition", {"id": ID, "version": VERSION, **workflow_fields,
    "steps": array(ref("WorkflowStep"), minItems=1, maxItems=10), "published_by": ID, "created_at": TIME},
    ("id", "version", *workflow_fields, "published_by", "created_at"))


allocation = obj({"project_id": nullable(ID), "amount": MONEY}, ("project_id", "amount"))
cost_fields = {"title": NAME, "occurred_on": nullable(DATE), "currency": nullable(CURRENCY), "amount": nullable(MONEY),
    "category": {"type": "string", "maxLength": 50}, "counterparty": nullable(ref("SpendSupplier")),
    "allocations": array(allocation, maxItems=100), "evidence_version_ids": array(ID, maxItems=20), "notes": TEXT}
add("CostContent", cost_fields, tuple(cost_fields))
cost_base = {**EXPECTED, "revision_id": ID, "content_hash": HASH, "reason": NAME}
add("CostCreate", {**cost_fields, "source_key": {"type": "string", "pattern": r"^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$"},
    "project_id": nullable(ID), "classification": CLASS, "reason": NAME}, (*cost_fields, "source_key", "project_id", "classification", "reason"))
add("CostPatch", {**cost_fields, **cost_base}, (*cost_fields, *cost_base))
cost_party_evidence = add("CostPartyEvidence", {"party_id": ID, "revision_id": ID, "name": NAME}, ("party_id", "revision_id", "name"))
cost_link_fields = {"id": ID, "source_type": enum("purchase", "claim", "contract", "subscription_period"), "source_id": ID, "source_revision_id": ID,
    "relation": enum("incurred_for", "reimbursed_via", "contractual_basis", "subscription_charge"), "title": nullable(NAME), "reason": NAME, "actor_id": ID, "created_at": TIME}
add("CostBusinessLink", cost_link_fields, tuple(cost_link_fields))
cost_record = {**RECORD, **cost_fields, "source_key": NAME, "project_id": nullable(ID), "creator_id": ID, "classification": CLASS,
    "entry_kind": enum("expense", "reversal"), "base_currency": nullable(CURRENCY), "base_amount": nullable(MONEY),
    "state": enum("draft", "confirmed", "reversed"), "reverses_id": nullable(ID), "reversal_id": nullable(ID),
    "current_revision_id": ID, "content_hash": HASH, "counterparty_evidence": nullable(cost_party_evidence),
    "attachments": array(ref("BusinessFileReference")), "links": array(ref("CostBusinessLink"), maxItems=100),
    "paid_amount": nullable(MONEY), "outstanding_amount": nullable(MONEY), "settlement_state": nullable(enum("unsettled", "partial", "settled")), "settlement_visibility": enum("not_confirmed", "restricted", "complete"),
    **{key: {"type": "boolean"} for key in ("can_edit", "can_confirm", "can_reverse", "can_link")}}
add("Cost", cost_record, tuple(cost_record))
add("CostConfirm", cost_base, tuple(cost_base))
add("CostReverse", {**cost_base, "effective_on": DATE}, (*cost_base, "effective_on"))
cost_revision = {**cost_fields, "id": ID, "seq": VERSION, "base_revision_id": nullable(ID), "content_hash": HASH,
    "author_id": ID, "reason": NAME, "created_at": TIME, "counterparty_evidence": nullable(cost_party_evidence), "attachments": array(ref("BusinessFileReference"))}
add("CostRevision", cost_revision, tuple(cost_revision))
cost_event = {"id": ID, "version": VERSION, "revision_id": ID, "kind": enum("draft_created", "draft_revised", "confirmed", "reversed", "link_added", "reversal_created"),
    "state": enum("draft", "confirmed", "reversed"), "actor_id": ID, "reason": NAME, "created_at": TIME}
add("CostEvent", cost_event, tuple(cost_event))
report_decimal = {"type": "string", "pattern": r"^-?[0-9]+(\.[0-9]{1,6})?$"}
cost_total = {"currency": CURRENCY, "expense_amount": report_decimal, "reversal_amount": report_decimal, "net_amount": report_decimal, "entry_count": {"type": "integer", "minimum": 0}}
add("CostTotal", cost_total, tuple(cost_total))
cost_report = {"kind": {"const": "incurred"}, "from": DATE, "to": DATE, "timezone": {"const": "Asia/Hong_Kong"}, "as_of": TIME,
    "base_currency": {"type": "null"}, "fx_coverage": {"const": "unconfigured"}, "totals": array(ref("CostTotal")), "unconfirmed_count": {"type": "integer", "minimum": 0}}
add("CostReport", cost_report, tuple(cost_report))
add("Reverse", {**EXPECTED, "reason": NAME, "effective_on": DATE}, ("expected_version", "reason", "effective_on"))
add("CostLink", {**cost_base, "source_id": ID, "source_revision_id": ID, "source_type": enum("purchase", "claim", "contract"), "relation": enum("incurred_for", "reimbursed_via", "contractual_basis")}, (*cost_base, "source_type", "source_id", "source_revision_id", "relation"))
add("PaymentRefundFX", {"original_currency": CURRENCY, "original_amount": MONEY, "rate": {"type": "string", "pattern": r"^(0|[1-9][0-9]{0,13})(\.[0-9]{1,10})?$"}, "source": NAME}, ("original_currency", "original_amount", "rate", "source"))
payment_fields = {"title": NAME, "direction": nullable(enum("outgoing", "incoming", "refund", "transfer")), "paid_at": nullable(TIME),
    "currency": nullable(CURRENCY), "amount": nullable(MONEY), "category": {"type": "string", "maxLength": 50},
    "counterparty": nullable(ref("SpendSupplier")), "employee_id": nullable(ID), "refund_of_id": nullable(ID),
    "method_label": {"type": "string", "maxLength": 100}, "external_ref": {"type": "string", "maxLength": 200},
    "evidence_version_ids": array(ID, maxItems=20), "notes": TEXT}
payment_fields["refund_fx"] = ref("PaymentRefundFX")
add("PaymentContent", payment_fields, tuple(k for k in payment_fields if k != "refund_fx"))
add("PaymentCreate", {**payment_fields, "source_key": SCHEMAS["CostCreate"]["properties"]["source_key"], "project_id": nullable(ID), "classification": CLASS, "reason": NAME}, (*(k for k in payment_fields if k != "refund_fx"), "source_key", "project_id", "classification", "reason"))
add("PaymentPatch", {**payment_fields, **cost_base}, (*(k for k in payment_fields if k != "refund_fx"), *cost_base))
payment_record = {**RECORD, **payment_fields, "source_key": NAME, "project_id": nullable(ID), "creator_id": ID, "classification": CLASS,
    "record_kind": enum("payment", "reversal"), "state": enum("draft", "recorded", "reversed"), "reverses_id": nullable(ID), "reversal_id": nullable(ID),
    "current_revision_id": ID, "content_hash": HASH, "paid_on": nullable(DATE), "counterparty_evidence": nullable(cost_party_evidence),
    "attachments": array(ref("BusinessFileReference")), "effective_allocated_amount": report_decimal, "refunded_amount": MONEY, "contract_return_reserved_amount": MONEY, "unallocated_amount": nullable(MONEY),
    **{key: {"type": "boolean"} for key in ("can_edit", "can_record", "can_allocate", "can_reverse")}}
add("Payment", payment_record, tuple(k for k in payment_record if k != "refund_fx"))
add("PaymentFXInput", {"cost_amount": MONEY, "rate": {"type": "string", "pattern": r"^(0|[1-9][0-9]{0,13})(\.[0-9]{1,10})?$"}, "source": NAME, "acknowledged": {"type": "boolean", "const": True}}, ("cost_amount", "rate", "source", "acknowledged"))
add("PaymentFXQuote", {"rate": {"type": "string", "pattern": r"^(0|[1-9][0-9]{0,13})(\.[0-9]{1,10})?$"}, "source": NAME, "quoted_on": DATE}, ("rate", "source", "quoted_on"))
refund_allocation = obj({"source_allocation_id": ID, "amount": MONEY, "cost_amount": MONEY}, ("source_allocation_id", "amount"))
add("PaymentRecord", {**cost_base, "expected_source_version": nullable(VERSION), "refund_allocations": array(refund_allocation, maxItems=100), "refund_fx_acknowledged": {"type": "boolean"}}, (*cost_base, "expected_source_version", "refund_allocations"))
add("PaymentReverse", {**cost_base, "effective_at": TIME}, (*cost_base, "effective_at"))
add("PaymentAllocate", {**cost_base, "cost_id": ID, "cost_version": VERSION, "cost_revision_id": ID, "cost_content_hash": HASH, "amount": MONEY,
    "claim_id": nullable(ID), "claim_revision_id": nullable(ID), "claim_line_id": nullable(ID), "fx": ref("PaymentFXInput")}, (*cost_base, "cost_id", "cost_version", "cost_revision_id", "cost_content_hash", "amount", "claim_id", "claim_revision_id", "claim_line_id"))
add("PaymentRelease", {**cost_base, "amount": MONEY, "cost_amount": MONEY}, (*cost_base, "amount"))
add("PaymentRefundAllocate", {**cost_base, "source_allocation_id": ID, "amount": MONEY, "cost_amount": MONEY}, (*cost_base, "source_allocation_id", "amount"))
payment_allocation = {"id": ID, "payment_id": ID, "cost_id": ID, "cost_revision_id": ID, "kind": enum("allocation", "release", "refund", "reversal"),
    "effect": {"type": "integer", "enum": [-1, 1]}, "amount": MONEY, "root_id": ID, "source_allocation_id": nullable(ID), "claim_id": nullable(ID), "claim_revision_id": nullable(ID), "claim_line_id": nullable(ID),
    "shares": array(allocation, maxItems=100), "remaining_amount": MONEY, "actor_id": ID, "reason": NAME, "created_at": TIME}
# New fields are optional in the schema for immutable pre-116 archived tool receipts.
# Current allocation reads always emit both axes; clients require them for FX writes.
payment_allocation_required = tuple(payment_allocation)
payment_allocation.update({"cost_amount": MONEY, "cost_currency": CURRENCY, "payment_currency": CURRENCY,
    "cost_exponent": {"type": "integer", "minimum": 0, "maximum": 6}, "payment_exponent": {"type": "integer", "minimum": 0, "maximum": 6},
    "cost_shares": array(allocation, maxItems=100), "remaining_cost_amount": MONEY, "fx": nullable(ref("PaymentFXQuote"))})
add("PaymentAllocation", payment_allocation, payment_allocation_required)
fee_fact = {"id": ID, "version": VERSION, "revision_id": ID, "content_hash": HASH, "title": NAME,
    "project_id": nullable(ID), "classification": CLASS, "currency": CURRENCY, "amount": MONEY}
add("PaymentFeeFact", fee_fact, tuple(fee_fact))
fee_basis = {"payment": ref("PaymentFeeFact"), "fee_payment": ref("PaymentFeeFact"), "cost": ref("PaymentFeeFact"), "allocation": ref("PaymentAllocation")}
add("PaymentFeeBasis", fee_basis, tuple(fee_basis))
fee_version = {"type": "integer", "minimum": 0, "maximum": 9007199254740991}
fee_preview = {"payment_id": ID, "fee_version": fee_version, "basis": {"type": "string", "minLength": 1, "maxLength": 131072}, "basis_sha256": HASH}
add("PaymentFeePreview", fee_preview, tuple(fee_preview))
fee_revocation = {"id": ID, "version": VERSION, "actor_id": ID, "reason": NAME, "created_at": TIME}
add("PaymentFeeRevocation", fee_revocation, tuple(fee_revocation))
fee_link = {"id": ID, "payment_id": ID, "version": VERSION, "basis": fee_preview["basis"], "basis_sha256": HASH,
    "actor_id": ID, "reason": NAME, "created_at": TIME, "revocation": nullable(ref("PaymentFeeRevocation"))}
add("PaymentFeeLink", fee_link, tuple(fee_link))
fee_item = {"link": ref("PaymentFeeLink"), "current": ref("PaymentAllocation"), "state": enum("active", "exhausted", "revoked")}
add("PaymentFeeItem", fee_item, tuple(fee_item))
fee_ledger = {"payment_id": ID, "fee_version": fee_version, "can_manage": {"type": "boolean"},
    "items": array(ref("PaymentFeeItem"), maxItems=100), "next_cursor": nullable({"type": "string"})}
add("PaymentFeeLedger", fee_ledger, tuple(fee_ledger))
fee_create = {**cost_base, "expected_fee_version": fee_version, "fee_payment_id": ID, "allocation_id": ID, "basis_sha256": HASH, "reviewed": {"const": True, "type": "boolean"}}
add("PaymentFeeCreate", fee_create, tuple(fee_create))
fee_revoke = {**cost_base, "expected_fee_version": fee_version, "basis_sha256": HASH, "reviewed": {"const": True, "type": "boolean"}}
add("PaymentFeeRevoke", fee_revoke, tuple(fee_revoke))
payment_revision = {**payment_fields, "id": ID, "seq": VERSION, "base_revision_id": nullable(ID), "content_hash": HASH, "author_id": ID, "reason": NAME, "created_at": TIME,
    "counterparty_evidence": nullable(cost_party_evidence), "attachments": array(ref("BusinessFileReference"))}
add("PaymentRevision", payment_revision, tuple(k for k in payment_revision if k != "refund_fx"))
payment_event = {**cost_event, "kind": enum("draft_created", "draft_revised", "recorded", "allocation_added", "allocation_released", "refund_allocated", "reversed", "reversal_created"), "state": enum("draft", "recorded", "reversed"), "allocation_id": nullable(ID)}
add("PaymentEvent", payment_event, tuple(payment_event))
claim_settlement = {"claim_id": ID, "revision_id": nullable(ID), "currency": nullable(CURRENCY), "eligible_amount": nullable(MONEY), "paid_amount": nullable(MONEY), "outstanding_amount": nullable(MONEY),
    "state": nullable(enum("unsettled", "partial", "settled")), "visibility": enum("not_approved", "restricted", "complete")}
add("ClaimSettlement", claim_settlement, tuple(claim_settlement))
cash_total = {"currency": CURRENCY, **{key: report_decimal for key in ("outgoing_amount", "refund_amount", "correction_amount", "net_amount", "unallocated_amount")}, "entry_count": {"type": "integer", "minimum": 0}}
add("CashTotal", cash_total, tuple(cash_total))
cash_report = {**cost_report, "kind": {"const": "cash"}, "totals": array(ref("CashTotal"))}
add("CashReport", cash_report, tuple(cash_report))

subscription_price_input = {"effective_from": DATE, "effective_to": nullable(DATE), "currency": CURRENCY, "amount": MONEY, "plan": {"type": "string", "maxLength": 200}}
add("SubscriptionPriceInput", subscription_price_input, tuple(subscription_price_input))
subscription_fields = {"product": NAME, "supplier": nullable(ref("SpendSupplier")), "owner_id": nullable(ID), "category": {"type": "string", "maxLength": 50},
    "start_date": nullable(DATE), "trial_until": nullable(DATE), "cycle": nullable(enum("month", "year", "custom")), "timezone": {"type": "string", "maxLength": 200},
    "first_charge_date": nullable(DATE), "anchor_day": nullable({"type": "integer", "minimum": 1, "maximum": 31}), "custom_interval_days": nullable({"type": "integer", "minimum": 1, "maximum": 3660}),
    "renewal_mode": enum("", "automatic", "manual"), "payment_method_label": {"type": "string", "maxLength": 100},
    "reminder_days": array({"type": "integer", "minimum": 0, "maximum": 90}, maxItems=10, uniqueItems=True),
    "vendor_state": enum("unknown", "active", "paused", "cancelled"), "vendor_checked_at": nullable(TIME), "vendor_evidence_version_ids": array(ID, maxItems=20, uniqueItems=True),
    "prices": array(ref("SubscriptionPriceInput"), maxItems=1000), "notes": TEXT}
add("SubscriptionContent", subscription_fields, tuple(subscription_fields))
add("SubscriptionCreate", {**subscription_fields, "source_key": SCHEMAS["CostCreate"]["properties"]["source_key"], "project_id": nullable(ID), "classification": CLASS, "reason": NAME}, (*subscription_fields, "source_key", "project_id", "classification", "reason"))
add("SubscriptionPatch", {**subscription_fields, **cost_base}, (*subscription_fields, *cost_base))
add("SubscriptionPriceChange", {**cost_base, **subscription_price_input}, (*cost_base, *subscription_price_input))
subscription_transition = {**cost_base, "action": enum("activate", "pause", "resume", "cancel", "verify_vendor", "archive", "restore"), "effective_on": nullable(DATE),
    "vendor_state": enum("", "unknown", "active", "paused", "cancelled"), "vendor_checked_at": nullable(TIME), "vendor_evidence_version_ids": array(ID, maxItems=20, uniqueItems=True)}
add("SubscriptionTransition", subscription_transition, tuple(subscription_transition))
subscription_record = {**RECORD, **subscription_fields, "source_key": NAME, "project_id": nullable(ID), "creator_id": ID, "classification": CLASS,
    "state": enum("draft", "active", "paused", "cancelled", "archived"), "current_revision_id": ID, "content_hash": HASH, "cancel_effective_on": nullable(DATE), "paused_on": nullable(DATE),
    "next_charge_date": nullable(DATE), "signals": array(NAME), "supplier_evidence": nullable(cost_party_evidence), "attachments": array(ref("BusinessFileReference")),
    "owner_display_name": nullable(NAME), "owner_available": {"type": "boolean"}, "can_edit": {"type": "boolean"}, "can_manage": {"type": "boolean"}}
add("Subscription", subscription_record, tuple(subscription_record))
subscription_revision = {**subscription_fields, "id": ID, "seq": VERSION, "base_revision_id": nullable(ID), "content_hash": HASH, "author_id": ID, "reason": NAME, "created_at": TIME,
    "supplier_evidence": nullable(cost_party_evidence), "attachments": array(ref("BusinessFileReference")), "owner_display_name": nullable(NAME), "owner_available": {"type": "boolean"}}
add("SubscriptionRevision", subscription_revision, tuple(subscription_revision))
subscription_event = {"id": ID, "version": VERSION, "revision_id": ID, "kind": enum("draft_created", "config_revised", "price_changed", "activated", "paused", "resumed", "cancelled", "archived", "restored", "periods_generated", "period_changed"),
    "state": enum("draft", "active", "paused", "cancelled", "archived"), "period_id": nullable(ID), "effective_on": nullable(DATE), "actor_id": nullable(ID), "job_id": nullable(ID), "reason": NAME, "created_at": TIME}
add("SubscriptionEvent", subscription_event, tuple(subscription_event))
add("SubscriptionOwner", {"id": ID, "display_name": NAME}, ("id", "display_name"))
add("SubscriptionPrice", {**subscription_price_input, "id": ID, "revision_id": ID, "position": {"type": "integer", "minimum": 1, "maximum": 1000}}, (*subscription_price_input, "id", "revision_id", "position"))
add("SubscriptionGenerate", {**cost_base, "from": DATE, "to": DATE}, (*cost_base, "from", "to"))
add("SubscriptionUnexpectedPeriod", {**cost_base, "start_date": DATE}, (*cost_base, "start_date"))
subscription_generation = {"subscription_id": ID, "version": VERSION, "from": DATE, "to": DATE, "generated_ids": array(ID), "existing_count": {"type": "integer", "minimum": 0}, "missing_price_dates": array(DATE), "coverage_complete": {"type": "boolean"}}
add("SubscriptionGeneration", subscription_generation, tuple(subscription_generation))
period_fixed = {"configuration_revision_id": ID, "price_id": ID, "currency": CURRENCY, "amount": MONEY, "timezone": NAME, "category": NAME, "owner_id": ID, "supplier": ref("SpendSupplier")}
subscription_period = {**RECORD, **period_fixed, "subscription_id": ID, "product": NAME, "current_revision_id": ID, "content_hash": HASH, "start_date": DATE, "end_date": DATE, "charge_type": {"const": "recurring"},
    "state": enum("forecast", "awaiting_confirmation", "confirmed", "waived", "void"), "recorded_state": enum("forecast", "confirmed", "waived", "void"), "actual_amount": nullable(report_decimal), "actual_fingerprint": {"type": "string", "pattern": "^[0-9a-f]{32}$"}, "cost_ids": array(ID, maxItems=100),
    "anomalies": array(NAME), "review_reason": nullable(NAME), "review_current": {"type": "boolean"}, "can_manage": {"type": "boolean"}}
add("SubscriptionPeriod", subscription_period, tuple(subscription_period))
add("SubscriptionPeriodReforecast", {**cost_base, "configuration_revision_id": ID, "configuration_hash": HASH}, (*cost_base, "configuration_revision_id", "configuration_hash"))
add("SubscriptionPeriodLink", {**cost_base, "cost_id": ID, "cost_version": VERSION, "cost_revision_id": ID, "cost_content_hash": HASH}, (*cost_base, "cost_id", "cost_version", "cost_revision_id", "cost_content_hash"))
period_decision = {**cost_base, "expected_actual_fingerprint": nullable({"type": "string", "pattern": "^[0-9a-f]{32}$"}), "action": enum("confirm", "waive", "void", "reopen"), "acknowledge_difference": {"type": "boolean"}, "invoice_missing": {"type": "boolean"}, "evidence_version_ids": array(ID, maxItems=20, uniqueItems=True)}
add("SubscriptionPeriodDecision", period_decision, tuple(period_decision))
period_revision = {**period_fixed, "id": ID, "seq": VERSION, "base_revision_id": nullable(ID), "content_hash": HASH, "reason": NAME, "created_at": TIME}
add("SubscriptionPeriodRevision", period_revision, tuple(period_revision))
add("SubscriptionCostSnapshot", {"cost_id": ID, "revision_id": ID, "state": enum("confirmed", "reversed"), "content_hash": HASH, "currency": CURRENCY, "amount": MONEY}, ("cost_id", "revision_id", "state", "content_hash", "currency", "amount"))
period_event = {"id": ID, "version": VERSION, "revision_id": ID, "kind": enum("generated", "unexpected", "reforecast", "cost_linked", "confirmed", "waived", "void", "reopened"), "state": enum("forecast", "confirmed", "waived", "void"),
    "cost_link_id": nullable(ID), "review_fingerprint": nullable({"type": "string", "pattern": "^[0-9a-f]{32}$"}), "acknowledge_difference": {"type": "boolean"}, "invoice_missing": {"type": "boolean"},
    "actor_id": nullable(ID), "job_id": nullable(ID), "evidence_version_ids": array(ID, maxItems=20), "attachments": array(ref("BusinessFileReference")), "reason": NAME, "created_at": TIME}
period_event["cost_snapshot"] = array(ref("SubscriptionCostSnapshot"), maxItems=100)
add("SubscriptionPeriodEvent", period_event, tuple(period_event))
subscription_reminder = {"id": ID, "period_id": ID, "days_before": {"type": "integer", "minimum": 0, "maximum": 90}, "remind_on": DATE, "recipient_id": ID, "recipient_display_name": NAME, "period_start_date": DATE, "timezone": NAME, "delivery_state": enum("queued", "delivered", "suppressed", "retry_wait", "failed"), "created_at": TIME}
add("SubscriptionReminder", subscription_reminder, tuple(subscription_reminder))
forecast_total = {"currency": CURRENCY, "expected_amount": report_decimal, **{key: {"type": "integer", "minimum": 0} for key in ("period_count", "awaiting_count", "reviewed_count", "anomaly_count")}}
add("ForecastTotal", forecast_total, tuple(forecast_total))
forecast_report = {"kind": {"const": "forecast"}, "from": DATE, "to": DATE, "calendar_basis": {"const": "subscription_local_date"}, "timezones": array(NAME), "as_of": TIME, "base_currency": {"type": "null"}, "fx_coverage": {"const": "unconfigured"},
    "totals": array(ref("ForecastTotal")), "coverage_complete": {"type": "boolean"}, "coverage_limit_reached": {"type": "boolean"}, **{key: {"type": "integer", "minimum": 0} for key in ("missing_period_count", "missing_price_count", "draft_count", "incomplete_subscription_count")}}
add("ForecastReport", forecast_report, tuple(forecast_report))

add("ConversationCreate", {"type": enum("private", "project"), "project_id": nullable(ID), "title": NAME,
    "member_ids": array(ID, maxItems=99, uniqueItems=True)}, ("type", "title", "member_ids"))
SCHEMAS["ConversationCreate"]["allOf"] = [{"if": {"properties": {"type": {"const": "project"}}},
    "then": {"required": ["project_id"], "properties": {"project_id": ID}}},
    {"if": {"properties": {"type": {"const": "private"}}}, "then": {"properties": {"member_ids": {"maxItems": 0}}}}]
add("ConversationMember", {"user_id": ID, "display_name": NAME, "available": {"type": "boolean"}, "is_creator": {"type": "boolean"}},
    ("user_id", "display_name", "available", "is_creator"))
add("Conversation", {**RECORD, "type": enum("private", "project", "telegram"), "project_id": nullable(ID), "title": NAME,
    "state": enum("active", "archived"), "classification": CLASS, "creator_id": ID, "current_revision_id": ID,
    "context_security_version": VERSION, "members": array(ref("ConversationMember"), minItems=1, maxItems=100),
    "can_edit": {"type": "boolean"}, "can_manage_members": {"type": "boolean"}},
    ("id", "version", "type", "project_id", "title", "state", "classification", "creator_id", "current_revision_id", "context_security_version", "members", "can_edit", "can_manage_members", "created_at", "updated_at"))
add("ConversationPatch", {**EXPECTED, "title": NAME, "state": enum("active", "archived"), "reason": NAME},
    ("expected_version", "title", "state", "reason"))
add("ConversationMemberChange", {**EXPECTED, "user_id": ID, "action": enum("add", "remove"), "reason": NAME},
    ("expected_version", "user_id", "action", "reason"))
add("ConversationAccessResult", {"conversation_id": ID, "version": VERSION, "context_security_version": VERSION, "access_lost": {"type": "boolean"}},
    ("conversation_id", "version", "context_security_version", "access_lost"))
add("ConversationCandidates", {"items": array(ref("ConversationMember"), maxItems=100)}, ("items",))
context_view = {"conversation_id": ID, "conversation_revision_id": ID, "context_security_version": VERSION,
    "type": enum("project", "task", "post", "file", "service", "agent_finance_report"), "id": ID, "version_id": nullable(ID), "resource_version": VERSION,
    "title": NAME, "state": NAME, "text": {"type": "string", "maxLength": 10000},
    "offset": {"type": "integer", "minimum": 0, "maximum": 2097152}, "total_chars": {"type": "integer", "minimum": 0},
    "next_offset": nullable({"type": "integer", "minimum": 0}), "source_sha256": HASH, "context_sha256": HASH,
    "evidence": array(ref("BusinessFileReference"), maxItems=20), "untrusted_data": {"const": True}, "observed_at": TIME}
add("ConversationContextView", context_view, tuple(context_view))
index_evidence = {"extractor": NAME, "state": enum("pending", "ready", "unsupported", "blocked", "failed"),
    "source_sha256": HASH, "text_sha256": nullable(HASH), "indexed_at": nullable(TIME),
    "total_characters": nullable({"type": "integer", "minimum": 0, "maximum": 2097152}), "error_code": nullable(NAME)}
add("FileTextIndexEvidence", index_evidence, tuple(index_evidence))
context_view["text_index"] = ref("FileTextIndexEvidence")
profile = {"id": ID, "skill_id": NAME, "skill_version": NAME, "skill_sha256": HASH,
    "runtime_version": NAME, "configuration_sha256": HASH, "max_input_tokens": VERSION,
    "max_output_tokens": VERSION, "max_tool_calls": VERSION, "timeout_seconds": VERSION}
notification_categories = enum("mention", "task", "approval", "request_result", "contract_due", "subscription_due", "replica_failure", "incident", "secretary")
notification_target = {"type": enum("task", "request", "contract", "subscription_period", "file", "incident", "agent_run"),
    "id": ID, "container_id": nullable(ID), "web_path": {"type": "string", "minLength": 1, "maxLength": 300}}
add("NotificationTarget", {**notification_target, "comment_id": ID}, tuple(notification_target))
notification_fields = {"id": ID, "category": notification_categories, "title": NAME, "resource_label": NAME,
    "current_state": NAME, "attention_required": {"type": "boolean"}, "read": {"type": "boolean"},
    "delivery_seq": VERSION, "read_seq": {"type": "integer", "minimum": 0, "maximum": 9007199254740991},
    "business_event_id": ID, "template_version": {"const": 1}, "target": ref("NotificationTarget"),
    "created_at": TIME, "updated_at": TIME, "read_at": nullable(TIME)}
add("Notification", notification_fields, tuple(notification_fields))
notification_summary = {"unread_count": {"type": "integer", "minimum": 0, "maximum": 100},
    "attention_count": {"type": "integer", "minimum": 0, "maximum": 100}, "count_complete": {"type": "boolean"}, "as_of": TIME}
add("NotificationSummary", notification_summary, tuple(notification_summary))
add("NotificationRead", {"delivery_seq": VERSION}, ("delivery_seq",))
notification_failure = {"job_id": ID, "state": enum("retry_wait", "dead"), "safe_code": nullable(NAME),
    "attempt": {"type": "integer", "minimum": 0, "maximum": 16}, "max_attempts": {"type": "integer", "minimum": 1, "maximum": 16},
    "first_failed_at": nullable(TIME), "updated_at": TIME}
add("NotificationFailure", notification_failure, tuple(notification_failure))
notification_retry = {"id": ID, "source_job_id": ID, "retry_job_id": ID, "business_event_id": ID, "created_by": ID, "created_at": TIME}
add("NotificationRetryReceipt", notification_retry, tuple(notification_retry))
add("NotificationRetryInput", {"expected_revision": {"type":"string","pattern":"^[0-9a-f]{64}$"}, "reason": NAME}, ("expected_revision","reason"))
notification_outcome = {"delivered": {"type":"integer","minimum":0,"maximum":100}, "suppressed": {"type":"integer","minimum":0,"maximum":100},
    "outcome": enum("delivered","suppressed"), "safe_code": NAME, "completed_at": TIME}
add("NotificationDeliveryOutcome", notification_outcome, tuple(notification_outcome))
notification_job = {"job_id": ID, "kind":enum("notification.deliver.v1","event.rejected.v1"),
    "state":enum("queued","running","succeeded","retry_wait","dead","cancelled"), "safe_code":nullable(NAME),
    "attempt": {"type":"integer","minimum":0,"maximum":16}, "max_attempts":{"type":"integer","minimum":1,"maximum":16},
    "first_failed_at":nullable(TIME), "updated_at":TIME,"available_at":TIME,"revision":{"type":"string","pattern":"^[0-9a-f]{64}$"},
    "business_event_id":nullable(ID),"event_type":nullable(NAME), "can_retry":{"type":"boolean"},
    "retry_block":enum("ready","not_authorized","not_failed","already_retried","invalid_input","rejected_event","missing_event","unsupported_event"),
    "retry_id":nullable(ID),"retry_job_id":nullable(ID),"parent_job_id":nullable(ID),"result":nullable(ref("NotificationDeliveryOutcome"))}
add("NotificationDeliveryJob", notification_job, tuple(notification_job))

notification_policy = {"id":ID,"version":VERSION,"enabled":{"type":"boolean"},"timezone":NAME,
    "days_before":{"type":"array","items":{"type":"integer","minimum":0,"maximum":365},"maxItems":12,"uniqueItems":True},
    "effective_on":DATE,"created_by":ID,"reason":NAME,"created_at":TIME}
add("NotificationContractPolicy",notification_policy,tuple(notification_policy))
add("NotificationContractPolicyState",{"current":nullable(ref("NotificationContractPolicy")),"can_publish":{"type":"boolean"}},("current","can_publish"))
notification_policy_input={"expected_version":{"type":"integer","minimum":0,"maximum":9007199254740990},
    "enabled":{"type":"boolean"},"timezone":NAME,"days_before":notification_policy["days_before"],"reason":NAME}
add("NotificationContractPolicyInput",notification_policy_input,tuple(notification_policy_input))

budget_tokens = {"type": "integer", "minimum": 0, "maximum": 1000000000000}
budget_policy_input = {"expected_version": {"type": "integer", "minimum": 0},
    "organization_tokens": budget_tokens, "user_tokens": budget_tokens, "project_tokens": budget_tokens, "skill_tokens": budget_tokens, "reason": NAME}
add("BudgetPolicyInput", budget_policy_input, tuple(budget_policy_input))
budget_policy = {k:v for k,v in budget_policy_input.items() if k != "expected_version"}
budget_policy.update({"id": ID, "version": VERSION, "published_by": ID, "created_at": TIME})
add("BudgetPolicy", budget_policy, tuple(budget_policy))
budget_summary = {"policy": nullable(ref("BudgetPolicy")), "month": {"type":"string","pattern":r"^\d{4}-\d{2}$"},
    "scope": enum("user","organization","project","skill"), "scope_id": nullable(NAME),
    "limit_tokens": nullable(budget_tokens), "reserved_tokens": {"type":"integer","minimum":0}, "charged_tokens": {"type":"integer","minimum":0},
    "pending_tokens": {"type":"integer","minimum":0}, "available_tokens": nullable({"type":"integer"}), "unbudgeted_runs": {"type":"integer","minimum":0}}
add("BudgetSummary", budget_summary, tuple(budget_summary))
budget_entry = {"id": ID, "policy_id": ID, "actor_id": ID, "project_id": nullable(ID), "skill_id": NAME, "run_version":VERSION,
    "run_state":enum("queued","running","awaiting_approval","committing","succeeded","failed","cancelled","interrupted"),
    "status":enum("reserved","settled","pending","released","reconciled"),
    "estimated_tokens":{"type":"integer","minimum":1}, "reserved_tokens":{"type":"integer","minimum":0}, "charged_tokens":{"type":"integer","minimum":0},
    "pending_tokens":{"type":"integer","minimum":0}, "input_tokens":nullable(budget_tokens), "output_tokens":nullable(budget_tokens),
    "evidence_reference":nullable(NAME), "reconciliation_reason":nullable(NAME), "reconciled_by":nullable(ID), "reconciled_at":nullable(TIME), "created_at":TIME}
add("BudgetEntry", budget_entry, tuple(budget_entry))
budget_reconcile = {"expected_version":VERSION,"input_tokens":budget_tokens,"output_tokens":budget_tokens,"evidence_reference":NAME,"reason":NAME}
add("BudgetReconcileInput", budget_reconcile, tuple(budget_reconcile))
add("RuntimeProfile", profile, tuple(profile))
add("RuntimeOptions", {"available": {"type": "boolean"}, "reason": nullable(NAME),
    "profiles": array(ref("RuntimeProfile"), maxItems=8)}, ("available", "reason", "profiles"))
source_snapshot = {"type": enum("project", "task", "post", "milestone", "deliverable", "risk", "issue", "file", "service", "agent_plan", "agent_mutation", "agent_file_update", "agent_file_create", "agent_organization_plan", "agent_directory", "agent_oa_request", "agent_contract_body", "agent_post_write", "agent_finance_report"), "id": ID, "version_id": nullable(ID),
    "resource_version": VERSION, "sha256": HASH}
add("AgentSourceSnapshot", source_snapshot, ("type", "id", "resource_version", "sha256"))
mutation_arguments = {"id": ID, **EXPECTED, "reason": NAME, "name": NAME, "scope": TEXT, "type": NAME,
    "start_date": nullable(DATE), "due_date": nullable(DATE), "title": NAME, "body": TEXT,
    "priority": enum("low", "normal", "high", "urgent"), "assignee_id": nullable(ID), "parent_id": nullable(ID), "milestone_id": nullable(ID),
    "start_at": nullable(TIME), "due_at": nullable(TIME), "owner_id": ID, "acceptance_criteria": {"type": "string", "minLength": 1, "maxLength": 20000}, "action": enum("add", "remove", "start", "review", "return", "complete", "block", "resume", "cancel", "reopen", "activate", "hold", "close", "achieve", "miss")}
mutation_arguments.update({"predecessor_id":ID,"expected_predecessor_version":VERSION,"review_required":{"type":"boolean"},"description":TEXT,"severity":enum("low","medium","high","critical"),
    "probability":nullable(enum("low","medium","high")),"mitigation":TEXT,"resolution":TEXT,
    "state":enum("open","mitigating","realized","investigating","resolved","closed")})
add("AgentMutationArguments", mutation_arguments, ("id", "expected_version", "reason"))
add("AgentMutationInput", {"tool": enum("update_task_dependency", "create_task", "create_risk", "update_risk", "create_issue", "update_issue", "create_deliverable", "update_deliverable", "create_milestone", "update_project", "update_task", "transition_task", "transition_project", "update_milestone", "transition_milestone"), "arguments": ref("AgentMutationArguments")}, ("tool", "arguments"))
mutation_view = {"run_id": ID, "tool": enum("update_task_dependency", "create_task", "create_risk", "update_risk", "create_issue", "update_issue", "create_deliverable", "update_deliverable", "create_milestone", "update_project", "update_task", "transition_task", "transition_project", "update_milestone", "transition_milestone"), "resource_type": enum("project", "task", "milestone", "deliverable", "risk", "issue"),
    "resource_id": ID, "base_version": VERSION, "intent_sha256": HASH, "arguments": ref("AgentMutationArguments"),
    "before": {"type": "object", "additionalProperties": True}, "status": enum("authorized", "committed"),
    "operation_id": nullable(ID), "result_version": nullable(VERSION), "result": nullable({"type": "object", "additionalProperties": True})}
add("AgentMutationView", mutation_view, tuple(mutation_view))
file_recovery_input = {"source_run_id": ID, "draft_id": ID, "content_sha256": HASH, "reviewed": {"type":"boolean", "const":True}}
add("AgentFileRecoveryInput", file_recovery_input, tuple(file_recovery_input))
file_recovery_view = {"source_run_id": ID, "draft_id": ID, "content_sha256": HASH, "recovery_sha256": HASH, "depth": {"type":"integer", "minimum":1, "maximum":8}}
add("AgentFileRecoveryView", file_recovery_view, tuple(file_recovery_view))
add("AgentFileUpdateInput", {"file_id": ID, "base_version_id": ID, "expected_file_version": VERSION}, ("file_id", "base_version_id", "expected_file_version"))
file_target = {"name": NAME, "folder_id": ID, "classification": CLASS, "media_type": enum("text/plain", "text/markdown")}
add("AgentFileUpdateTarget", {**file_target, "folder_version": VERSION}, tuple(file_target))
file_create_input = {"folder_id": ID, "expected_folder_version": VERSION, "name": NAME, "media_type": enum("text/plain", "text/markdown")}
add("AgentFileCreateInput", file_create_input, tuple(file_create_input))
file_diff = {"start_char": {"type": "integer", "minimum": 0}, "removed_chars": {"type": "integer", "minimum": 0}, "added_chars": {"type": "integer", "minimum": 0},
    "removed_sha256": HASH, "added_sha256": HASH, "before": {"type": "string", "maxLength": 1000}, "after": {"type": "string", "maxLength": 1000},
    "before_complete": {"type": "boolean"}, "after_complete": {"type": "boolean"}, "line_ending": enum("none", "lf", "crlf"), "utf8_bom": {"type": "boolean"}}
add("AgentFileDiff", file_diff, tuple(file_diff))
file_draft = {"id": ID, "content_sha256": HASH, "size_bytes": BYTES, "diff": ref("AgentFileDiff"), "state": NAME, "scan_state": NAME, "plan_hash": nullable(HASH), "result_version_id": nullable(ID)}
add("AgentFileDraft", file_draft, tuple(file_draft))
file_update = {"run_id": ID, "file_id": ID, "base_version_id": ID, "file_version": VERSION, "intent_sha256": HASH, "target": ref("AgentFileUpdateTarget"),
    "status": enum("authorized", "committed"), "drafts": array(ref("AgentFileDraft"), maxItems=3), "operation_id": nullable(ID), "result": nullable(ref("FileCommitResult"))}
add("AgentFileUpdateView", {**file_update, "recovery":ref("AgentFileRecoveryView")}, tuple(file_update))
file_create_view = {k:v for k,v in file_update.items() if k not in ("file_id", "base_version_id", "file_version")}
add("AgentFileCreateView", {**file_create_view, "recovery":ref("AgentFileRecoveryView")}, tuple(file_create_view))
oa_draft = {"kind": enum("contract", "purchase", "claim"), "business_id": ID, "expected_business_version": VERSION,
    "business_revision_id": ID, "business_content_hash": HASH, "reason": {"type":"string","minLength":1,"maxLength":200}, "request_id": ID, "expected_version": VERSION}
add("AgentOADraftArguments", oa_draft, tuple(k for k in oa_draft if k not in ("request_id","expected_version")))
SCHEMAS["AgentOADraftArguments"]["dependentRequired"] = {"request_id":["expected_version"],"expected_version":["request_id"]}
oa_submit = {"request_id": ID, **EXPECTED, "revision_id": ID, "content_hash": HASH, "workflow_definition_id": ID, "preview_hash": HASH, "reason": {"type":"string","minLength":1,"maxLength":200}}
add("AgentOASubmitArguments", oa_submit, tuple(oa_submit))
oa_text={k:v for k,v in oa_draft.items() if k not in ("request_id","expected_version")}
oa_text["fields"]={"type":"array","minItems":1,"maxItems":3,"uniqueItems":True,"items":enum("renewal_terms","termination_terms","purpose","notes")}
add("AgentOATextArguments",oa_text,tuple(oa_text))
add("AgentOATextAdopt",{"draft_sha256":HASH,"reviewed":{"const":True},"reason":{"type":"string","minLength":1,"maxLength":200}},("draft_sha256","reviewed","reason"))
SCHEMAS["AgentOARequestInput"] = {"oneOf":[obj({"tool":{"const":"draft_request"},"arguments":ref("AgentOADraftArguments")},("tool","arguments")), obj({"tool":{"const":"submit_request"},"arguments":ref("AgentOASubmitArguments")},("tool","arguments")), obj({"tool":{"const":"draft_business_text"},"arguments":ref("AgentOATextArguments")},("tool","arguments"))]}
oa_business = {"id":ID,"kind":enum("contract","purchase","claim"),"version":VERSION,"creator_id":ID,"title":NAME,"project_id":nullable(ID),"classification":CLASS,"lifecycle":NAME,"approval_state":NAME,"current_revision_id":ID,"content_hash":HASH,"request_id":nullable(ID),"payload":ref("RequestPayload")}
add("AgentOABusinessView",oa_business,tuple(oa_business))
add("AgentOARequestBasis",{"business":ref("AgentOABusinessView"),"request":nullable(ref("Request")),"preview":nullable(ref("SubmissionPreview"))},("business","request","preview"))
add("AgentOATextDraftView",{"source_refs":array(ref("AgentSourceSnapshot"),maxItems=100),"fields":{"type":"object","minProperties":1,"maxProperties":3,"additionalProperties":{"type":"string","maxLength":20000}},"draft_sha256":HASH,"operation_id":nullable(ID),"result":nullable(ref("AgentOABusinessView"))},("fields","draft_sha256","source_refs","operation_id","result"))
oa_view = {"run_id":ID,"tool":enum("draft_request","submit_request","draft_business_text"),"kind":enum("contract","purchase","claim"),"business_id":ID,"request_id":nullable(ID),"arguments":{"oneOf":[ref("AgentOADraftArguments"),ref("AgentOASubmitArguments"),ref("AgentOATextArguments")]},"before":ref("AgentOARequestBasis"),"intent_sha256":HASH,"status":enum("authorized","committed"),"operation_id":nullable(ID),"result":nullable(ref("Request"))}
oa_view["status"]=enum("authorized","committed","drafted","adopted")
add("AgentOARequestView",oa_view,tuple(oa_view))
SCHEMAS["AgentOARequestView"]["properties"]["text_draft"]=ref("AgentOATextDraftView")
post_base={"post_id":ID,"expected_version":VERSION,"base_revision_id":ID,"content_hash":HASH}
post_draft={"tool":{"const":"draft_post"},**post_base,"type":enum("project_log","meeting_note","sop","article"),"title":NAME,"classification":enum("internal","project_restricted"),"source_refs":array(ref("ResourceRef"),maxItems=20),"reason":NAME}
add("AgentPostDraftInput",post_draft,("tool","type","title","classification","reason"))
SCHEMAS["AgentPostDraftInput"]["dependentRequired"]={key:list(post_base) for key in post_base}
add("AgentPostPublishInput",{"tool":{"const":"publish_post"},**post_base,"reason":NAME},("tool",*post_base,"reason"))
SCHEMAS["AgentPostWriteInput"]={"oneOf":[ref("AgentPostDraftInput"),ref("AgentPostPublishInput")]}
add("AgentPostWriteBasis",{"scope":obj({"project_id":nullable(ID),"version":{"type":"integer","minimum":0},"state":NAME,"classification":CLASS},("project_id","version","state","classification")),"post":nullable(ref("Post")),"attachments":array(ref("BusinessFileReference"),maxItems=40)},("scope","post","attachments"))
post_write={"run_id":ID,"input":ref("AgentPostWriteInput"),"before":ref("AgentPostWriteBasis"),"intent_sha256":HASH,"status":enum("authorized","committed"),"operation_id":nullable(ID),"result":nullable(ref("Post"))}
add("AgentPostWriteView",post_write,tuple(post_write))
contract_body_input={"contract_id":ID,"expected_version":VERSION,"base_revision_id":ID,"content_hash":HASH,"reason":NAME}
add("AgentContractBodyInput",contract_body_input,tuple(contract_body_input))
contract_body_file={"draft_id":ID,"evidence":ref("BusinessFileReference"),"content":{"type":"string","minLength":1,"maxLength":49152}}
add("AgentContractBodyFile",contract_body_file,tuple(contract_body_file))
contract_body_view={"run_id":ID,"input":ref("AgentContractBodyInput"),"before":ref("AgentOABusinessView"),"intent_sha256":HASH,"file_intent_sha256":HASH,"status":enum("authorized","file_committed","adopted"),"file":nullable(ref("AgentContractBodyFile")),"operation_id":nullable(ID),"result":nullable(ref("AgentOABusinessView"))}
add("AgentContractBodyView",contract_body_view,tuple(contract_body_view))
contract_body_adopt={"intent_sha256":HASH,"version_id":ID,"content_sha256":HASH,"reviewed":{"const":True},"reason":NAME}
add("AgentContractBodyAdopt",contract_body_adopt,tuple(contract_body_adopt))
add("RunCreate", {**EXPECTED, "context_security_version": VERSION, "profile_id": ID,
    "client_message_id": ID, "message": {"type": "string", "minLength": 1, "maxLength": 20000},
    "skill_id": NAME, "context_refs": array(ref("ResourceRef"), maxItems=20), "mutation": ref("AgentMutationInput"), "file_update": ref("AgentFileUpdateInput"), "file_create": ref("AgentFileCreateInput"), "file_recovery": ref("AgentFileRecoveryInput"), "oa_request": ref("AgentOARequestInput"), "post_write":ref("AgentPostWriteInput")},
    ("expected_version", "context_security_version", "profile_id", "client_message_id", "message"))
SCHEMAS["RunCreate"]["properties"]["contract_body"]=ref("AgentContractBodyInput")
add("Run", {**RECORD, "conversation_id": ID,
    "state": enum("queued", "running", "awaiting_approval", "committing", "succeeded", "failed", "cancelled", "interrupted"),
    "last_event_seq": {"type": "integer", "minimum": 0}, "result_refs": array(ref("ResourceRef")),
    "safe_error_code": nullable(NAME), "actor_id": ID, "profile_id": ID, "client_message_id": ID, "can_cancel": {"type": "boolean"},
    "cancel_requested_at": nullable(TIME), "expires_at": TIME, "mutation": ref("AgentMutationView"), "file_update": ref("AgentFileUpdateView"), "file_create": ref("AgentFileCreateView"), "oa_request": ref("AgentOARequestView"), "post_write":ref("AgentPostWriteView"), "outcome": nullable(enum("draft", "pending", "awaiting_input", "completed", "conflict", "denied", "failed"))},
    ("id", "version", "conversation_id", "state", "last_event_seq", "actor_id", "profile_id", "client_message_id", "can_cancel", "cancel_requested_at", "expires_at"))
SCHEMAS["Run"]["properties"]["contract_body"]=ref("AgentContractBodyView")
add("Message", {"id": ID, "run_id": ID, "seq": VERSION, "role": enum("user", "assistant", "system"), "body": TEXT,
    "source_refs": array(ref("AgentSourceSnapshot"), maxItems=20), "visibility": enum("available", "unavailable"),
    "created_at": TIME}, ("id", "run_id", "seq", "role", "body", "source_refs", "visibility", "created_at"))
run_event = {"id": ID, "run_id": ID, "seq": VERSION,
    "type": enum("run.queued", "run.started", "run.cancel_requested", "run.cancelled", "run.interrupted", "run.failed", "run.completed", "run.awaiting_approval", "run.committing"),
    "state": SCHEMAS["Run"]["properties"]["state"], "safe_code": nullable(NAME), "created_at": TIME}
add("RunEvent", run_event, tuple(run_event))
add("RunEventPage", {"items": array(ref("RunEvent"), maxItems=100), "after_seq": {"type": "integer", "minimum": 0},
    "has_more": {"type": "boolean"}, "run": ref("Run")}, ("items", "after_seq", "has_more", "run"))
plan_project = {**project_fields, "classification": enum("internal", "project_restricted"), "member_ids": array(ID, maxItems=30, uniqueItems=True)}
add("AgentPlanProject", plan_project, ("name", "scope", "owner_id", "classification"))
add("AgentProjectPlanCreate", {**EXPECTED, "context_security_version": VERSION, "project": ref("AgentPlanProject"), "reason": NAME}, ("expected_version", "context_security_version", "project", "reason"))
add("AgentPlanAction", {**EXPECTED, "plan_hash": HASH, "reason": NAME}, ("expected_version", "plan_hash", "reason"))
template_snapshot = {"type": "object", "properties": {"id": ID, "published_revision": VERSION, "schema_sha256": HASH}, "additionalProperties": False}
plan_fields = {"id": ID, "version": VERSION, "conversation_id": ID, "conversation_revision_id": ID, "context_security_version": VERSION,
    "actor_id": ID, "actor_auth_version": VERSION, "organization_auth_version": VERSION, "origin_run_id": nullable(ID), "kind": {"const": "project.create"},
    "plan_hash": HASH, "project": ref("AgentPlanProject"), "template_snapshot": template_snapshot, "reason": NAME,
    "state": enum("proposed", "approved", "revoked", "committed"), "operation_id": nullable(ID), "project_id": nullable(ID),
    "created_at": TIME, "expires_at": TIME, "blocked_reason": nullable(NAME), "can_approve": {"type": "boolean"}, "can_commit": {"type": "boolean"}, "can_revoke": {"type": "boolean"}}
add("AgentPlan", plan_fields, tuple(plan_fields))
add("AgentPlanList", {"items": array(ref("AgentPlan"), maxItems=50), "has_more": {"type": "boolean"}}, ("items", "has_more"))
add("AgentOrganizationAction", {"kind": enum("folder.create", "folder.move", "file.move"), "id": ID,
    "expected_version": VERSION, "name": NAME, "target_folder_id": ID, "expected_target_version": VERSION}, ("kind", "id", "expected_version"))
organization_entry = {"kind": enum("folder", "file"), "id": ID, "version": VERSION, "name": NAME,
    "parent_id": nullable(ID), "project_id": nullable(ID), "classification": CLASS}
add("AgentOrganizationEntry", organization_entry, tuple(organization_entry))
add("AgentOrganizationPreview", {"action": ref("AgentOrganizationAction"), "source": ref("AgentOrganizationEntry"),
    "path": array(ref("AgentOrganizationEntry"), maxItems=32), "target": nullable(ref("AgentOrganizationEntry")),
    "target_path": array(ref("AgentOrganizationEntry"), maxItems=32)}, ("action", "source", "path", "target", "target_path"))
add("AgentDirectory", {"folder": ref("AgentOrganizationEntry"), "items": array(ref("AgentOrganizationEntry"), maxItems=50), "has_more": {"type": "boolean"}}, ("folder", "items", "has_more"))
add("AgentOrganizationPlanCreate", {**EXPECTED, "context_security_version": VERSION,
    "actions": array(ref("AgentOrganizationAction"), minItems=1, maxItems=10), "reason": NAME}, ("expected_version", "context_security_version", "actions", "reason"))
add("AgentOrganizationOutcome", {"operation_id": ID, "data": {"oneOf": [ref("Folder"), ref("File")]}}, ("operation_id", "data"))
organization_plan_fields = {key: value for key, value in plan_fields.items() if key not in ("kind", "project", "template_snapshot")}
organization_plan_fields.update({"preview": array(ref("AgentOrganizationPreview"), minItems=1, maxItems=10), "results": array(ref("AgentOrganizationOutcome"), maxItems=10)})
add("AgentOrganizationPlan", organization_plan_fields, tuple(organization_plan_fields))
add("AgentOrganizationPlanList", {"items": array(ref("AgentOrganizationPlan"), maxItems=50), "has_more": {"type": "boolean"}}, ("items", "has_more"))
add("Operation", {"id": ID, "state": enum("in_progress", "succeeded", "failed"),
    "result_ref": nullable(ref("ResourceRef")), "committed": {"type": "boolean"}}, ("id", "state", "committed"))
STATUS = enum("operational", "degraded", "down", "maintenance", "unknown")
INCIDENT_STATE = enum("open", "investigating", "monitoring", "resolved")
STATUS_TEXT = {"type": "string", "maxLength": 10000}
status_content = {"name": NAME, "owner_id": ID, "host_label": {"type": "string", "maxLength": 200},
    "endpoint_label": {"type": "string", "maxLength": 500}, "description": STATUS_TEXT}
add("ServiceCreate", {**status_content, "project_id": nullable(ID), "classification": CLASS, "reason": NAME},
    (*status_content, "project_id", "classification", "reason"))
add("ServicePatch", {**status_content, **EXPECTED, "lifecycle": enum("active", "archived"), "reason": NAME},
    (*status_content, "expected_version", "lifecycle", "reason"))
thresholds = {"failure_count": {"type": "integer", "minimum": 1, "maximum": 10},
    "recovery_count": {"type": "integer", "minimum": 1, "maximum": 10},
    "interval_seconds": {"type": "integer", "minimum": 15, "maximum": 60},
    "stale_after_seconds": {"type": "integer", "minimum": 45, "maximum": 180},
    "latency_limit_ms": {"type": "integer", "minimum": 1, "maximum": 120000}}
public_key = {"type": "string", "pattern": "^[A-Za-z0-9+/]{43}=$"}
source_content = {"name": NAME, "provider": enum("uptime_kuma", "prometheus", "adapter"),
    "external_ref": NAME, "public_key": public_key, "enabled": {"type": "boolean"}, **thresholds}
add("ServiceSourceInput", {**source_content, **EXPECTED,
    "expected_source_version": {"type": "integer", "minimum": 0, "maximum": 9007199254740991},
    "base_revision_id": nullable(ID), "reason": NAME},
    (*source_content, "expected_version", "expected_source_version", "base_revision_id", "reason"))
projection = {"sequence": VERSION, "observed_at": TIME, "received_at": TIME,
    "state": enum("operational", "degraded", "down", "unknown"),
    "failure_streak": {"type": "integer", "minimum": 0, "maximum": 10},
    "success_streak": {"type": "integer", "minimum": 0, "maximum": 10}}
add("ServiceProjection", projection, tuple(projection))
source_view = {**source_content, "id": ID, "service_id": ID, "revision_id": ID, "version": VERSION,
    "published_by": ID, "reason": NAME, "created_at": TIME, "state": STATUS,
    "state_reason": {"type": "string", "maxLength": 200}, "projection": nullable(ref("ServiceProjection"))}
add("ServiceSource", source_view, tuple(source_view))
source_revision = {**source_content, "id": ID, "source_id": ID, "seq": VERSION, "base_revision_id": nullable(ID),
    "published_by": ID, "reason": NAME, "created_at": TIME, "is_current": {"type": "boolean"}}
add("ServiceSourceRevision", source_revision, tuple(source_revision))
service_view = {**RECORD, **status_content, "creator_id": ID, "project_id": nullable(ID), "classification": CLASS,
    "lifecycle": enum("active", "archived"), "state": STATUS, "observed_state": STATUS,
    "unknown_sources": {"type": "integer", "minimum": 0}, "maintenance_id": nullable(ID), "evaluated_at": TIME,
    "recheck_at": {**TIME, "description": "最迟重新读取时刻；不构成在此之前健康或授权不会变化的保证。"},
    "sources": array(ref("ServiceSource")), "can_manage": {"type": "boolean"}, "can_update_incident": {"type": "boolean"}}
add("Service", service_view, tuple(service_view))
signed_observation = {"source_revision_id": ID, "sequence": {"type": "string", "pattern": "^[1-9][0-9]{0,15}$"},
    "observed_at": TIME, "healthy": {"type": "boolean"}, "latency_ms": nullable({"type": "integer", "minimum": 0, "maximum": 3600000})}
add("SignedObservation", signed_observation, tuple(signed_observation))
observation = {"id": ID, "source_id": ID, **signed_observation, "received_at": TIME, "payload_sha256": HASH,
    "applied": {"type": "boolean"}, "ignored_reason": enum("", "LATE_SEQUENCE", "LATE_OBSERVATION", "STALE_SAMPLE")}
add("ServiceObservation", observation, tuple(observation))
proof = {**observation, "signed_payload": {"type": "string", "maxLength": 5464},
    "signature": {"type": "string", "minLength": 88, "maxLength": 88}, "public_key": public_key}
add("ServiceObservationProof", proof, tuple(proof))
ack = {"id": ID, "source_id": ID, "source_revision_id": ID, "sequence": observation["sequence"], "payload_sha256": HASH, "applied": {"type": "boolean"}, "ignored_reason": observation["ignored_reason"], "replayed": {"type": "boolean"}}
add("ObservationAck", ack, tuple(ack))
add("MaintenanceInput", {**EXPECTED, "start_at": TIME, "end_at": TIME, "reason": NAME},
    ("expected_version", "start_at", "end_at", "reason"))
add("MaintenanceCancel", {**EXPECTED, "reason": NAME}, ("expected_version", "reason"))
maintenance = {"id": ID, "service_id": ID, "start_at": TIME, "end_at": TIME, "reason": NAME,
    "created_by": ID, "created_at": TIME, "cancelled_by": nullable(ID), "cancelled_at": nullable(TIME), "cancellation_reason": nullable(NAME)}
add("MaintenanceWindow", maintenance, tuple(maintenance))
incident_create = {"title": NAME, "service_ids": array(ID, minItems=1, maxItems=20, uniqueItems=True),
    "owner_id": ID, "dedup_key": NAME, "severity": enum("low", "medium", "high", "critical"),
    "description": STATUS_TEXT, "timeline_note": {"type": "string", "minLength": 1, "maxLength": 10000}, "reason": NAME}
add("IncidentCreate", incident_create, tuple(incident_create))
incident_patch = {**EXPECTED, "state": INCIDENT_STATE, "timeline_note": incident_create["timeline_note"],
    "resolution": STATUS_TEXT, "postmortem": STATUS_TEXT, "reason": NAME}
add("IncidentPatch", incident_patch, tuple(incident_patch))
incident = {**RECORD, **{k: v for k, v in incident_create.items() if k not in ("timeline_note", "reason")},
    "creator_id": ID, "state": INCIDENT_STATE, "resolution": STATUS_TEXT, "postmortem": STATUS_TEXT,
    "resolved_at": nullable(TIME), "can_update": {"type": "boolean"}}
add("Incident", incident, tuple(incident))
entry = {"id": ID, "incident_id": ID, "seq": VERSION, "state": INCIDENT_STATE, "note": incident_create["timeline_note"],
    "resolution": STATUS_TEXT, "postmortem": STATUS_TEXT, "actor_id": ID, "created_at": TIME}
add("IncidentEntry", entry, tuple(entry))

add("BusinessPartyReference", {"id": ID, "revision_id": ID, "name": NAME, "kind": enum("organization", "person")}, ("id", "revision_id", "name", "kind"))
add("BusinessFileReference", {"file_id": ID, "version_id": ID, "seq": VERSION, "name": NAME, "sha256": HASH, "size_bytes": BYTES,
    "media_type": NAME, "scan_state": enum("clean", "waived"), "scan_waiver_ref": nullable(ID)},
    ("file_id", "version_id", "seq", "name", "sha256", "size_bytes", "media_type", "scan_state", "scan_waiver_ref"))
contract_references = {"counterparty": nullable(ref("BusinessPartyReference")), "draft_file": nullable(ref("BusinessFileReference"))}
add("ContractPaymentPlanItem", {"id": ID, "title": NAME, "amount": nullable(MONEY), "due_date": nullable(DATE), "conditions": TEXT},
    ("id", "title", "amount", "due_date", "conditions"))
add("ContractPaymentSchedule", {"mode": enum("scheduled", "no_payment"), "reason": TEXT, "items": array(ref("ContractPaymentPlanItem"), maxItems=100)},
    ("mode", "reason", "items"))
SCHEMAS["ContractPaymentSchedule"]["allOf"] = [
    {"if": {"properties": {"mode": {"const": "scheduled"}}}, "then": {"properties": {"items": {"minItems": 1}}}},
    {"if": {"properties": {"mode": {"const": "no_payment"}}}, "then": {"properties": {"items": {"maxItems": 0}, "reason": {"minLength": 1}}}}
]
contract_content = {"payment_schedule": nullable(ref("ContractPaymentSchedule")), "title": NAME, "direction": nullable(enum("receivable", "payable", "none")), "type": nullable(NAME),
    "category": {"type": "string", "maxLength": 50}, "counterparty_id": nullable(ID), "counterparty_revision_id": nullable(ID),
    "owner_id": nullable(ID), "currency": nullable(CURRENCY), "amount": nullable(MONEY), "tax_amount": nullable(MONEY),
    "start_date": nullable(DATE), "end_date": nullable(DATE), "renewal_terms": TEXT, "termination_terms": TEXT,
    "notes": TEXT, "draft_file_version_id": nullable(ID)}
add("ContractCreate", {**contract_content, "project_id": nullable(ID), "parent_contract_id": nullable(ID), "classification": CLASS, "reason": NAME},
    ("title", "classification", "reason"))
add("ContractPatch", {**EXPECTED, **contract_content, "base_revision_id": ID, "reason": NAME},
    ("expected_version", "base_revision_id", *contract_content, "reason"))
add("ContractSignatureRecord", {"id": ID, "revision_id": ID, "request_id": ID, "request_revision_id": ID, "instance_id": ID,
    "signed_on": DATE, "effective_on": DATE, "signed_file_version_id": ID, "signed_file": nullable(ref("BusinessFileReference")),
    "attestation": TEXT, "reason": NAME, "actor_id": ID, "created_at": TIME},
    ("id", "revision_id", "request_id", "request_revision_id", "instance_id", "signed_on", "effective_on", "signed_file_version_id", "signed_file", "attestation", "reason", "actor_id", "created_at"))
add("Contract", {"payload_schema_version": {"type": "integer", "enum": [1, 2]}, "signature": nullable(ref("ContractSignatureRecord")), "can_register_signature": {"type": "boolean"}, "can_activate": {"type": "boolean"}, **RECORD, **contract_content, **contract_references, "project_id": nullable(ID), "creator_id": ID, "parent_contract_id": nullable(ID), "classification": CLASS,
    "contract_no": NAME, "current_revision_id": ID, "approved_revision_id": nullable(ID), "active_revision_id": nullable(ID), "request_id": nullable(ID),
    "content_hash": HASH, "deleted_at": nullable(TIME), "can_edit": {"type": "boolean"}, "can_prepare_request": {"type": "boolean"}, "can_read_history": {"type": "boolean"}, "can_trash": {"type": "boolean"},
    "lifecycle": enum("draft", "pending_signature", "effective", "completed", "terminated"),
    "approval_state": enum("not_submitted", "in_review", "approved", "returned", "rejected", "withdrawn")},
    ("payload_schema_version", "signature", "can_register_signature", "can_activate", *RECORD, *contract_content, *contract_references, "project_id", "creator_id", "parent_contract_id", "classification", "contract_no", "current_revision_id",
     "approved_revision_id", "active_revision_id", "request_id", "content_hash", "deleted_at", "can_edit", "can_prepare_request", "can_read_history", "can_trash", "lifecycle", "approval_state"))
add("ContractRevision", {**contract_content, **contract_references, "id": ID, "contract_id": ID, "seq": VERSION, "base_revision_id": nullable(ID),
    "payload_schema_version": {"type": "integer", "enum": [1, 2]}, "content_hash": HASH, "author_id": ID, "reason": NAME, "created_at": TIME},
    (*contract_content, *contract_references, "id", "contract_id", "seq", "base_revision_id", "payload_schema_version", "content_hash", "author_id", "reason", "created_at"))
add("ContractTrashInput", {**EXPECTED, "action": enum("trash", "restore"), "reason": NAME}, ("expected_version", "action", "reason"))
# Shared physical revision store; separate purchase/claim business permissions.
supplier = {"party_id": ID, "revision_id": ID}
add("SpendSupplier", supplier, tuple(supplier))
add("SpendSupplierEvidence", {**supplier, "name": NAME, "kind": enum("organization", "person")}, (*supplier, "name", "kind"))
spend_line = {"id": ID, "description": {"type": "string", "maxLength": 1000}, "quantity": nullable(MONEY), "unit": {"type": "string", "maxLength": 50},
    "unit_price": nullable(MONEY), "declared_amount": nullable(MONEY), "tax_amount": nullable(MONEY), "discount_amount": nullable(MONEY),
    "expense_date": nullable(DATE), "category": {"type": "string", "maxLength": 50}, "supplier": nullable(ref("SpendSupplier")),
    "receipt_version_ids": array(ID, maxItems=20, uniqueItems=True), "missing_receipt_reason": {"type": "string", "maxLength": 2000},
    "duplicate_receipt_reason": {"type": "string", "maxLength": 2000}, "funding": enum("", "employee_advance", "company_paid")}
add("SpendLine", spend_line, tuple(spend_line))
spend_content = {"title": NAME, "category": {"type": "string", "maxLength": 50}, "purpose": {"type": "string", "maxLength": 4000},
    "owner_id": nullable(ID), "currency": nullable(CURRENCY), "pricing_basis": enum("", "net_plus_tax", "gross_includes_tax"),
    "rounding_mode": enum("", "half_up", "half_even"), "expected_delivery_date": nullable(DATE), "period_start": nullable(DATE), "period_end": nullable(DATE),
    "budget_reference": {"type": "string", "maxLength": 500}, "supplier_candidates": array(ref("SpendSupplier"), maxItems=20, uniqueItems=True),
    "quote_version_ids": array(ID, maxItems=20, uniqueItems=True), "lines": array(ref("SpendLine"), maxItems=100), "notes": TEXT}
add("SpendContent", spend_content, tuple(spend_content))
add("SpendLineInput", {**spend_line, "receipt_version_ids": nullable(spend_line["receipt_version_ids"])}, ("id", "description"))
spend_input = {**spend_content, "lines": nullable(array(ref("SpendLineInput"), maxItems=100)),
    "supplier_candidates": nullable(spend_content["supplier_candidates"]), "quote_version_ids": nullable(spend_content["quote_version_ids"])}
add("SpendCreate", {**spend_input, "project_id": nullable(ID), "classification": CLASS, "reason": NAME}, ("title", "classification", "reason"))
add("SpendPatch", {**EXPECTED, **spend_input, "base_revision_id": ID, "reason": NAME}, ("expected_version", *spend_input, "base_revision_id", "reason"))
add("SpendArchiveInput", {**EXPECTED, "reason": NAME}, ("expected_version", "reason"))
add("SpendLineValue", {"line_id": ID, "amount": nullable(MONEY), "duplicate_receipt": {"type": "boolean"}}, ("line_id", "amount", "duplicate_receipt"))
spend_values = {"amount": nullable(MONEY), "employee_reimbursement_amount": nullable(MONEY), "line_values": array(ref("SpendLineValue")),
    "suppliers": array(ref("SpendSupplierEvidence")), "attachments": array(ref("BusinessFileReference"))}
spend_record = {**RECORD, **spend_content, **spend_values, "kind": enum("purchase", "claim"), "document_no": NAME, "project_id": nullable(ID),
    "creator_id": ID, "classification": CLASS, "lifecycle": enum("draft", "requested", "ordering", "receiving", "cancelled", "completed"), "approval_state": enum("not_submitted", "in_review", "approved", "returned", "rejected", "withdrawn"),
    "current_revision_id": ID, "approved_revision_id": nullable(ID), "request_id": nullable(ID), "payload_schema_version": {"const": 1}, "content_hash": HASH,
    "can_edit": {"type": "boolean"}, "can_archive": {"type": "boolean"}, "can_execute": {"type": "boolean"}, "can_create_variance": {"type": "boolean"}, "can_prepare_request": {"type": "boolean"}, "can_read_history": {"type": "boolean"}, "deleted_at": nullable(TIME)}
add("Spend", spend_record, tuple(spend_record))
spend_revision = {**spend_content, **spend_values, "id": ID, "document_id": ID, "seq": VERSION, "base_revision_id": nullable(ID),
    "payload_schema_version": {"const": 1}, "content_hash": HASH, "author_id": ID, "reason": NAME, "created_at": TIME}
add("SpendRevision", spend_revision, tuple(spend_revision))
# Approval commands bind business revisions, submission preview and human decisions.
request_states = enum("draft", "in_review", "approved", "returned", "rejected", "withdrawn")
request_base = {"expected_business_version": VERSION, "business_revision_id": ID, "business_content_hash": HASH, "reason": NAME}
add("RequestCreate", {"kind": enum("contract", "purchase", "claim", "purchase_variance", "change"), "business_id": ID, **request_base}, ("kind", "business_id", *request_base))
add("RequestRevise", {**EXPECTED, **request_base}, ("expected_version", *request_base))
add("RequestSubmit", {**EXPECTED, "revision_id": ID, "content_hash": HASH, "workflow_definition_id": ID, "preview_hash": HASH, "reason": NAME},
    ("expected_version", "revision_id", "content_hash", "workflow_definition_id", "preview_hash", "reason"))
add("RequestWithdraw", {**EXPECTED, "revision_id": ID, "content_hash": HASH, "reason": NAME}, ("expected_version", "revision_id", "content_hash", "reason"))
add("ApprovalDecision", {**EXPECTED, "request_version": VERSION, "revision_id": ID, "content_hash": HASH,
    "decision": enum("approve", "return", "reject"), "reason": TEXT}, ("expected_version", "request_version", "revision_id", "content_hash", "decision"))
SCHEMAS["ApprovalDecision"]["allOf"] = [{"if": {"properties": {"decision": {"enum": ["return", "reject"]}}},
    "then": {"required": ["reason"], "properties": {"reason": {"type": "string", "minLength": 1}}}}]
request_payload = {"kind": {"const": "contract"}, "business_id": ID, "business_revision_id": ID, "business_content_hash": HASH,
    "contract_no": NAME, "project_id": nullable(ID), "classification": CLASS, "content": obj(contract_content, tuple(key for key in contract_content if key != "payment_schedule")),
    "counterparty": nullable(ref("BusinessPartyReference")), "attachments": array(ref("BusinessFileReference"))}
request_payload["business_payload_schema_version"] = {"type": "integer", "enum": [1, 2]}
add("ContractRequestPayload", request_payload, tuple(key for key in request_payload if key != "business_payload_schema_version"))
spend_payload = {"kind": enum("purchase", "claim"), "business_id": ID, "business_revision_id": ID, "business_content_hash": HASH,
    "business_payload_schema_version": {"const": 1}, "document_no": NAME, "project_id": nullable(ID), "classification": CLASS,
    "content": ref("SpendContent"), "amount": nullable(MONEY), "employee_reimbursement_amount": nullable(MONEY),
    "suppliers": array(ref("SpendSupplierEvidence")), "attachments": array(ref("BusinessFileReference")), "line_values": array(ref("SpendLineValue"))}
add("SpendRequestPayload", spend_payload, tuple(spend_payload))
variance_basis = {"purchase_id": ID, "purchase_version": VERSION, "original_revision_id": ID, "original_content_hash": HASH,
    "execution_plan_revision_id": nullable(ID), "content": ref("SpendContent"), "amount": nullable(MONEY),
    "line_values": array(ref("SpendLineValue")), "commitments": array(ref("PurchaseFulfillmentLine"))}
add("PurchaseVarianceBasis", variance_basis, tuple(variance_basis))
variance_payload = {**spend_payload, "kind": {"const": "purchase_variance"}, "purchase_variance": ref("PurchaseVarianceBasis")}
add("PurchaseVarianceRequestPayload", variance_payload, tuple(variance_payload))
SCHEMAS["RequestPayload"] = {"oneOf": [ref("ContractRequestPayload"), ref("SpendRequestPayload"), ref("PurchaseVarianceRequestPayload")]}
approval_step = {"id": ID, "version": VERSION, "request_id": ID, "request_version": VERSION, "request_title": NAME, "revision_id": ID, "instance_id": ID, "seq": {"type": "integer", "minimum": 1, "maximum": 10},
    "title": NAME, "assigned_user_id": ID, "assigned_user_name": NAME, "assigned_user_active": {"type": "boolean"}, "requires_mfa": {"type": "boolean"},
    "state": enum("pending", "active", "approved", "returned", "rejected", "cancelled"), "decision": nullable(enum("approve", "return", "reject")),
    "decision_reason": nullable(TEXT), "decided_at": nullable(TIME), "can_decide": {"type": "boolean"}, "updated_at": TIME}
add("ApprovalTask", approval_step, (*approval_step,))
request_fields = {**RECORD, "title": NAME, "kind": enum("contract", "purchase", "claim", "purchase_variance", "change"), "business_id": ID, "applicant_id": ID, "project_id": nullable(ID), "classification": CLASS,
    "state": request_states, "current_revision_id": ID, "active_instance_id": nullable(ID), "content_hash": HASH, "payload": ref("RequestPayload"),
    "workflow": nullable(ref("WorkflowDefinition")), "steps": array(ref("ApprovalTask")), "can_revise": {"type": "boolean"}, "can_submit": {"type": "boolean"}, "can_withdraw": {"type": "boolean"}}
add("Request", request_fields, (*request_fields,))
request_revision = {"id": ID, "request_id": ID, "seq": VERSION, "base_revision_id": nullable(ID), "payload_schema_version": {"const": 1},
    "payload": ref("RequestPayload"), "workflow": nullable(ref("WorkflowDefinition")), "content_hash": HASH, "steps": array(ref("ApprovalTask")),
    "author_id": ID, "reason": NAME, "created_at": TIME}
add("RequestRevision", request_revision, (*request_revision,))
preview = {"request_id": ID, "request_version": VERSION, "revision_id": ID, "content_hash": HASH, "preview_hash": HASH,
    "workflow": ref("WorkflowDefinition"), "payload": ref("RequestPayload")}
add("SubmissionPreview", preview, (*preview,))

add("ContractSignature", {**EXPECTED, "revision_id": ID, "content_hash": HASH, "signed_on": DATE, "signed_file_version_id": ID,
    "attestation": {"type": "string", "minLength": 1, "maxLength": 20000}, "reason": NAME},
    ("expected_version", "revision_id", "content_hash", "signed_on", "signed_file_version_id", "attestation", "reason"))
add("ContractActivation", {**EXPECTED, "action": {"const": "activate"}, "revision_id": ID, "content_hash": HASH, "signature_id": ID, "reason": NAME},
    ("expected_version", "action", "revision_id", "content_hash", "signature_id", "reason"))
add("ContractScheduleInput", {**EXPECTED, "base_revision_id": ID, "payment_schedule": nullable(ref("ContractPaymentSchedule")), "reason": NAME},
    ("expected_version", "base_revision_id", "payment_schedule", "reason"))
milestone_fields = {"id": ID, "contract_id": ID, "revision_id": ID, "plan_item_id": ID, "position": {"type": "integer", "minimum": 1, "maximum": 100},
    "title": NAME, "direction": enum("receivable", "payable"), "currency": CURRENCY, "amount": MONEY, "due_date": DATE, "conditions": TEXT, "created_at": TIME}
add("ContractMilestone", milestone_fields, (*milestone_fields,))

finance_version = {"type": "integer", "minimum": 0, "maximum": 9007199254740990}
finance_fingerprint = {"type": "string", "pattern": "^[a-f0-9]{32}$"}
finance_base = {"expected_version": finance_version, "contract_version": VERSION, "reason": NAME}
finance_link = {**finance_base, "milestone_id": ID, "kind": enum("cost", "cash", "refund"), "source_id": ID,
    "source_version": VERSION, "source_revision_id": ID, "source_content_hash": HASH, "source_event_id": nullable(ID), "amount": MONEY}
add("ContractFinanceLink", finance_link, tuple(finance_link))
add("ContractFinanceRelease", {**finance_base, "amount": MONEY}, (*finance_base, "amount"))
finance_review = {**finance_base, "expected_fingerprint": finance_fingerprint, "conditions_satisfied": {"type": "boolean"},
    "acknowledge_difference": {"type": "boolean"}, "acknowledge_unattributed_refunds": {"type": "boolean"}, "evidence_version_ids": array(ID, minItems=1, maxItems=20, uniqueItems=True)}
add("ContractFinanceReview", finance_review, tuple(finance_review))
add("ContractFinanceReceipt", {"contract_id": ID, "milestone_id": ID, "version": VERSION, "event_id": ID}, ("contract_id", "milestone_id", "version", "event_id"))
finance_node = {key: milestone_fields[key] for key in ("id", "position", "title", "direction", "currency", "amount", "due_date", "conditions")}
finance_node.update({"cost_amount": nullable(MONEY), "cost_visibility": enum("complete", "restricted", "not_applicable"),
    "cash_amount": nullable(report_decimal), "cash_visibility": enum("complete", "restricted"), "outstanding_amount": nullable(MONEY), "excess_amount": nullable(report_decimal),
    "has_unattributed_refunds": nullable({"type": "boolean"}), "fingerprint": nullable(finance_fingerprint), "review_state": enum("restricted", "unreviewed", "stale", "current"),
    "review_id": nullable(ID), "conditions_satisfied": nullable({"type": "boolean"}), "can_review": {"type": "boolean"}})
add("ContractFinanceNode", finance_node, tuple(finance_node))
finance_summary = {"contract_id": ID, "contract_version": VERSION, "active_revision_id": nullable(ID), "schedule_mode": nullable(enum("scheduled", "no_payment")),
    "version": finance_version, "can_reconcile": {"type": "boolean"}, "history_visibility": enum("complete", "restricted"), "nodes": array(ref("ContractFinanceNode"), maxItems=100)}
add("ContractFinance", finance_summary, tuple(finance_summary))
finance_snapshot_source = {"event_id": ID, "kind": enum("cost", "cash", "refund"), "amount": MONEY, "remaining_amount": MONEY,
    "source_event_id": nullable(ID), "cost_id": nullable(ID), "payment_id": nullable(ID), "pinned_revision_id": ID,
    "current_version": VERSION, "current_revision_id": ID, "content_hash": HASH, "state": enum("confirmed", "recorded", "reversed"),
    "source_amount": MONEY, "currency": CURRENCY, "direction": nullable(enum("outgoing", "incoming", "refund", "transfer")),
    "effective_amount": report_decimal, "unattributed_refund_amount": MONEY, "refund_family_fingerprint": nullable(finance_fingerprint)}
add("ContractFinanceSnapshotSource", finance_snapshot_source, tuple(finance_snapshot_source))
finance_snapshot = {"milestone_id": ID, "contract_revision_id": ID, "sources": array(ref("ContractFinanceSnapshotSource"), maxItems=100)}
add("ContractFinanceSnapshot", finance_snapshot, tuple(finance_snapshot))
finance_event = {"id": ID, "contract_id": ID, "milestone_id": ID, "contract_revision_id": ID, "seq": VERSION, "kind": enum("cost", "cash", "refund", "release", "review"),
    "cost_id": nullable(ID), "payment_id": nullable(ID), "source_version": nullable(VERSION), "source_revision_id": nullable(ID), "source_content_hash": nullable(HASH), "source_event_id": nullable(ID),
    "amount": nullable(MONEY), "remaining_amount": nullable(MONEY), "effective_amount": report_decimal, "review_snapshot": nullable(ref("ContractFinanceSnapshot")),
    "review_fingerprint": nullable(finance_fingerprint), "conditions_satisfied": nullable({"type": "boolean"}), "acknowledge_difference": nullable({"type": "boolean"}), "acknowledge_unattributed_refunds": nullable({"type": "boolean"}),
    "actor_id": ID, "actor_name": NAME, "reason": NAME, "created_at": TIME, "attachments": array(ref("BusinessFileReference")), "can_release": {"type": "boolean"}}
add("ContractFinanceEvent", finance_event, tuple(finance_event))

closure_node = {key: milestone_fields[key] for key in ("id", "title", "direction", "currency", "amount")}
closure_node.update({"cash_amount": report_decimal, "has_unattributed_refunds": {"type": "boolean"}, "review_id": nullable(ID),
    "review_current": {"type": "boolean"}, "conditions_satisfied": nullable({"type": "boolean"}), "sources": ref("ContractFinanceSnapshot")})
add("ContractClosureNode", closure_node, tuple(closure_node))
closure_cost = {"id": ID, "version": VERSION, "revision_id": ID, "content_hash": HASH, "state": enum("confirmed", "reversed"),
    "currency": CURRENCY, "amount": MONEY, "paid_amount": report_decimal, "allocation_fingerprint": finance_fingerprint}
add("ContractClosureCost", closure_cost, tuple(closure_cost))
closure_child = {"id": ID, "version": VERSION, "lifecycle": enum("draft", "pending_signature", "effective", "completed", "terminated")}
add("ContractClosureChild", closure_child, tuple(closure_child))
closure_basis = {"revision_id": ID, "finance_version": finance_version, "schedule_mode": nullable(enum("scheduled", "no_payment")),
    "nodes": array(ref("ContractClosureNode"), maxItems=100), "costs": array(ref("ContractClosureCost"), maxItems=1000),
    "children": array(ref("ContractClosureChild"), maxItems=1000)}
add("ContractClosureSnapshot", closure_basis, tuple(closure_basis))
closure_blockers = enum("contract_not_effective", "restricted", "plan_unknown", "node_amount_difference", "unattributed_refunds",
    "node_review_required", "conditions_unconfirmed", "cost_unsettled", "supplement_unresolved", "supplement_effective")
closure_preview = {"contract_id": ID, "contract_version": VERSION, "visibility": enum("complete", "restricted", "not_active"),
    "basis": nullable(ref("ContractClosureSnapshot")), "fingerprint": nullable(finance_fingerprint), "blockers": array(closure_blockers, uniqueItems=True),
    "can_complete": {"type": "boolean"}, "can_terminate": {"type": "boolean"}}
add("ContractClosurePreview", closure_preview, tuple(closure_preview))
closure_input = {**EXPECTED, "revision_id": ID, "expected_finance_version": finance_version, "expected_fingerprint": finance_fingerprint,
    "action": enum("completed", "terminated"), "occurred_on": DATE, "terms_verified": {"type": "boolean"},
    "attestation": {"type": "string", "minLength": 1, "maxLength": 20000}, "acknowledge_unsettled": {"type": "boolean"},
    "unsettled_disposition": TEXT, "evidence_version_ids": array(ID, minItems=1, maxItems=20, uniqueItems=True), "reason": NAME}
add("ContractClosureInput", closure_input, tuple(closure_input))
add("ContractClosureReceipt", {"contract_id": ID, "version": VERSION, "closure_id": ID, "action": enum("completed", "terminated")},
    ("contract_id", "version", "closure_id", "action"))
closure_record = {"id": ID, "contract_id": ID, "contract_version": VERSION, "action": enum("completed", "terminated"), "occurred_on": DATE,
    "basis": ref("ContractClosureSnapshot"), "fingerprint": finance_fingerprint, "basis_state": enum("current", "changed"),
    "terms_verified": {"type": "boolean"}, "attestation": TEXT, "acknowledge_unsettled": {"type": "boolean"}, "unsettled_disposition": TEXT,
    "actor_id": ID, "actor_name": NAME, "reason": NAME, "created_at": TIME, "attachments": array(ref("BusinessFileReference"), minItems=1, maxItems=20)}
add("ContractClosure", closure_record, tuple(closure_record))

purchase_item = obj({"description": NAME, "quantity": MONEY, "unit": NAME, "unit_price": MONEY,
    "tax": MONEY, "discount": MONEY}, ("description", "quantity", "unit_price"))
purchase_fields = {"purpose": TEXT, "project_id": nullable(ID), "currency": CURRENCY,
    "items": array(purchase_item, minItems=1), "quote_version_ids": array(ID), "expected_delivery_on": DATE}
add("PurchaseCreate", purchase_fields, ("purpose", "currency", "items"))
add("PurchasePatch", {**EXPECTED, **purchase_fields}, ("expected_version",))
add("PurchaseItem", {"id": ID, "line_no": VERSION, **purchase_item["properties"], "line_total": MONEY},
    ("id", "line_no", "description", "quantity", "unit_price", "line_total"))
add("Purchase", {**RECORD, **purchase_fields, "items": array(ref("PurchaseItem")), "purchase_no": NAME,
    "request_id": nullable(ID), "state": enum("draft", "requested", "ordering", "receiving", "completed", "cancelled")},
    ("id", "version", "purchase_no", "currency", "items", "state"))
execution_base = {**EXPECTED, "revision_id": ID, "content_hash": HASH, "reason": NAME}
add("PurchaseExecutionInput", execution_base, tuple(execution_base))
order_item = {"purchase_line_id": ID, "quantity": MONEY, "unit_price": MONEY, "tax_amount": MONEY, "discount_amount": MONEY}
add("PurchaseOrderLineInput", order_item, tuple(order_item))
execution_files = array(ID, minItems=1, maxItems=20, uniqueItems=True)
execution_notes = {"type": "string", "maxLength": 4000}
order_input = {**execution_base, "execution_plan_revision_id": nullable(ID), "supplier": ref("SpendSupplier"), "external_ref": NAME, "ordered_on": DATE, "expected_delivery_on": DATE,
    "evidence_version_ids": execution_files, "lines": array(ref("PurchaseOrderLineInput"), minItems=1, maxItems=100), "notes": execution_notes}
add("PurchaseOrderCreate", order_input, tuple(key for key in order_input if key != "execution_plan_revision_id"))
order_line = {**order_item, "id": ID, "description": {"type": "string", "maxLength": 1000}, "unit": {"type": "string", "maxLength": 50},
    "amount": MONEY, "net_accepted_quantity": MONEY, "outstanding_quantity": MONEY}
add("PurchaseOrderLine", order_line, tuple(order_line))
execution_record = {"id": ID, "purchase_id": ID, "purchase_version": VERSION, "evidence_version_ids": execution_files,
    "attachments": array(ref("BusinessFileReference")), "notes": execution_notes, "actor_id": ID, "reason": NAME, "created_at": TIME}
order_record = {**execution_record, "revision_id": ID, "request_revision_id": ID, "execution_plan_revision_id": nullable(ID), "supplier": ref("SpendSupplier"),
    "supplier_evidence": nullable(ref("SpendSupplierEvidence")), "external_ref": NAME, "ordered_on": DATE, "expected_delivery_on": DATE,
    "currency": CURRENCY, "amount": MONEY, "lines": array(ref("PurchaseOrderLine"), minItems=1, maxItems=100), "state": enum("open", "fulfilled", "cancelled")}
add("PurchaseOrder", order_record, tuple(order_record))
receipt_line = {"order_line_id": ID, "quantity": MONEY, "result": enum("accepted", "rejected", "returned"),
    "reverses_line_id": nullable(ID), "reason": {"type": "string", "minLength": 1, "maxLength": 2000}}
add("PurchaseReceiptLineInput", receipt_line, tuple(receipt_line))
receipt_input = {**execution_base, "order_id": ID, "received_on": DATE, "evidence_version_ids": execution_files,
    "lines": array(ref("PurchaseReceiptLineInput"), minItems=1, maxItems=100), "notes": execution_notes}
add("PurchaseReceiptCreate", receipt_input, tuple(receipt_input))
receipt_record_line = {**receipt_line, "id": ID, "returnable_quantity": MONEY}
add("PurchaseReceiptLine", receipt_record_line, tuple(receipt_record_line))
receipt_record = {**execution_record, "order_id": ID, "received_on": DATE, "lines": array(ref("PurchaseReceiptLine"), minItems=1, maxItems=100)}
add("PurchaseReceipt", receipt_record, tuple(receipt_record))
execution_event = {"id": ID, "seq": VERSION, "purchase_version": VERSION,
    "kind": enum("order_registered", "receipt_registered", "order_cancelled", "purchase_cancelled", "plan_activated", "purchase_completed"), "order_id": nullable(ID), "receipt_id": nullable(ID), "variance_revision_id": nullable(ID),
    "lifecycle": enum("requested", "ordering", "receiving", "cancelled", "completed"), "actor_id": ID, "reason": NAME, "created_at": TIME}
add("PurchaseExecutionEvent", execution_event, tuple(execution_event))
fulfillment_line = {"line_id": ID, **{name: MONEY for name in ("approved_quantity", "reserved_quantity", "net_accepted_quantity", "outstanding_quantity",
    "approved_amount", "reserved_amount", "reserved_tax", "reserved_discount")}}
add("PurchaseFulfillmentLine", fulfillment_line, tuple(fulfillment_line))
fulfillment = {"purchase_id": ID, "version": VERSION, "lifecycle": enum("requested", "ordering", "receiving", "cancelled", "completed"), "currency": nullable(CURRENCY),
    **{name: {"type": "integer", "minimum": 0} for name in ("order_count", "receipt_count", "cancelled_order_count")},
    "lines": array(ref("PurchaseFulfillmentLine"), minItems=1, maxItems=100)}
add("PurchaseFulfillment", fulfillment, tuple(fulfillment))
variance_input = {**spend_content, "expected_purchase_version": VERSION, "execution_plan_revision_id": nullable(ID), "reason": NAME}
add("PurchaseVarianceCreate", variance_input, tuple(variance_input))
add("PurchaseVariancePatch", {**variance_input, **EXPECTED, "base_revision_id": ID}, (*variance_input, "expected_version", "base_revision_id"))
add("PurchaseVarianceActivation", {**execution_base, "expected_purchase_version": VERSION}, (*execution_base, "expected_purchase_version"))
variance_record = {**spend_record, "kind": {"const": "purchase_variance"}, "lifecycle": enum("draft", "requested", "effective"),
    "basis": ref("PurchaseVarianceBasis"), "can_activate": {"type": "boolean"}}
add("PurchaseVariance", variance_record, tuple(variance_record))
variance_revision = {**spend_revision, "basis": ref("PurchaseVarianceBasis")}
add("PurchaseVarianceRevision", variance_revision, tuple(variance_revision))
execution_plan = {"purchase_id": ID, "purchase_version": VERSION, "variance_id": nullable(ID), "variance_revision_id": nullable(ID),
    "variance_request_id": nullable(ID), "content": ref("SpendContent"), "amount": nullable(MONEY), "line_values": array(ref("SpendLineValue")), "activated_at": nullable(TIME),
    "suppliers": array(ref("SpendSupplierEvidence")), "attachments": array(ref("BusinessFileReference"))}
add("PurchaseExecutionPlan", execution_plan, tuple(execution_plan))

completion_order_line = {key: value for key, value in order_line.items() if key != "outstanding_quantity"}
add("PurchaseCompletionOrderLine", completion_order_line, tuple(completion_order_line))
completion_order = {"id": ID, "purchase_version": VERSION, "execution_plan_revision_id": nullable(ID), "external_ref": NAME,
    "supplier_id": ID, "supplier_revision_id": ID, "ordered_on": DATE, "amount": MONEY, "cancelled": {"type": "boolean"},
    "lines": array(ref("PurchaseCompletionOrderLine"), minItems=1, maxItems=100)}
add("PurchaseCompletionOrder", completion_order, tuple(completion_order))
completion_receipt_line = {**receipt_line, "id": ID}
add("PurchaseCompletionReceiptLine", completion_receipt_line, tuple(completion_receipt_line))
completion_receipt = {"id": ID, "order_id": ID, "purchase_version": VERSION, "received_on": DATE,
    "lines": array(ref("PurchaseCompletionReceiptLine"), minItems=1, maxItems=100)}
add("PurchaseCompletionReceipt", completion_receipt, tuple(completion_receipt))
completion_variance = {"id": ID, "version": VERSION, "revision_id": ID, "content_hash": HASH, "title": NAME,
    "approval_state": enum("not_submitted", "in_review", "approved", "returned", "rejected", "withdrawn"),
    "lifecycle": enum("draft", "requested", "effective"), "basis_purchase_version": VERSION}
add("PurchaseCompletionVariance", completion_variance, tuple(completion_variance))
completion_cost = {"id": ID, "version": VERSION, "revision_id": ID, "content_hash": HASH, "title": NAME,
    "entry_kind": enum("expense", "reversal"), "state": enum("draft", "confirmed", "reversed"), "currency": nullable(CURRENCY),
    "amount": nullable(MONEY), "paid_amount": MONEY}
add("PurchaseCompletionCost", completion_cost, tuple(completion_cost))
completion_allocation = {"id": ID, "cost_id": ID, "cost_revision_id": ID,
    "kind": enum("allocation", "refund", "release", "reversal"), "effect": {"type": "integer", "enum": [-1, 1]}, "amount": MONEY}
add("PurchaseCompletionAllocation", completion_allocation, tuple(completion_allocation))
completion_payment = {"id": ID, "version": VERSION, "revision_id": ID, "content_hash": HASH,
    "state": enum("draft", "recorded", "reversed"), "record_kind": enum("payment", "reversal"),
    "direction": nullable(enum("incoming", "outgoing", "refund")), "currency": nullable(CURRENCY), "amount": nullable(MONEY),
    "refund_of_id": nullable(ID), "unattributed_refund_amount": MONEY, "allocations": array(ref("PurchaseCompletionAllocation"))}
add("PurchaseCompletionPayment", completion_payment, tuple(completion_payment))
completion_basis = {"revision_id": ID, "content_hash": HASH, "currency": CURRENCY, "execution_plan_revision_id": nullable(ID),
    "execution_amount": MONEY, "lines": array(ref("PurchaseFulfillmentLine"), minItems=1, maxItems=100),
    "orders": array(ref("PurchaseCompletionOrder"), maxItems=1000), "receipts": array(ref("PurchaseCompletionReceipt"), maxItems=5000),
    "variances": array(ref("PurchaseCompletionVariance"), maxItems=1000), "costs": array(ref("PurchaseCompletionCost"), maxItems=1000),
    "payments": array(ref("PurchaseCompletionPayment"), maxItems=1000)}
add("PurchaseCompletionSnapshot", completion_basis, tuple(completion_basis))
completion_mode = enum("recorded_costs", "no_payable_obligation")
completion_preview = {"purchase_id": ID, "purchase_version": VERSION, "visibility": enum("restricted", "not_approved", "complete"),
    "basis": nullable(ref("PurchaseCompletionSnapshot")), "fingerprint": nullable(finance_fingerprint),
    "blockers": array(enum("restricted", "not_approved", "no_accepted_delivery", "order_unfulfilled", "variance_in_review", "cost_unconfirmed", "cost_unsettled", "unattributed_refund", "currency_mismatch"), uniqueItems=True),
    "has_difference": {"type": "boolean"}, "settlement_modes": array(completion_mode, uniqueItems=True), "can_complete": {"type": "boolean"}}
add("PurchaseCompletionPreview", completion_preview, tuple(completion_preview))
completion_input = {**execution_base, "expected_fingerprint": finance_fingerprint, "occurred_on": DATE,
    "delivery_verified": {"type": "boolean"}, "costs_verified": {"type": "boolean"}, "differences_verified": {"type": "boolean"},
    "settlement_mode": completion_mode, "attestation": {**TEXT, "minLength": 1}, "difference_disposition": TEXT, "evidence_version_ids": execution_files}
add("PurchaseCompletionInput", completion_input, tuple(completion_input))
add("PurchaseCompletionCommandReceipt", {"purchase_id": ID, "version": VERSION, "completion_id": ID}, ("purchase_id", "version", "completion_id"))
completion_record = {"id": ID, "purchase_id": ID, "purchase_version": VERSION, "occurred_on": DATE,
    "basis": ref("PurchaseCompletionSnapshot"), "fingerprint": finance_fingerprint, "basis_state": enum("current", "changed"),
    **{key: completion_input[key] for key in ("delivery_verified", "costs_verified", "differences_verified", "settlement_mode", "attestation", "difference_disposition")},
    "actor_id": ID, "actor_name": NAME, "reason": NAME, "created_at": TIME, "attachments": array(ref("BusinessFileReference"))}
add("PurchaseCompletion", completion_record, tuple(completion_record))

claim_item = obj({"expense_date": DATE, "supplier_id": ID, "category": NAME, "amount": MONEY,
    "receipt_version_ids": array(ID), "linked_cost_id": ID, "missing_receipt_reason": TEXT},
    ("expense_date", "category", "amount"))
claim_fields = {"project_id": nullable(ID), "currency": CURRENCY, "description": TEXT,
    "items": array(claim_item, minItems=1)}
add("ClaimCreate", claim_fields, ("currency", "description", "items"))
add("ClaimPatch", {**EXPECTED, **claim_fields}, ("expected_version",))
add("Claim", {**RECORD, **claim_fields, "claim_no": NAME, "request_id": nullable(ID), "total": MONEY,
    "settlement_state": enum("unsettled", "partially_settled", "settled")},
    ("id", "version", "claim_no", "currency", "total", "settlement_state"))

add("MilestoneCreate", {"title": NAME, "owner_id": ID, "due_at": TIME}, ("title", "owner_id", "due_at"))
add("MilestonePatch", {**EXPECTED, "title": NAME, "owner_id": ID, "due_at": TIME}, ("expected_version",))
add("Milestone", {**RECORD, "project_id": ID, "creator_id": ID, "title": NAME, "owner_id": nullable(ID), "due_at": nullable(TIME),
    "overdue": {"type": "boolean"},
    "state": enum("open", "achieved", "missed", "cancelled")}, ("id", "version", "project_id", "title", "state"))
deliverable_content = {"name": NAME, "acceptance_criteria": {**TEXT, "minLength": 1}, "owner_id": ID, "milestone_id": nullable(ID), "reason": NAME}
add("DeliverableCreate", deliverable_content, tuple(deliverable_content))
add("DeliverablePatch", {**EXPECTED, **deliverable_content}, ("expected_version", *deliverable_content))
deliverable_output = {**RECORD, "project_id": ID, "creator_id": nullable(ID), "name": NAME, "acceptance_criteria": TEXT,
    "owner_id": ID, "milestone_id": nullable(ID), "current_revision_id": nullable(ID), "state": enum("draft", "submitted", "accepted", "changes_requested"),
    "protocol_version": {"type": "integer", "enum": [0, 1]}, "can_edit": {"type": "boolean"}, "can_submit": {"type": "boolean"}, "can_decide": {"type": "boolean"}}
add("Deliverable", deliverable_output, tuple(deliverable_output))
add("DeliverableSubmit", {**EXPECTED, "expected_revision_id": nullable(ID), "file_version_ids": array(ID, minItems=1, maxItems=20, uniqueItems=True), "description": {**TEXT, "minLength": 1}, "reason": NAME},
    ("expected_version", "expected_revision_id", "file_version_ids", "description", "reason"))
add("DeliverableDecisionInput", {**EXPECTED, "revision_id": ID, "content_hash": HASH, "decision": enum("accept", "request_changes"),
    "files_verified": {"type": "boolean"}, "criteria_verified": {"type": "boolean"}, "reason": {**TEXT, "minLength": 1}},
    ("expected_version", "revision_id", "content_hash", "decision", "files_verified", "criteria_verified", "reason"))
deliverable_decision = {"id": ID, "decision": enum("accept", "request_changes"), "reason": TEXT, "decided_by": ID, "decided_at": TIME,
    "head_version": VERSION, "files_verified": {"type": "boolean"}, "criteria_verified": {"type": "boolean"}}
add("DeliverableDecision", deliverable_decision, tuple(deliverable_decision))
deliverable_revision = {"id": ID, "deliverable_id": ID, "seq": VERSION, "head_version": VERSION, "project_version": VERSION, "name": NAME,
    "acceptance_criteria": TEXT, "owner_id": ID, "milestone_id": nullable(ID), "classification": CLASS, "content_hash": HASH,
    "description": TEXT, "reason": NAME, "submitted_by": ID, "submitted_at": TIME, "files": array(ref("BusinessFileReference"), minItems=1, maxItems=20), "decision": nullable(ref("DeliverableDecision"))}
add("DeliverableRevision", deliverable_revision, tuple(deliverable_revision))
add("DeliverableReceipt", {"id": ID, "version": VERSION, "revision_id": nullable(ID)}, ("id", "version", "revision_id"))
for entity, states in (("Risk", ("open", "mitigating", "closed", "realized")),
        ("Issue", ("open", "investigating", "resolved", "closed"))):
    fields = {"title": NAME, "description": TEXT, "owner_id": ID, "severity": enum("low", "medium", "high", "critical"),
        "due_at": TIME}
    if entity == "Risk":
        fields.update({"probability": enum("low", "medium", "high"), "mitigation": TEXT})
    else:
        fields["resolution"] = TEXT
    add(entity + "Create", fields, ("title", "description", "owner_id", "severity"))
    add(entity + "Patch", {**EXPECTED, **fields, "state": enum(*states), "reason": NAME}, ("expected_version",))
    SCHEMAS[entity + "Patch"]["properties"]["due_at"] = nullable(TIME)
    if entity == "Risk":
        SCHEMAS[entity + "Patch"]["properties"]["probability"] = nullable(enum("low", "medium", "high"))
    output_fields = {**RECORD, **fields, "project_id": ID, "creator_id": ID, "state": enum(*states)}
    if entity == "Risk":
        output_fields["issue_id"] = ID
    add(entity, output_fields,
        ("id", "version", "project_id", "title", "state"))
SIGNED_MONEY = {"type": "string", "pattern": r"^-?(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$"}
budget_line = {"category": NAME, "period_start": DATE, "period_end": DATE, "amount": MONEY}
add("ProjectBudgetLine", budget_line, tuple(budget_line))
change_content = {"title": NAME, "category": NAME, "currency": CURRENCY, "scope": {**TEXT,"minLength":1},
    "scope_delta": {**TEXT,"minLength":1}, "schedule_delta": {**TEXT,"minLength":1}, "start_date": nullable(DATE), "due_date": nullable(DATE),
    "budget_lines": nullable(array(ref("ProjectBudgetLine"),maxItems=100)), "file_version_ids": array(ID,maxItems=20,uniqueItems=True)}
add("ChangeContent",change_content,tuple(change_content))
change_basis = {"project_id":ID,"project_version":VERSION,"name":NAME,"scope":TEXT,"start_date":nullable(DATE),"due_date":nullable(DATE),
    "classification":CLASS,"currency":CURRENCY,"budget_id":nullable(ID),"budget_lines":array(ref("ProjectBudgetLine"),maxItems=100),"budget_total":MONEY}
add("ChangeBasis",change_basis,tuple(change_basis))
change_revision = {"id":ID,"change_id":ID,"seq":VERSION,"base_revision_id":nullable(ID),"payload_schema_version":{"const":1},
    "content":ref("ChangeContent"),"basis":ref("ChangeBasis"),"content_hash":HASH,"approval_amount":MONEY,"budget_delta":SIGNED_MONEY,
    "target_budget_id":nullable(ID),"author_id":ID,"reason":NAME,"created_at":TIME,"attachments":array(ref("BusinessFileReference"),maxItems=20)}
add("ChangeRevision",change_revision,tuple(change_revision))
change_implementation = {"id":ID,"change_id":ID,"revision_id":ID,"request_revision_id":ID,"project_id":ID,"before_project_version":VERSION,
    "after_project_version":VERSION,"change_version":VERSION,"actor_id":ID,"mfa_verified_at":TIME,"scope_verified":{"const":True},"budget_verified":{"const":True},"reason":NAME,"created_at":TIME}
add("ChangeImplementation",change_implementation,tuple(change_implementation))
change_head = {**RECORD,"project_id":ID,"creator_id":ID,"classification":CLASS,"state":enum("draft","in_review","approved","rejected","withdrawn","implemented"),
    "lifecycle":enum("draft","requested","effective"),"approval_state":enum("not_submitted","in_review","approved","returned","rejected","withdrawn"),
    "current_revision_id":ID,"approved_revision_id":nullable(ID),"request_id":nullable(ID),"revision":ref("ChangeRevision"),
    "can_edit":{"type":"boolean"},"can_prepare_request":{"type":"boolean"},"can_implement":{"type":"boolean"},"implementation":nullable(ref("ChangeImplementation"))}
add("Change",change_head,tuple(change_head))
change_create = {"content":ref("ChangeContent"),"base_project_version":VERSION,"base_budget_id":nullable(ID),"reason":NAME}
add("ChangeCreate",change_create,tuple(change_create))
add("ChangePatch",{**change_create,**EXPECTED,"base_revision_id":ID},(*change_create,"expected_version","base_revision_id"))
change_submit = {**EXPECTED,"revision_id":ID,"content_hash":HASH,"reason":NAME}
add("ChangeSubmit",change_submit,tuple(change_submit))
change_implement = {**change_submit,"base_project_version":VERSION,"request_revision_id":ID,"scope_verified":{"const":True},"budget_verified":{"const":True}}
add("ChangeImplement",change_implement,tuple(change_implement))
add("ChangeReceipt",{"id":ID,"version":VERSION,"revision_id":ID},("id","version","revision_id"))
closeout_kinds = enum("tasks","milestones","deliverables","risks","issues","changes","budgets","contracts","purchases","claims","costs","payments","subscriptions")
closeout_item = {"kind":closeout_kinds,"id":ID,"version":VERSION,"title":{"type":"string"},"state":{"type":"string"},"blocking":{"type":"boolean"}}
add("ProjectCloseoutItem",closeout_item,tuple(closeout_item))
closeout_group = {"kind":closeout_kinds,"visibility":enum("complete","restricted"),"count":nullable(COUNT),"open_count":nullable(COUNT),"items":array(ref("ProjectCloseoutItem"),maxItems=5000)}
add("ProjectCloseoutGroup",closeout_group,tuple(closeout_group))
closeout = {"project":ref("Project"),"as_of":TIME,"visibility":enum("complete","restricted"),"fingerprint":nullable({"type":"string","pattern":"^[0-9a-f]{32}$"}),"groups":array(ref("ProjectCloseoutGroup"),minItems=13,maxItems=13),"can_close":{"type":"boolean"},"can_archive":{"type":"boolean"}}
add("ProjectCloseout",closeout,tuple(closeout))
project_archive_input = {**EXPECTED,"expected_fingerprint":{"type":"string","pattern":"^[0-9a-f]{32}$"},"summary":{**TEXT,"minLength":1},"lessons_learned":{**TEXT,"minLength":1},"follow_up":{**TEXT,"minLength":1},"business_verified":{"const":True},"financial_verified":{"const":True},"file_version_ids":array(ID,maxItems=20,uniqueItems=True),"reason":NAME}
add("ProjectArchiveInput",project_archive_input,tuple(project_archive_input))
project_archive = {"id":ID,"project_id":ID,"project_version":VERSION,"project_name":NAME,"fingerprint":{"type":"string","pattern":"^[0-9a-f]{32}$"},"summary":TEXT,"lessons_learned":TEXT,"follow_up":TEXT,"business_verified":{"const":True},"financial_verified":{"const":True},"file_version_ids":array(ID,maxItems=20),"attachments":array(ref("BusinessFileReference"),maxItems=20),"groups":array(ref("ProjectCloseoutGroup"),minItems=13,maxItems=13),"actor_id":ID,"actor_name":NAME,"created_at":TIME,"reason":NAME,"current_basis_state":enum("current","changed")}
add("ProjectArchive",project_archive,tuple(project_archive))
add("ProjectArchiveReceipt",{"project":ref("Project"),"archive":ref("ProjectArchive")},("project","archive"))
add("ProjectArchiveList",{"visibility":enum("complete","restricted"),"items":array(ref("ProjectArchive"),maxItems=100),"next_cursor":nullable(CURSOR)},("visibility","items","next_cursor"))
budget = {"id":ID,"project_id":ID,"revision":VERSION,"state":enum("draft","approved","superseded"),"currency":CURRENCY,"creator_id":ID,
    "change_id":ID,"change_revision_id":ID,"predecessor_id":nullable(ID),"lines":array(ref("ProjectBudgetLine"),maxItems=100),"total":MONEY,"created_at":TIME}
add("ProjectBudget",budget,tuple(budget))
# Current implemented project budget against authorized canonical expense shares.
REPORT_MONEY = {"type":"string","maxLength":80,"pattern":r"^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$"}
REPORT_SIGNED_MONEY = {**REPORT_MONEY,"pattern":r"^-?(0|[1-9][0-9]*)(\.[0-9]{1,6})?$"}
comparison_metrics = {"expense_amount":REPORT_MONEY,"reversal_amount":REPORT_MONEY,"net_amount":REPORT_SIGNED_MONEY,"entry_count":{"type":"integer","minimum":0},"unconfirmed_count":{"type":"integer","minimum":0}}
add("BudgetComparisonMetrics",comparison_metrics,tuple(comparison_metrics))
comparison_line = {"index":{"type":"integer","minimum":0,"maximum":99},"category":NAME,"period_start":DATE,"period_end":DATE,"budget_amount":MONEY,"variance":REPORT_SIGNED_MONEY,"over_budget":{"type":"boolean"},**comparison_metrics}
add("BudgetComparisonLine",comparison_line,tuple(comparison_line))
add("BudgetComparisonBasis",{"id":ID,"revision":VERSION,"total":REPORT_MONEY},("id","revision","total"))
comparison = {"project_id":ID,"project_version":VERSION,"currency":CURRENCY,"basis_hash":HASH,"budget":nullable(ref("BudgetComparisonBasis")),"budget_variance":nullable(REPORT_SIGNED_MONEY),"scope":{"const":"authorized_records_only"},"timezone":{"const":"Asia/Hong_Kong"},"as_of":TIME,"lines":array(ref("BudgetComparisonLine"),maxItems=100),"matched":ref("BudgetComparisonMetrics"),"outside":ref("BudgetComparisonMetrics"),"lifetime":ref("BudgetComparisonMetrics")}
add("BudgetComparison",comparison,tuple(comparison))
comparison_entry = {"id":ID,"revision_id":ID,"version":VERSION,"title":NAME,"category":{"type":"string","maxLength":50},"occurred_on":nullable(DATE),"currency":CURRENCY,"entry_kind":enum("expense","reversal"),"state":enum("draft","confirmed","reversed"),"share_amount":MONEY,"signed_amount":REPORT_SIGNED_MONEY,"line_index":nullable({"type":"integer","minimum":0,"maximum":99}),"created_at":TIME}
add("BudgetComparisonEntry",comparison_entry,tuple(comparison_entry))
comparison_entries = {"basis_hash":HASH,"as_of":TIME,"items":array(ref("BudgetComparisonEntry"),maxItems=100),"next_cursor":nullable(CURSOR)}
add("BudgetComparisonEntries",comparison_entries,tuple(comparison_entries))
change_snapshot = {"business_payload_schema_version":{"const":1},"kind":{"const":"change"},"business_id":ID,"business_revision_id":ID,"business_content_hash":HASH,
    "project_id":ID,"classification":CLASS,"content":ref("ChangeContent"),"project_change_basis":ref("ChangeBasis"),"project_change_budget_delta":SIGNED_MONEY,"amount":MONEY,"attachments":array(ref("BusinessFileReference"),maxItems=20)}
add("ChangeRequestPayload",change_snapshot,tuple(change_snapshot))
SCHEMAS["RequestPayload"]["oneOf"].append(ref("ChangeRequestPayload"))



# Native inventory/import admission reuses the existing upload and replica pipeline.
add("EmptyCommand", {})
import_batch_fields = {"source_namespace":ID,"inventory_sha256":HASH,"mapping_sha256":HASH,"plan_sha256":HASH,
    "entry_count":{"type":"integer","minimum":1,"maximum":1000000},"total_bytes":BYTES,"reason":NAME}
add("FileImportBatchInput",import_batch_fields,tuple(import_batch_fields))
add("FileImportBatch",{**import_batch_fields,"id":ID,"created_at":TIME,"owner_id":ID,
    "admitted_count":{"type":"integer","minimum":0},"admitted_bytes":BYTES,"admitted_chain":HASH,
    "approved":{"type":"boolean"},"approval_expires_at":nullable(TIME)},
    (*import_batch_fields,"id","created_at","owner_id","admitted_count","admitted_bytes","admitted_chain","approved","approval_expires_at"))
import_entry_fields={"ordinal":{"type":"integer","minimum":1,"maximum":1000000},
    "source_path":{"type":"string","minLength":1,"maxLength":1000},"source_key":HASH,"source_sha256":HASH,"size_bytes":BYTES,
    "source_modified_at":TIME,"source_mode":{"type":"string","maxLength":32},"source_owner":NAME,"mapped_owner_id":ID,
    "folder_id":ID,"folder_version":VERSION,"project_id":nullable(ID),"classification":CLASS,"name":NAME,"media_type":NAME,
    "file_id":ID,"base_version_id":ID}
import_entry_required=tuple(k for k in import_entry_fields if k not in ("file_id","base_version_id"))
add("FileImportEntryInput",import_entry_fields,import_entry_required)
SCHEMAS["FileImportEntryInput"]["dependentRequired"]={"file_id":["base_version_id"],"base_version_id":["file_id"]}
add("FileImportEntry",{**import_entry_fields,"id":ID,"batch_id":ID,"created_at":TIME,
    "upload":nullable(ref("Upload")),"file_version":nullable(ref("FileVersion")),
    "verification_state":enum("awaiting_upload","created","uploading","finalizing","verifying","ready","committed","failed","aborted","expired","blocked","primary_verified","replicas_verified","content_mismatch","mapping_changed"),
    "mapping_current":{"type":"boolean"},"mapped_owner_can_read":{"type":"boolean"}},
    (*import_entry_required,"id","batch_id","created_at","upload","file_version","verification_state","mapping_current","mapped_owner_can_read"))
add("FileImportApprovalInput",{"plan_sha256":HASH,"reviewed_mappings":{"const":True},"sources_read_only":{"const":True},"reason":NAME},
    ("plan_sha256","reviewed_mappings","sources_read_only","reason"))
add("FileImportRetryInput",{"upload_id":ID,"expected_version":VERSION,"reason":NAME},("upload_id","expected_version","reason"))

def wrapped(name, listing=False):
    key = name + ("ListResponse" if listing else "Response")
    if key not in SCHEMAS:
        data = obj({"items": array(ref(name)), "next_cursor": nullable(CURSOR)}, ("items", "next_cursor")) if listing else ref(name)
        add(key, {"data": data, "meta": ref("Meta")}, ("data", "meta"))
    return ref(key)


MINIAPP = enum("wechat", "douyin")
MINIAPP_APP_ID = {"type": "string", "pattern": "^[A-Za-z0-9_-]{5,128}$"}
MINIAPP_CODE = {"type": "string", "minLength": 1, "maxLength": 512}
add("MiniappProfile", {"platform": MINIAPP, "app_id": MINIAPP_APP_ID}, ("platform", "app_id"))
add("MiniappProfiles", {"items": array(ref("MiniappProfile"), maxItems=2)}, ("items",))
add("MiniappCodeInput", {"platform": MINIAPP, "app_id": MINIAPP_APP_ID, "code": MINIAPP_CODE}, ("platform", "app_id", "code"))
add("MiniappProofInput", {"platform": MINIAPP, "app_id": MINIAPP_APP_ID, "code": MINIAPP_CODE,
    "current_password": {"type": "string", "minLength": 1, "maxLength": 1024}}, ("platform", "app_id", "code", "current_password"))
add("MiniappProof", {"id": ID, "platform": MINIAPP, "app_id": MINIAPP_APP_ID, "expires_at": TIME}, ("id", "platform", "app_id", "expires_at"))
add("MiniappBinding", {"id": ID, "platform": MINIAPP, "app_id": MINIAPP_APP_ID, "version": VERSION, "created_at": TIME,
    "revoked_at": nullable(TIME), "is_current_session_source": {"type": "boolean"}},
    ("id", "platform", "app_id", "version", "created_at", "revoked_at", "is_current_session_source"))
add("MiniappBindInput", {"proof_id": ID, "confirmed": {"type": "boolean", "const": True}, "reason": NAME}, ("proof_id", "confirmed", "reason"))
add("MiniappRevokeInput", {**EXPECTED, "reason": NAME}, ("expected_version", "reason"))

def operation(path, method, name, result, body=None, code=200, listing=False):
    params = [{"name": p, "in": "path", "required": True, "schema": ID} for p in re.findall(r"\{([^}]+)\}", path)]
    if listing or (method == "get" and result in ("Comments", "MentionCandidates")):
        params += [{"name": "cursor", "in": "query", "schema": CURSOR},
            {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 100, "default": 20}}]
    if method != "get":
        params += [{"$ref": "#/components/parameters/IdempotencyKey"}, {"$ref": "#/components/parameters/CsrfToken"}]
    op = {"operationId": name, "tags": [path.strip("/").split("/")[0]], "summary": name,
        "security": [{"bearerAuth": []}, {"cookieAuth": []}], "parameters": params,
        "responses": {str(code): {"description": "Accepted" if code == 202 else "Success",
            "content": {"application/json": {"schema": wrapped(result, listing)}}}}}
    for status in (400, 401, 403, 404, 409, 410, 412, 413, 422, 429, 500, 503):
        op["responses"][str(status)] = {"$ref": "#/components/responses/Failure"}
    if body:
        op["requestBody"] = {"required": True, "content": {"application/json": {"schema": ref(body)}}}
    PATHS.setdefault(path, {})[method] = op


operation("/project-templates", "get", "listProjectTemplates", "ProjectTemplate", listing=True)
PATHS["/project-templates"]["get"]["parameters"] += [{"name": "state", "in": "query", "schema": enum("draft", "published", "archived")}, {"name": "q", "in": "query", "schema": NAME}, {"name": "key", "in": "query", "schema": template_key}]
operation("/project-templates", "post", "createProjectTemplate", "ProjectTemplate", "ProjectTemplateCreate", 201)
operation("/project-templates/{id}", "get", "getProjectTemplate", "ProjectTemplate")
operation("/project-templates/{id}", "patch", "updateProjectTemplate", "ProjectTemplate", "ProjectTemplatePatch")
operation("/project-templates/{id}/history", "get", "listProjectTemplateHistory", "ProjectTemplateEvent", listing=True)
for action in ("publish", "retire", "revise"):
    operation("/project-templates/{id}/" + action, "post", action + "ProjectTemplate", "ProjectTemplate", "ProjectTemplateAction", 201 if action == "revise" else 200)

operation("/me", "get", "getMe", "Me")
operation("/me/telegram","get","getOwnTelegramStatus","TelegramStatus")
operation("/me/telegram/challenges","post","createTelegramBindingChallenge","TelegramChallenge","TelegramChallengeCreate",201)
operation("/me/telegram/challenges/{id}/confirm","post","confirmTelegramBinding","TelegramBinding","TelegramChallengeConfirm",201)
operation("/me/telegram/challenges/{id}/cancel","post","cancelTelegramBindingChallenge","TelegramChallenge","TelegramReasonInput")
operation("/me/telegram/bindings/{id}/revoke","post","revokeTelegramBinding","TelegramBinding","TelegramReasonInput")
operation("/me/telegram/requests","get","listOwnTelegramRequests","TelegramRequest",listing=True)
operation("/me/telegram/requests/{id}","get","getOwnTelegramRequest","TelegramRequest")
operation("/me/telegram/requests/{id}/deliveries","get","getOwnTelegramDeliveryDetails","TelegramDeliveryDetails")
operation("/me/telegram/requests/{id}/deliveries/{delivery_id}/reconcile","post","reconcileOwnTelegramDelivery","TelegramDelivery","TelegramDeliveryReconcile")
operation("/telegram/channels","get","listTelegramChannels","TelegramChannel",listing=True)
operation("/telegram/channels","post","createTelegramChannel","TelegramChannel","TelegramChannelCreate",201)
operation("/telegram/channels/{id}","patch","updateTelegramChannel","TelegramChannel","TelegramChannelPatch")
operation("/telegram/channels/{id}/history","get","listTelegramChannelHistory","TelegramChannelRevision",listing=True)
PATHS["/integrations/telegram/{bot_id}/webhook"]={"post":{
    "operationId":"receiveTelegramUpdate","tags":["telegram"],"summary":"Persist an authenticated Telegram update before acknowledging it",
    "security":[],"parameters":[{"name":"bot_id","in":"path","required":True,"schema":TG_NUMBER},
        {"name":"X-Telegram-Bot-Api-Secret-Token","in":"header","required":True,"schema":{"type":"string","minLength":32,"maxLength":256}}],
    "requestBody":{"required":True,"content":{"application/json":{"schema":{"type":"object","required":["update_id"],"additionalProperties":True,"properties":{"update_id":{"type":"integer","minimum":0,"maximum":9007199254740991}}}}}},
    "responses":{"200":{"description":"Durably received; does not imply a model run or delivery","content":{"application/json":{"schema":ref("TelegramWebhookAck")}}},
        **{str(status):{"$ref":"#/components/responses/Failure"} for status in (400,404,409,413,415,422,500)}}}}
operation("/auth/miniapps", "get", "getMiniappProfiles", "MiniappProfiles")
operation("/auth/miniapps/login", "post", "loginMiniapp", "LoginResult", "MiniappCodeInput")
operation("/me/miniapp-binding-proofs", "post", "createMiniappProof", "MiniappProof", "MiniappProofInput", 201)
operation("/me/miniapp-bindings", "get", "listOwnMiniappBindings", "MiniappBinding", listing=True)
operation("/me/miniapp-bindings", "post", "bindOwnMiniapp", "MiniappBinding", "MiniappBindInput", 201)
operation("/me/miniapp-bindings/{id}/revoke", "post", "revokeOwnMiniapp", "MiniappBinding", "MiniappRevokeInput")
for miniapp_route in ("/auth/miniapps", "/auth/miniapps/login", "/me/miniapp-binding-proofs"):
    for miniapp_operation in PATHS[miniapp_route].values():
        miniapp_operation["parameters"] = [parameter for parameter in miniapp_operation["parameters"] if parameter.get("$ref") != "#/components/parameters/IdempotencyKey"]
        if miniapp_route.startswith("/auth/"):
            miniapp_operation["security"] = []
operation("/me/sessions", "get", "listDeviceSessions", "DeviceSession", listing=True)
operation("/me/sessions/{id}/revoke", "post", "revokeDeviceSession", "SessionRevocation", "SessionRevokeInput")
operation("/me/password", "post", "changeOwnPassword", "PasswordChanged", "PasswordChangeInput")
operation("/users/{id}/password-resets", "post", "issuePasswordReset", "PasswordResetReceipt", "PasswordResetInput", 201)
for route, name, body, result in (
        ("login", "login", "LoginInput", "LoginResult"),
        ("mfa/enroll", "enrollMFA", "MFAEnrollInput", "LoginResult"),
        ("mfa/verify", "verifyMFA", "MFAInput", "LoginResult"),
        ("mfa/recover", "recoverMFA", "MFARecoveryInput", "LoginResult"),
        ("activate", "activateAccount", "ActivateInput", "ActivationResult"),
        ("password/reset", "completePasswordReset", "CompletePasswordResetInput", "PasswordChanged"),
        ("refresh", "refreshSession", "RefreshInput", "LoginResult"),
        ("logout", "logout", "EmptyInput", "LogoutResult")):
    operation("/auth/"+route, "post", name, result, body)
    authentication_operation = PATHS["/auth/"+route]["post"]
    authentication_operation["parameters"] = [parameter for parameter in authentication_operation["parameters"]
        if parameter.get("$ref") != "#/components/parameters/IdempotencyKey"]
    if route != "logout":
        authentication_operation["security"] = []
operation("/auth/mfa/step-up", "post", "stepUpMFA", "MFAStepUpResult", "MFAStepUpInput")
PATHS["/auth/mfa/step-up"]["post"]["parameters"] = [parameter for parameter in PATHS["/auth/mfa/step-up"]["post"]["parameters"]
    if parameter.get("$ref") != "#/components/parameters/IdempotencyKey"]
operation("/capabilities", "get", "getCapabilities", "Capability")
operation("/dashboard", "get", "getDashboard", "Dashboard")
workbench_item = {"id": ID, "version": VERSION, "revision_id": ID, "related_id": nullable(ID), "title": NAME,
    "state": enum("active", "overdue", "due_soon", "awaiting_confirmation", "open", "investigating", "monitoring", "file_unavailable", "replica_attention", "replica_pending"), "due_on": nullable(DATE), "updated_at": TIME}
add("WorkbenchItem", workbench_item, tuple(workbench_item))
workbench_group = {"kind": enum("approvals", "contracts", "subscription_periods", "incidents", "file_attention"), "count": COUNT,
    "items": array(ref("WorkbenchItem"), maxItems=5), "has_more": {"type": "boolean"}}
add("WorkbenchGroup", workbench_group, tuple(workbench_group))
workbench_summary = {"as_of": TIME, "scope": enum("my_readable_work"), "business_date": DATE,
    "contract_timezone": enum("Asia/Hong_Kong"), "contract_through": DATE,
    "groups": array(ref("WorkbenchGroup"), minItems=5, maxItems=5)}
add("WorkbenchSummary", workbench_summary, tuple(workbench_summary))
operation("/dashboard/business", "get", "getWorkbenchSummary", "WorkbenchSummary")

overview_kinds = enum("tasks", "milestones", "deliverables", "risks", "issues", "changes", "budgets", "files", "posts", "contracts", "purchases", "claims", "costs", "payments", "subscriptions", "services")
overview_count = {"type": "integer", "minimum": 0, "maximum": 9007199254740991}
overview_item = {"id": ID, "version": VERSION, "revision_id": nullable(ID), "title": TEXT, "state": NAME, "updated_at": TIME}
add("ProjectOverviewItem", overview_item, tuple(overview_item))
add("ProjectOverviewState", {"state": NAME, "count": overview_count}, ("state", "count"))
overview_group = {"kind": overview_kinds, "available": {"type": "boolean"}, "count": nullable(overview_count), "states": array(ref("ProjectOverviewState")), "items": array(ref("ProjectOverviewItem"), maxItems=5), "has_more": {"type": "boolean"}}
add("ProjectOverviewGroup", overview_group, tuple(overview_group))
overview_incurred = {"available": {"type": "boolean"}, "totals": array(ref("CostTotal")), "unconfirmed_count": nullable(overview_count)}
add("ProjectIncurred", overview_incurred, tuple(overview_incurred))
overview_budget = {"budget_id": ID, "revision": VERSION, "currency": CURRENCY, "amount": report_decimal}
add("ProjectOverviewBudget", overview_budget, tuple(overview_budget))
overview = {"project_id": ID, "project_version": VERSION, "as_of": TIME, "scope": {"const": "readable_records"}, "groups": array(ref("ProjectOverviewGroup"), minItems=16, maxItems=16), "incurred": ref("ProjectIncurred"), "budgets": array(ref("ProjectOverviewBudget"))}
add("ProjectOverview", overview, tuple(overview))
operation("/projects/{id}/overview", "get", "getProjectOverview", "ProjectOverview")
operation("/projects/{id}/members", "get", "getProjectMembers", "ProjectMembers")
operation("/projects/{id}/members", "post", "changeProjectMember", "Project", "ProjectMemberChange")
operation("/projects/{id}/member-candidates", "get", "listProjectMemberCandidates", "ProjectMembers")
operation("/milestones/{id}", "get", "getMilestone", "Milestone")
operation("/milestones/{id}", "patch", "updateMilestone", "Milestone", "MilestonePatch")
operation("/milestones/{id}/transition", "post", "transitionMilestone", "Milestone", "Transition")
operation("/tasks/{id}/dependencies", "get", "getTaskDependencies", "TaskDependencies")
operation("/users", "get", "listUsers", "User", listing=True)
operation("/users", "post", "inviteUser", "InvitationReceipt", "UserInvite", 201)
operation("/users/{id}/invitation", "get", "getUserInvitation", "InvitationReceipt")
operation("/users/{id}/disable", "post", "disableUser", "User", "Transition")
operation("/roles", "get", "listRoles", "Role", listing=True)
operation("/role-bindings", "get", "listRoleBindings", "RoleBinding", listing=True)
operation("/role-bindings", "post", "createRoleBinding", "RoleBinding", "RoleBindingCreate", 201)
operation("/role-bindings/{id}/revoke", "post", "revokeRoleBinding", "RevocationResult", "RoleBindingRevoke")
operation("/classification-policies", "get", "listClassificationPolicies", "ClassificationPolicy", listing=True)
operation("/classification-policies", "post", "publishClassificationPolicy", "ClassificationPolicy", "ClassificationPolicyInput", 201)
operation("/classification-policies/{level}/history", "get", "getClassificationPolicyHistory", "ClassificationPolicy", listing=True)
PATHS["/classification-policies/{level}/history"]["get"]["parameters"][0]["schema"] = POLICY_LEVEL
for route, entity in (("projects", "Project"), ("posts", "Post"), ("subscriptions", "Subscription")):
    operation("/" + route, "get", "list" + entity + "s", entity, listing=True)
    operation("/" + route, "post", "create" + entity, entity, entity + "Create", 201)
    operation("/" + route + "/{id}", "get", "get" + entity, entity)
    operation("/" + route + "/{id}", "patch", "update" + entity, entity, entity + "Patch")
operation("/projects/{id}/transition", "post", "transitionProject", "Project", "Transition")
operation("/projects/{id}/tasks", "get", "listTasks", "Task", listing=True)
operation("/projects/{id}/tasks", "post", "createTask", "Task", "TaskCreate", 201)
operation("/tasks/{id}", "get", "getTask", "Task")
operation("/tasks/{id}", "patch", "updateTask", "Task", "TaskPatch")
operation("/tasks/{id}/transition", "post", "transitionTask", "Task", "Transition")
operation("/tasks/{id}/dependencies", "post", "changeTaskDependency", "Task", "TaskDependency")
operation("/posts/{id}/publish", "post", "publishPost", "Post", "PostTransition")
operation("/posts/{id}/revisions", "get", "listPostRevisions", "PostRevision", listing=True)
operation("/posts/{id}/restore", "post", "restorePostRevision", "Post", "PostRestore")
for parent in ("posts", "tasks"):
    operation("/"+parent+"/{id}/comments", "get", "list"+parent.title()+"Comments", "Comments")
    operation("/"+parent+"/{id}/comments", "post", "create"+parent.title()+"Comment", "Comment", "PostCommentCreate" if parent == "posts" else "CommentCreate", 201)
operation("/comments/{id}", "get", "getComment", "Comment")
operation("/tasks/{id}/mention-candidates", "get", "listTaskMentionCandidates", "MentionCandidates")
PATHS["/tasks/{id}/mention-candidates"]["get"]["parameters"] += [{"name": "q", "in": "query", "schema": {"type": "string", "maxLength": 200}}]
operation("/comments/{id}", "patch", "editComment", "Comment", "CommentPatch", 201)
operation("/comments/{id}/history", "get", "listCommentHistory", "Comments")
operation("/subscriptions/{id}/transition", "post", "transitionSubscription", "Subscription", "SubscriptionTransition")
operation("/subscriptions/{id}/prices", "post", "addSubscriptionPrice", "Subscription", "SubscriptionPriceChange")
operation("/subscriptions/{id}/periods", "get", "listSubscriptionPeriods", "SubscriptionPeriod", listing=True)
operation("/subscriptions/{id}/periods/generate", "post", "generateSubscriptionPeriods", "SubscriptionGeneration", "SubscriptionGenerate")
operation("/subscriptions/{id}/periods/unexpected", "post", "recordUnexpectedSubscriptionPeriod", "SubscriptionPeriod", "SubscriptionUnexpectedPeriod", 201)
operation("/subscriptions/{id}/revisions", "get", "listSubscriptionRevisions", "SubscriptionRevision", listing=True)
operation("/subscriptions/{id}/revisions/{revision}", "get", "getSubscriptionRevision", "SubscriptionRevision")
operation("/subscriptions/{id}/events", "get", "listSubscriptionEvents", "SubscriptionEvent", listing=True)
operation("/subscriptions/{id}/reminders", "get", "listSubscriptionReminders", "SubscriptionReminder", listing=True)
operation("/subscriptions/owners", "get", "listSubscriptionOwners", "SubscriptionOwner", listing=True)
PATHS["/subscriptions/owners"]["get"]["parameters"] += [{"name": "project_id", "in": "query", "schema": ID}, {"name": "classification", "in": "query", "required": True, "schema": CLASS}]
operation("/subscription-periods", "get", "listAllSubscriptionPeriods", "SubscriptionPeriod", listing=True)
operation("/subscription-periods/{id}", "get", "getSubscriptionPeriod", "SubscriptionPeriod")
operation("/subscription-periods/{id}/reforecast", "post", "reforecastSubscriptionPeriod", "SubscriptionPeriod", "SubscriptionPeriodReforecast")
operation("/subscription-periods/{id}/costs", "post", "linkSubscriptionCost", "SubscriptionPeriod", "SubscriptionPeriodLink")
operation("/subscription-periods/{id}/review", "post", "reviewSubscriptionPeriod", "SubscriptionPeriod", "SubscriptionPeriodDecision")
operation("/subscription-periods/{id}/revisions", "get", "listSubscriptionPeriodRevisions", "SubscriptionPeriodRevision", listing=True)
operation("/subscription-periods/{id}/events", "get", "listSubscriptionPeriodEvents", "SubscriptionPeriodEvent", listing=True)
operation("/reports/forecast", "get", "readForecastReport", "ForecastReport")
subscription_filters = [{"name": key, "in": "query", "schema": val} for key,val in {"q": {"type": "string", "maxLength": 200}, "currency": CURRENCY, "category": {"type": "string", "maxLength": 50}, "from": DATE, "to": DATE, "project_id": ID, "company_only": {"type": "boolean"}, "supplier_id": ID, "owner_id": ID}.items()]
PATHS["/subscriptions"]["get"]["parameters"] += [f for f in subscription_filters if f["name"] not in ("from", "to")] + [{"name": "state", "in": "query", "schema": enum("draft", "active", "paused", "cancelled", "archived")}]
for path in ("/subscriptions/{id}/periods", "/subscription-periods"):
    PATHS[path]["get"]["parameters"] += subscription_filters + [{"name": "state", "in": "query", "schema": enum("forecast", "awaiting_confirmation", "confirmed", "waived", "void")}, {"name": "forecast_only", "in": "query", "schema": {"type": "boolean"}}]
PATHS["/reports/forecast"]["get"]["parameters"] += subscription_filters
operation("/folders", "get", "listFolders", "Folder", listing=True)
operation("/folders", "post", "createFolder", "Folder", "FolderCreate", 201)
operation("/folders/{id}", "get", "getFolder", "Folder")
operation("/folders/{id}/move", "post", "moveFolder", "Folder", "Move")
operation("/files", "get", "listFiles", "File", listing=True)
for storage_path, container in (("/folders", "parent_id"), ("/files", "folder_id")):
    PATHS[storage_path]["get"]["parameters"] += [
        {"name": container, "in": "query", "schema": ID}, {"name": "project_id", "in": "query", "schema": ID},
        {"name": "q", "in": "query", "schema": NAME}]
PATHS["/files"]["get"]["parameters"].append({"name": "state", "in": "query", "schema": enum("active", "trashed")})
PATHS["/files"]["get"]["parameters"].append({"name": "company_only", "in": "query", "schema": {"type": "boolean"}})
operation("/files/{id}", "get", "getFile", "File")
operation("/files/{id}/versions", "get", "listFileVersions", "FileVersion", listing=True)
operation("/files/{id}/versions/{version}", "get", "getFileVersion", "FileVersion")
operation("/file-versions/{id}", "get", "getFileVersionByID", "FileVersion")
operation("/files/{id}/history-restores", "post", "createHistoryRestore", "HistoryRestore", "HistoryRestoreCreate", code=202)
operation("/storage/retention-policies", "get", "listRetentionPolicies", "RetentionPolicy", listing=True)
operation("/storage/retention-policies", "post", "publishRetentionPolicy", "RetentionPolicy", "RetentionPolicyInput", 201)
operation("/files/{id}/retention", "get", "getFileRetention", "RetentionOverview")
operation("/files/{id}/retention-holds", "get", "listRetentionHolds", "RetentionHold", listing=True)
operation("/files/{id}/retention-holds", "post", "createRetentionHold", "RetentionHold", "RetentionHoldCreate", 201)
operation("/retention-holds/{id}/release", "post", "releaseRetentionHold", "RetentionHold", "RetentionHoldRelease")
operation("/file-versions/{id}/retention-assessments", "post", "assessFileRetention", "RetentionAssessment", "RetentionAssessmentInput", 201)
operation("/retention-assessments/{id}", "get", "getRetentionAssessment", "RetentionAssessment")
operation("/folders/{id}/upload-cleanup-candidates","get","listMultipartCleanupCandidates","CleanupCandidate",listing=True)
operation("/folders/{id}/upload-cleanup-plans","get","listMultipartCleanupPlans","CleanupPlan",listing=True)
operation("/folders/{id}/upload-cleanup-plans","post","createMultipartCleanupPlan","CleanupPlan","CleanupCreate",201)
operation("/upload-cleanup-plans/{id}","get","readMultipartCleanupPlan","CleanupPlan")
operation("/upload-cleanup-plans/{id}/approve","post","approveMultipartCleanupPlan","CleanupPlan","CleanupDecision",202)
operation("/upload-cleanup-plans/{id}/cancel","post","cancelMultipartCleanupPlan","CleanupPlan","CleanupDecision")
operation("/upload-cleanup-items/{id}/retry","post","retryMultipartCleanupItem","CleanupPlan","UploadFinalize",202)
operation("/upload-cleanup-items/{id}/receipts","get","listMultipartCleanupReceipts","CleanupReceipt",listing=True)
operation("/files/{id}/history-restores", "get", "listHistoryRestores", "HistoryRestore", listing=True)
operation("/history-restores/{id}", "get", "getHistoryRestore", "HistoryRestore")
operation("/history-restores/{id}/commit", "post", "commitHistoryRestore", "FileCommitResult", "HistoryRestoreCommit", code=201)
operation("/history-restores/{id}/cancel", "post", "cancelHistoryRestore", "HistoryRestore", "HistoryRestoreCancel")
operation("/files/{id}/move", "post", "moveFile", "File", "Move")
operation("/files/{id}/trash", "post", "trashFile", "File", "Transition")
operation("/files/{id}/restore", "post", "restoreFile", "File", "RestoreFile")
operation("/file-versions/{id}/download", "post", "createDownloadGrant", "DownloadResult", "DownloadRequest")
PATHS["/downloads/{id}/content"] = {"get": {
    "operationId": "readAuthenticatedDownload", "tags": ["downloads"],
    "summary": "Read one authenticated bounded byte range from a fixed verified version",
    "security": [{"bearerAuth": []}, {"cookieAuth": []}],
    "parameters": [{"name": "id", "in": "path", "required": True, "schema": ID},
        {"name": "Range", "in": "header", "schema": {"type": "string", "maxLength": 100},
         "description": "Single closed byte range, at most 32 MiB. Required when the object is larger than 32 MiB."},
        {"name": "If-Range", "in": "header", "schema": NAME,
         "description": "Optional exact strong ETag returned by a prior response; mismatch is rejected."}],
    "responses": {
        "200": {"description": "Complete object of at most 32 MiB", "content": {"application/octet-stream": {"schema": {"type": "string", "format": "binary"}}}},
        "206": {"description": "One exact byte range", "content": {"application/octet-stream": {"schema": {"type": "string", "format": "binary"}}}},
        **{str(code): {"$ref": "#/components/responses/Failure"} for code in (400, 401, 403, 404, 409, 410, 412, 416, 429, 500, 502, 503)}}}}
operation("/downloads/{id}/chunks", "get", "readAuthenticatedDownloadChunk", "DownloadChunk")
PATHS["/downloads/{id}/chunks"]["get"]["summary"] = "Read at most 1 MiB as authenticated fixed-version base64 JSON; no cloud URL or redirect"
PATHS["/downloads/{id}/chunks"]["get"]["parameters"] += [
    {"name": "offset", "in": "query", "required": True, "schema": BYTES},
    {"name": "length", "in": "query", "required": True, "schema": BYTES,
     "description": "0..1048576 decoded bytes. Zero is allowed only for an empty file at offset zero."},
    {"name": "If-Range", "in": "header", "required": True,
     "schema": {"type": "string", "pattern": '^"[a-f0-9]{64}"$'},
     "description": "Exact quoted SHA-256 of the fixed version. Mismatch is 412; omission is 428."}]
for status in (416, 428, 502):
    PATHS["/downloads/{id}/chunks"]["get"]["responses"][str(status)] = {"$ref": "#/components/responses/Failure"}
operation("/storage/upload-capabilities", "get", "getUploadCapabilities", "UploadCapabilities")

operation("/file-imports","post","createFileImport","FileImportBatch","FileImportBatchInput",201)
operation("/file-imports/{id}","get","getFileImport","FileImportBatch")
operation("/file-imports/{id}/entries","post","addFileImportEntry","FileImportEntry","FileImportEntryInput",201)
operation("/file-imports/{id}/entries","get","listFileImportEntries","FileImportEntry",listing=True)
operation("/file-imports/{id}/approve","post","approveFileImport","FileImportBatch","FileImportApprovalInput")
operation("/file-import-entries/{id}","get","getFileImportEntry","FileImportEntry")
operation("/file-import-entries/{id}/uploads","post","startFileImportUpload","FileImportEntry","EmptyCommand",201)
operation("/file-import-entries/{id}/retry","post","retryFileImportUpload","FileImportEntry","FileImportRetryInput",201)
for p in ("/file-imports/{id}/entries","/file-import-entries/{id}/uploads"):
    PATHS[p]["post"]["responses"]["200"]=PATHS[p]["post"]["responses"]["201"]

operation("/uploads", "get", "listUploads", "Upload", listing=True)
PATHS["/uploads"]["get"]["parameters"].append({"name": "state", "in": "query", "schema": enum("active", "all")})
operation("/uploads", "post", "createUpload", "Upload", "UploadCreate", 201)
operation("/file-replicas/{id}/retry", "post", "retryFileReplica", "FileReplica", "ReplicaRetry")
operation("/uploads/{id}", "get", "getUpload", "Upload")
operation("/uploads/{id}/scan-report", "get", "getUploadScanReport", "UploadScanReport")
operation("/storage/scan-waiver-policies", "get", "listScanWaiverPolicies", "ScanWaiverPolicy", listing=True)
operation("/storage/scan-waiver-policies", "post", "publishScanWaiverPolicy", "ScanWaiverPolicy", "ScanWaiverPolicyInput", 201)
operation("/uploads/{id}/waivers", "post", "requestScanWaiver", "ScanWaiver", "ScanWaiverCreate", 201)
operation("/scan-waivers", "get", "listScanWaivers", "ScanWaiver", listing=True)
PATHS["/scan-waivers"]["get"]["parameters"].append({"name": "state", "in": "query", "schema": enum("pending", "approved", "rejected", "cancelled")})
operation("/scan-waivers/{id}", "get", "getScanWaiver", "ScanWaiver")
operation("/scan-waivers/{id}/scan-report", "get", "getWaiverScanReport", "UploadScanReport")
operation("/scan-waivers/{id}/decisions", "post", "decideScanWaiver", "ScanWaiver", "ScanWaiverDecision")
operation("/scan-waivers/{id}/cancel", "post", "cancelScanWaiver", "ScanWaiver", "Transition")
for leaf, name, result, body, code in (
    ("sign-parts", "signUploadParts", "SignedParts", "SignParts", 200),
    ("ack-parts", "ackUploadParts", "Upload", "AckParts", 200),
    ("resume", "resumeUpload", "Upload", "ResumeUpload", 200),
    ("rescan", "rescanUpload", "Upload", "UploadFinalize", 202),
    ("finalize", "finalizeUpload", "Upload", "UploadFinalize", 202),
    ("commit", "commitUpload", "FileCommitResult", "UploadCommit", 201),
    ("abort", "abortUpload", "Upload", "Transition", 200)):
    operation("/uploads/{id}/" + leaf, "post", name, result, body, code)
operation("/search", "get", "search", "SearchHit", listing=True)
PATHS["/search"]["get"]["parameters"] += [{"name": "q", "in": "query", "required": True, "schema": NAME},
    {"name": "project_id", "in": "query", "schema": ID},
    {"name":"type","in":"query","schema":enum("project","task","post","file","folder")},
    {"name":"company_only","in":"query","schema":{"type":"boolean"}}]
operation("/files/{id}/versions/{version}/text","get","getFileText","FileText")
PATHS["/files/{id}/versions/{version}/text"]["get"]["parameters"].extend([
    {"name":"offset","in":"query","schema":{"type":"integer","minimum":0,"default":0}},
    {"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":8000,"default":4000}}])
operation("/files/{id}/text-comparison","get","compareFileText","FileTextComparison")
PATHS["/files/{id}/text-comparison"]["get"]["parameters"].extend([
    {"name":"base_version_id","in":"query","required":True,"schema":ID},
    {"name":"target_version_id","in":"query","required":True,"schema":ID},
    {"name":"view","in":"query","schema":enum("changes","all")},
    {"name":"offset","in":"query","schema":{"type":"integer","minimum":0,"maximum":80000,"default":0}},
    {"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":100,"default":60}},
    {"name":"comparison_sha256","in":"query","schema":HASH}])
operation("/parties", "get", "listParties", "Party", listing=True)
PATHS["/parties"]["get"]["parameters"].extend([
    {"name": "q", "in": "query", "schema": {"type": "string", "maxLength": 200}},
    {"name": "state", "in": "query", "schema": enum("active", "archived")},
    {"name": "relationship", "in": "query", "schema": enum("supplier", "customer", "other")},
    {"name": "project_id", "in": "query", "schema": ID}])
operation("/parties", "post", "createParty", "Party", "PartyCreate", 201)
PATHS["/parties"]["get"]["parameters"].append({"name": "company_only", "in": "query", "schema": {"type": "boolean"}})
operation("/parties/{id}", "get", "getParty", "Party")
operation("/parties/{id}", "patch", "reviseParty", "Party", "PartyPatch")
operation("/parties/{id}/state", "post", "changePartyState", "Party", "PartyStateInput")
operation("/parties/{id}/revisions", "get", "listPartyRevisions", "PartyRevision", listing=True)
operation("/parties/{id}/revisions/{revision}", "get", "getPartyRevision", "PartyRevision")
operation("/workflow-definitions", "get", "listWorkflowDefinitions", "WorkflowDefinition", listing=True)
PATHS["/workflow-definitions"]["get"]["parameters"].append({"name": "kind", "in": "query", "schema": request_kinds})
operation("/workflow-definitions", "post", "publishWorkflowDefinition", "WorkflowDefinition", "WorkflowDefinitionInput", 201)
operation("/workflow-definitions/{id}", "get", "getWorkflowDefinition", "WorkflowDefinition")
operation("/workflow-definitions/{id}/history", "get", "listWorkflowDefinitionHistory", "WorkflowDefinition", listing=True)
operation("/requests", "get", "listRequests", "Request", listing=True)
operation("/requests", "post", "createRequest", "Request", "RequestCreate", 201)
operation("/requests/{id}", "get", "getRequest", "Request")
PATHS["/requests"]["get"]["parameters"] += [{"name": "q", "in": "query", "schema": {"type": "string", "maxLength": 200}}, {"name": "state", "in": "query", "schema": request_states}, {"name": "kind", "in": "query", "schema": request_kinds}]
operation("/requests/{id}/revisions", "post", "reviseRequestDraft", "Request", "RequestRevise")
operation("/requests/{id}/revisions", "get", "listRequestRevisions", "RequestRevision", listing=True)
operation("/requests/{id}/revisions/{revision}", "get", "getRequestRevision", "RequestRevision")
operation("/requests/{id}/submission-preview", "get", "previewSubmission", "SubmissionPreview")
operation("/requests/{id}/submit", "post", "submitRequest", "Request", "RequestSubmit")
operation("/requests/{id}/withdraw", "post", "withdrawRequest", "Request", "RequestWithdraw")
operation("/approval-tasks", "get", "listApprovalTasks", "ApprovalTask", listing=True)
operation("/approval-steps/{id}/decisions", "post", "decideApproval", "Request", "ApprovalDecision")
for route, entity in (("costs", "Cost"), ("payments", "Payment")):
    operation("/" + route, "get", "list" + entity + "s", entity, listing=True)
    operation("/" + route, "post", "create" + entity, entity, entity + "Create", 201)
    operation("/" + route + "/{id}", "get", "get" + entity, entity)
    operation("/" + route + "/{id}/reverse", "post", "reverse" + entity, entity, "CostReverse" if route == "costs" else "PaymentReverse")
operation("/costs/{id}", "patch", "reviseCostDraft", "Cost", "CostPatch")
operation("/costs/{id}/confirm", "post", "confirmCost", "Cost", "CostConfirm")
operation("/costs/{id}/links", "post", "linkCost", "Cost", "CostLink")
operation("/costs/{id}/revisions", "get", "listCostRevisions", "CostRevision", listing=True)
operation("/costs/{id}/revisions/{revision}", "get", "getCostRevision", "CostRevision")
operation("/costs/{id}/events", "get", "listCostEvents", "CostEvent", listing=True)
operation("/reports/expenses", "get", "readIncurredCosts", "CostReport")
cost_filters = [{"name": key, "in": "query", "schema": value} for key,value in {
    "q": {"type": "string", "maxLength": 200}, "state": enum("draft", "confirmed", "reversed", "incurred"), "currency": CURRENCY,
    "category": {"type": "string", "maxLength": 50}, "from": DATE, "to": DATE, "project_id": ID, "company_only": {"type": "boolean"}}.items()]
PATHS["/costs"]["get"]["parameters"] += cost_filters
PATHS["/reports/expenses"]["get"]["parameters"] += [v for v in cost_filters if v["name"] not in ("q", "state")] + [{"name": "kind", "in": "query", "schema": enum("incurred")}]
operation("/payments/{id}/record", "post", "recordPayment", "Payment", "PaymentRecord")
operation("/payments/{id}/allocations", "post", "allocatePayment", "Payment", "PaymentAllocate")
operation("/payments/{id}", "patch", "revisePaymentDraft", "Payment", "PaymentPatch")
operation("/payments/{id}/allocations", "get", "listPaymentAllocations", "PaymentAllocation", listing=True)
operation("/payments/{id}/allocations/{allocation}/release", "post", "releasePaymentAllocation", "Payment", "PaymentRelease")
operation("/payments/{id}/refund-allocations", "post", "allocateRefund", "Payment", "PaymentRefundAllocate")
operation("/payments/{id}/revisions", "get", "listPaymentRevisions", "PaymentRevision", listing=True)
operation("/payments/{id}/revisions/{revision}", "get", "getPaymentRevision", "PaymentRevision")
operation("/payments/{id}/events", "get", "listPaymentEvents", "PaymentEvent", listing=True)
operation("/payments/{id}/fees", "get", "listPaymentFees", "PaymentFeeLedger")
PATHS["/payments/{id}/fees"]["get"]["parameters"].extend([
    {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 100, "default": 20}},
    {"name": "cursor", "in": "query", "schema": {"type": "string"}}])
operation("/payments/{id}/fees", "post", "linkPaymentFee", "PaymentFeeLink", "PaymentFeeCreate", code=201)
operation("/payments/{id}/fees/basis", "get", "readPaymentFeeBasis", "PaymentFeePreview")
PATHS["/payments/{id}/fees/basis"]["get"]["parameters"].extend([
    {"name": name, "in": "query", "required": True, "schema": ID} for name in ("fee_payment_id", "allocation_id")])
operation("/payments/{id}/fees/{fee}/revoke", "post", "revokePaymentFee", "PaymentFeeLink", "PaymentFeeRevoke")
operation("/claims/{id}/settlement", "get", "readClaimSettlement", "ClaimSettlement")
operation("/reports/cash", "get", "readCashReport", "CashReport")
payment_filters = [v for v in cost_filters if v["name"] != "state"] + [{"name": "state", "in": "query", "schema": enum("draft", "recorded", "reversed", "cash")}, {"name": "direction", "in": "query", "schema": enum("outgoing", "incoming", "refund", "transfer")}]
PATHS["/payments"]["get"]["parameters"] += payment_filters
PATHS["/reports/cash"]["get"]["parameters"] += [v for v in payment_filters if v["name"] not in ("q", "state", "direction")]

operation("/conversations", "get", "listConversations", "Conversation", listing=True)
PATHS["/conversations"]["get"]["parameters"].extend([
    {"name": "q", "in": "query", "schema": {"type": "string", "maxLength": 200}},
    {"name": "type", "in": "query", "schema": enum("private", "project", "telegram")},
    {"name": "state", "in": "query", "schema": enum("active", "archived")},
    {"name": "project_id", "in": "query", "schema": ID}])
operation("/conversations", "post", "createConversation", "Conversation", "ConversationCreate", 201)
operation("/conversations/{id}", "get", "getConversation", "Conversation")
operation("/conversations/{id}", "patch", "updateConversation", "Conversation", "ConversationPatch")
operation("/conversations/{id}/members", "post", "changeConversationMember", "ConversationAccessResult", "ConversationMemberChange")
operation("/conversations/{id}/member-candidates", "get", "conversationMemberCandidates", "ConversationCandidates")
context_path = "/conversations/{id}/context-sources/{resourceType}/{resourceID}"
operation(context_path, "get", "readConversationContextSource", "ConversationContextView")
context_operation = PATHS[context_path]["get"]
for parameter in context_operation["parameters"]:
    if parameter["name"] == "resourceType":
        parameter["schema"] = enum("project", "task", "post", "file", "service", "agent_finance_report")
context_operation["parameters"].extend([
    {"name": "context_security_version", "in": "query", "required": True, "schema": VERSION},
    {"name": "version_id", "in": "query", "schema": ID},
    {"name": "offset", "in": "query", "schema": {"type": "integer", "minimum": 0, "maximum": 1048576, "default": 0}},
    {"name": "max_chars", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 10000, "default": 4000}}])
operation("/conversations/{id}/messages", "get", "listMessages", "Message", listing=True)
operation("/notifications", "get", "listNotifications", "Notification", listing=True)
for parameter in PATHS["/notifications"]["get"]["parameters"]:
    if parameter["name"] == "limit": parameter["schema"] = {"type": "integer", "minimum": 1, "maximum": 50, "default": 20}
PATHS["/notifications"]["get"]["parameters"] += [
    {"name": "state", "in": "query", "schema": enum("all", "unread", "attention")},
    {"name": "category", "in": "query", "schema": notification_categories}]
operation("/notifications/summary", "get", "getNotificationSummary", "NotificationSummary")
operation("/notifications/{id}", "get", "getNotification", "Notification")
operation("/notifications/{id}/read", "post", "markNotificationRead", "Notification", "NotificationRead")
operation("/notification-delivery-failures", "get", "listNotificationDeliveryFailures", "NotificationFailure", listing=True)
operation("/notification-delivery-jobs/{id}", "get", "getNotificationDeliveryJob", "NotificationDeliveryJob")
operation("/notification-delivery-jobs/{id}/retry", "post", "retryNotificationDelivery", "NotificationRetryReceipt", "NotificationRetryInput", 201)
operation("/notification-delivery-retries", "get", "listNotificationDeliveryRetries", "NotificationRetryReceipt", listing=True)
operation("/notification-contract-policy","get","getNotificationContractPolicy","NotificationContractPolicyState")
operation("/notification-contract-policy","post","publishNotificationContractPolicy","NotificationContractPolicy","NotificationContractPolicyInput",201)
operation("/notification-contract-policy-history","get","listNotificationContractPolicyHistory","NotificationContractPolicy",listing=True)
for parameter in PATHS["/notification-contract-policy-history"]["get"]["parameters"]:
    if parameter["name"]=="limit":parameter["schema"]={"type":"integer","minimum":1,"maximum":50,"default":20}
for parameter in PATHS["/notification-delivery-retries"]["get"]["parameters"]:
    if parameter["name"] == "limit": parameter["schema"] = {"type": "integer", "minimum": 1, "maximum": 50, "default": 20}
for parameter in PATHS["/notification-delivery-failures"]["get"]["parameters"]:
    if parameter["name"] == "limit": parameter["schema"] = {"type": "integer", "minimum": 1, "maximum": 50, "default": 20}
operation("/agent-budget", "get", "getAgentBudget", "BudgetSummary")
operation("/agent-budget/entries", "get", "listAgentBudgetEntries", "BudgetEntry", listing=True)
operation("/agent-budget/policies", "post", "publishAgentBudgetPolicy", "BudgetPolicy", "BudgetPolicyInput", 201)
operation("/agent-budget/entries/{id}/reconcile", "post", "reconcileAgentBudget", "BudgetEntry", "BudgetReconcileInput")
for path in ("/agent-budget/policies", "/agent-budget/entries/{id}/reconcile"):
    retired = PATHS[path]["post"]
    retired["deprecated"] = True
    retired["description"] = "Retired AI cost-control write. An authorized interactive manager receives410 AI_COST_CONTROL_RETIRED. Historical records remain read-only; no fee policy or reservation gates secretary execution."
    retired["responses"].pop("200", None)
    retired["responses"].pop("201", None)
    retired["responses"]["410"] = {"description": "AI cost control retired", "content": {"application/json": {"schema": ref("Error")}}}
for path in ("/agent-budget", "/agent-budget/entries"):
    PATHS[path]["get"].setdefault("parameters", []).extend([
        {"name":"month","in":"query","schema":{"type":"string","pattern":r"^\d{4}-\d{2}$"}},
        {"name":"scope","in":"query","schema":enum("mine","organization","user","project","skill")},
        {"name":"scope_id","in":"query","schema":NAME}])
operation("/conversations/{id}/runtime-options", "get", "getRuntimeOptions", "RuntimeOptions")
operation("/conversations/{id}/runs", "post", "createRun", "Run", "RunCreate", 202)
operation("/runs/{id}", "get", "getRun", "Run")
operation("/runs/{id}/oa-text-adoption", "post", "adoptOAText", "AgentOARequestView", "AgentOATextAdopt")
operation("/runs/{id}/contract-body-adoption", "post", "adoptContractBody", "AgentContractBodyView", "AgentContractBodyAdopt")
operation("/runs/{id}/cancel", "post", "cancelRun", "Run", "Transition")
operation("/runs/{id}/event-log", "get", "getRunEventLog", "RunEventPage")
PATHS["/runs/{id}/event-log"]["get"]["parameters"] += [
    {"name": "after_seq", "in": "query", "schema": {"type": "integer", "minimum": 0}},
    {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 100}}]
operation("/runs/{id}/events", "get", "streamRunEvents", "Run")
stream = PATHS["/runs/{id}/events"]["get"]
stream["parameters"] += [{"name": "Last-Event-ID", "in": "header", "schema": {"type": "string", "pattern": "^[0-9]+$"}},
    {"name": "after_seq", "in": "query", "schema": {"type": "integer", "minimum": 0}}]
stream["responses"]["200"] = {"description": "Ordered safe company events; private model reasoning excluded.",
    "content": {"text/event-stream": {"schema": {"type": "string"}}}}
operation("/conversations/{id}/plans", "get", "listAgentPlans", "AgentPlanList")
operation("/conversations/{id}/plans", "post", "proposeAgentProjectPlan", "AgentPlan", "AgentProjectPlanCreate", 201)
operation("/plans/{id}", "get", "getPlan", "AgentPlan")
operation("/plans/{id}/approve", "post", "approvePlan", "AgentPlan", "AgentPlanAction")
operation("/plans/{id}/revoke", "post", "revokePlan", "AgentPlan", "AgentPlanAction")
operation("/plans/{id}/commit", "post", "commitPlan", "AgentPlan", "AgentPlanAction")
operation("/conversations/{id}/directories/{folderID}", "get", "readAgentDirectory", "AgentDirectory")
operation("/conversations/{id}/organization-plans", "get", "listOrganizationPlans", "AgentOrganizationPlanList")
operation("/conversations/{id}/organization-plans", "post", "proposeOrganizationPlan", "AgentOrganizationPlan", "AgentOrganizationPlanCreate", 201)
operation("/organization-plans/{id}", "get", "getOrganizationPlan", "AgentOrganizationPlan")
for organization_action in ("approve", "revoke", "commit"):
    operation("/organization-plans/{id}/" + organization_action, "post", organization_action + "OrganizationPlan", "AgentOrganizationPlan", "AgentPlanAction")
operation("/operations/{id}", "get", "getOperation", "Operation")
operation("/services", "get", "listServices", "Service", listing=True)
operation("/services/{id}", "get", "getService", "Service")
operation("/services", "post", "createService", "Service", "ServiceCreate", 201)
operation("/services/{id}", "patch", "updateService", "Service", "ServicePatch")
operation("/services/{id}/sources", "post", "publishServiceSource", "Service", "ServiceSourceInput")
operation("/services/{id}/sources/{source}/revisions", "post", "reviseServiceSource", "Service", "ServiceSourceInput")
operation("/services/{id}/sources/{source}/revisions", "get", "listServiceSourceRevisions", "ServiceSourceRevision", listing=True)
operation("/services/{id}/observations", "get", "listServiceObservations", "ServiceObservation", listing=True)
operation("/services/{id}/observations/{observation}/evidence", "get", "getServiceObservationProof", "ServiceObservationProof")
operation("/services/{id}/maintenance-windows", "get", "listMaintenanceWindows", "MaintenanceWindow", listing=True)
operation("/services/{id}/maintenance-windows", "post", "scheduleMaintenance", "Service", "MaintenanceInput")
operation("/services/{id}/maintenance-windows/{window}/cancel", "post", "cancelMaintenance", "Service", "MaintenanceCancel")
operation("/status-sources/{source}/observations", "post", "ingestSignedObservation", "ObservationAck", "SignedObservation")
ingest = PATHS["/status-sources/{source}/observations"]["post"]
ingest["security"] = []
ingest["parameters"] = [p for p in ingest["parameters"] if "$ref" not in p]
ingest["parameters"].append({"name": "X-CP-Status-Signature", "in": "header", "required": True,
    "schema": {"type": "string", "minLength": 88, "maxLength": 88},
    "description": "Base64 Ed25519 signature of UTF-8 CP-STATUS-V1\\n + canonical lowercase source UUID + \\n + exact request bytes. No employee credentials. Max 4096 body bytes."})
PATHS["/services"]["get"]["parameters"] += [{"name": k, "in": "query", "schema": v} for k, v in
    (("q", NAME), ("lifecycle", enum("active", "archived")), ("project_id", ID))]
operation("/incidents", "get", "listIncidents", "Incident", listing=True)
operation("/incidents", "post", "createIncident", "Incident", "IncidentCreate", 201)
operation("/incidents/{id}", "get", "getIncident", "Incident")
operation("/incidents/{id}", "patch", "updateIncident", "Incident", "IncidentPatch")
operation("/incidents/{id}/timeline", "get", "listIncidentTimeline", "IncidentEntry", listing=True)
PATHS["/incidents"]["post"]["responses"]["200"] = {"description": "Existing identical business incident", "content": {"application/json": {"schema": wrapped("Incident")}}}
PATHS["/incidents"]["get"]["parameters"].append({"name": "state", "in": "query", "schema": INCIDENT_STATE})

for route, entity in (("contracts", "Contract"),):
    operation("/" + route, "get", "list" + entity + "s", entity, listing=True)
    operation("/" + route, "post", "create" + entity, entity, entity + "Create", 201)
    operation("/" + route + "/{id}", "get", "get" + entity, entity)
    operation("/" + route + "/{id}", "patch", "update" + entity, entity, entity + "Patch")
for route, entity in (("purchases", "Purchase"), ("claims", "Claim")):
    operation("/" + route, "get", "list" + entity + "s", "Spend", listing=True)
    operation("/" + route, "post", "create" + entity, "Spend", "SpendCreate", 201)
    operation("/" + route + "/{id}", "get", "get" + entity, "Spend")
    operation("/" + route + "/{id}", "patch", "update" + entity, "Spend", "SpendPatch")
    operation("/" + route + "/{id}/archive", "post", "archive" + entity + "Draft", "Spend", "SpendArchiveInput")
    operation("/" + route + "/{id}/revisions", "get", "list" + entity + "Revisions", "SpendRevision", listing=True)
    operation("/" + route + "/{id}/revisions/{revision}", "get", "get" + entity + "Revision", "SpendRevision")
    PATHS["/" + route]["get"]["parameters"].extend([
        {"name": "q", "in": "query", "schema": {"type": "string", "maxLength": 200}}, {"name": "project_id", "in": "query", "schema": ID},
        {"name": "lifecycle", "in": "query", "schema": spend_record["lifecycle"] if route == "purchases" else enum("draft", "requested")},
        {"name": "approval_state", "in": "query", "schema": spend_record["approval_state"]}])
PATHS["/contracts"]["get"]["parameters"].extend([
    {"name": "q", "in": "query", "schema": {"type": "string", "maxLength": 200}},
    {"name": "project_id", "in": "query", "schema": ID}, {"name": "trashed", "in": "query", "schema": {"type": "boolean"}},
    {"name": "lifecycle", "in": "query", "schema": enum("draft", "pending_signature", "effective", "completed", "terminated")}])
operation("/contracts/{id}/trash", "post", "trashContractDraft", "Contract", "ContractTrashInput")
operation("/contracts/{id}/revisions", "get", "listContractRevisions", "ContractRevision", listing=True)
operation("/contracts/{id}/revisions/{revision}", "get", "getContractRevision", "ContractRevision")
operation("/contracts/{id}/transition", "post", "transitionContract", "Contract", "ContractActivation")
operation("/contracts/{id}/signature", "post", "registerContractSignature", "Contract", "ContractSignature", 201)
operation("/contracts/{id}/milestones", "post", "reviseContractPaymentSchedule", "Contract", "ContractScheduleInput")
operation("/contracts/{id}/milestones", "get", "listContractMilestones", "ContractMilestone", listing=True)
operation("/contracts/{id}/finance", "get", "getContractFinance", "ContractFinance")
operation("/contracts/{id}/finance/events", "get", "listContractFinanceEvents", "ContractFinanceEvent", listing=True)
PATHS["/contracts/{id}/finance/events"]["get"]["parameters"].append({"name": "milestone_id", "in": "query", "schema": ID})
operation("/contracts/{id}/finance/entries", "post", "linkContractFinance", "ContractFinanceReceipt", "ContractFinanceLink")
operation("/contracts/{id}/finance/entries/{entry}/release", "post", "releaseContractFinance", "ContractFinanceReceipt", "ContractFinanceRelease")
operation("/contracts/{id}/finance/milestones/{milestone}/review", "post", "reviewContractFinance", "ContractFinanceReceipt", "ContractFinanceReview")
operation("/contracts/{id}/closure-preview", "get", "getContractClosurePreview", "ContractClosurePreview")
operation("/contracts/{id}/closure", "get", "getContractClosure", "ContractClosure")
operation("/contracts/{id}/closure", "post", "closeContract", "ContractClosureReceipt", "ContractClosureInput", code=201)
operation("/purchases/{id}/transition", "post", "transitionPurchase", "Purchase", "Transition")
operation("/purchases/{id}/fulfillment", "get", "getPurchaseFulfillment", "PurchaseFulfillment")
operation("/purchases/{id}/completion-preview", "get", "getPurchaseCompletionPreview", "PurchaseCompletionPreview")
operation("/purchases/{id}/completion", "get", "getPurchaseCompletion", "PurchaseCompletion")
operation("/purchases/{id}/completion", "post", "completePurchase", "PurchaseCompletionCommandReceipt", "PurchaseCompletionInput", code=201)
operation("/purchases/{id}/orders", "get", "listPurchaseOrders", "PurchaseOrder", listing=True)
operation("/purchases/{id}/orders/{order}", "get", "getPurchaseOrder", "PurchaseOrder")
operation("/purchases/{id}/orders", "post", "createPurchaseOrder", "Spend", "PurchaseOrderCreate", 201)
operation("/purchases/{id}/orders/{order}/cancel", "post", "cancelPurchaseOrderRemainder", "Spend", "PurchaseExecutionInput")
operation("/purchases/{id}/receipts", "get", "listPurchaseReceipts", "PurchaseReceipt", listing=True)
PATHS["/purchases/{id}/receipts"]["get"]["parameters"].append({"name": "order_id", "in": "query", "schema": ID})
operation("/purchases/{id}/receipts/{receipt}", "get", "getPurchaseReceipt", "PurchaseReceipt")
operation("/purchases/{id}/receipts", "post", "recordPurchaseReceipt", "Spend", "PurchaseReceiptCreate", 201)
operation("/purchases/{id}/execution-events", "get", "listPurchaseExecutionEvents", "PurchaseExecutionEvent", listing=True)
operation("/purchases/{id}/execution-plan", "get", "getPurchaseExecutionPlan", "PurchaseExecutionPlan")
operation("/purchases/{id}/variances", "get", "listPurchaseVariances", "PurchaseVariance", listing=True)
operation("/purchases/{id}/variances", "post", "createPurchaseVariance", "PurchaseVariance", "PurchaseVarianceCreate", 201)
operation("/purchase-variances/{id}", "get", "getPurchaseVariance", "PurchaseVariance")
operation("/purchase-variances/{id}", "patch", "updatePurchaseVariance", "PurchaseVariance", "PurchaseVariancePatch")
operation("/purchase-variances/{id}/revisions", "get", "listPurchaseVarianceRevisions", "PurchaseVarianceRevision", listing=True)
operation("/purchase-variances/{id}/revisions/{revision}", "get", "getPurchaseVarianceRevision", "PurchaseVarianceRevision")
operation("/purchase-variances/{id}/activate", "post", "activatePurchaseVariance", "PurchaseVariance", "PurchaseVarianceActivation")

operation("/purchases/{id}/cancel", "post", "cancelPurchase", "Spend", "PurchaseExecutionInput")
for leaf, entity in (("milestones", "Milestone"), ("deliverables", "Deliverable"), ("risks", "Risk"), ("issues", "Issue"), ("changes", "Change")):
    operation("/projects/{id}/" + leaf, "get", "list" + entity + "s", entity, listing=True)
    operation("/projects/{id}/" + leaf, "post", "create" + entity, entity, entity + "Create", 201)
for route, entity in (("risks", "Risk"), ("issues", "Issue")):
    operation("/" + route + "/{id}", "get", "get" + entity, entity)
    operation("/" + route + "/{id}", "patch", "update" + entity, entity, entity + "Patch")
operation("/projects/{id}/deliverables", "post", "createDeliverable", "DeliverableReceipt", "DeliverableCreate", 201)
operation("/deliverables/{id}", "get", "getDeliverable", "Deliverable")
operation("/deliverables/{id}", "patch", "updateDeliverable", "DeliverableReceipt", "DeliverablePatch")
operation("/deliverables/{id}/submit", "post", "submitDeliverable", "DeliverableReceipt", "DeliverableSubmit")
operation("/deliverables/{id}/decisions", "post", "decideDeliverable", "DeliverableReceipt", "DeliverableDecisionInput")
operation("/deliverables/{id}/revisions", "get", "listDeliverableRevisions", "DeliverableRevision", listing=True)
operation("/deliverables/{id}/revisions/{revision}", "get", "getDeliverableRevision", "DeliverableRevision")
operation("/projects/{id}/changes", "post", "createChange", "ChangeReceipt", "ChangeCreate", 201)
operation("/projects/{id}/change-basis", "get", "getProjectChangeBasis", "ChangeBasis")
operation("/projects/{id}/closeout", "get", "getProjectCloseout", "ProjectCloseout")
operation("/projects/{id}/archives", "get", "listProjectArchives", "ProjectArchiveList")
PATHS["/projects/{id}/archives"]["get"]["parameters"] += [{"name":"cursor","in":"query","schema":CURSOR},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":100,"default":20}}]
operation("/projects/{id}/archive", "post", "archiveProject", "ProjectArchiveReceipt", "ProjectArchiveInput", 201)
operation("/projects/{id}/budgets", "get", "listProjectBudgets", "ProjectBudget", listing=True)
operation("/projects/{id}/budget-comparison", "get", "compareProjectBudget", "BudgetComparison")
operation("/projects/{id}/budget-comparison/entries", "get", "listBudgetComparisonEntries", "BudgetComparisonEntries")
PATHS["/projects/{id}/budget-comparison"]["get"]["parameters"] += [{"name":"currency","in":"query","required":True,"schema":CURRENCY}]
PATHS["/projects/{id}/budget-comparison/entries"]["get"]["parameters"] += [{"name":"currency","in":"query","required":True,"schema":CURRENCY},{"name":"basis_hash","in":"query","required":True,"schema":HASH},{"name":"line","in":"query","schema":{"type":"string","pattern":r"^(outside|[0-9]|[1-9][0-9])$"}},{"name":"state","in":"query","schema":{**enum("incurred","draft"),"default":"incurred"}},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":100,"default":20}},{"name":"cursor","in":"query","schema":CURSOR}]
operation("/changes/{id}", "get", "getChange", "Change")
operation("/changes/{id}", "patch", "updateChange", "ChangeReceipt", "ChangePatch")
operation("/changes/{id}/revisions", "get", "listChangeRevisions", "ChangeRevision", listing=True)
operation("/changes/{id}/revisions/{revision}", "get", "getChangeRevision", "ChangeRevision")
operation("/changes/{id}/submit", "post", "prepareChangeRequest", "Request", "ChangeSubmit", 201)
operation("/changes/{id}/implement", "post", "implementChange", "ChangeReceipt", "ChangeImplement")
PATHS["/projects/{id}/change-basis"]["get"]["parameters"].append({
    "name": "currency", "in": "query", "required": True,
    "description": "Configured original currency whose current approved budget forms the fixed baseline.",
    "schema": {"type": "string", "pattern": "^[A-Z]{3}$"}})
PATHS["/projects/{id}/budgets"]["get"]["parameters"].extend([
    {"name": "currency", "in": "query", "schema": {"type": "string", "pattern": "^[A-Z]{3}$"}},
    {"name": "state", "in": "query", "schema": enum("draft", "approved", "superseded")}])
PATHS["/projects/{id}/changes"]["get"]["parameters"].extend([
    {"name": "q", "in": "query", "schema": {"type": "string", "maxLength": 200}},
    {"name": "state", "in": "query", "schema": enum("draft", "in_review", "approved", "rejected", "withdrawn", "implemented")}])

# Financial reports and immutable, expiring asynchronous exports.
# Immutable manual FX valuation policy; reporting remains distinct from settlement.
fx_rate = {"currency":CURRENCY,"valid_from":DATE,"valid_to":DATE,"rate_date":DATE,
    "rate":{"type":"string","pattern":r"^(0|[1-9][0-9]{0,9})(\.[0-9]{1,10})?$"},"source":NAME}
add("FinanceFXRate",fx_rate,tuple(fx_rate))
fx_content = {"title":NAME,"base_currency":CURRENCY,"method":enum("manual_interval"),"rounding":enum("scope_line_half_up"),"rates":array(ref("FinanceFXRate"),maxItems=2000)}
add("FinanceFXContent",fx_content,tuple(fx_content))
fx_book = {"id":ID,"revision":VERSION,"title":NAME,"base_currency":CURRENCY,"rate_count":{"type":"integer","minimum":0,"maximum":2000},
    "content_sha256":HASH,"creator_id":ID,"reason":NAME,"created_at":TIME}
add("FinanceFXBook",{**fx_book,"content":ref("FinanceFXContent")},tuple(fx_book))
add("FinanceFXPolicy",{"version":{"type":"integer","minimum":0,"maximum":9007199254740991},"current":nullable(ref("FinanceFXBook"))},("version","current"))
add("FinanceFXBookCreate",{"expected_version":{"type":"integer","minimum":0,"maximum":9007199254740991},"content":ref("FinanceFXContent"),"acknowledged":{"type":"boolean","enum":[True]},"reason":NAME},("expected_version","content","acknowledged","reason"))
fx_decimal = {"type":"string","pattern":r"^-?(0|[1-9][0-9]*)(\.[0-9]{1,6})?$"}
fx_count = {"type":"integer","minimum":0,"maximum":9007199254740991}
fx_source = {"status":enum("identity","converted","missing_rate","unconfirmed","unpriced"),"fx_date":DATE,
    "base_amount":nullable(fx_decimal),"rate":nullable(fx_rate["rate"]),"rate_index":nullable({"type":"integer","minimum":0,"maximum":1999}),
    "rate_date":nullable(DATE),"rate_source":nullable(NAME),"valid_from":nullable(DATE),"valid_to":nullable(DATE)}
add("FinancialSourceValuation",fx_source,tuple(fx_source))
fx_group = {"currency":nullable(CURRENCY),"covered_count":fx_count,"missing_rate_count":fx_count,"unpriced_count":fx_count,"unconfirmed_count":fx_count,
    "covered_amount":fx_decimal,"missing_native_amount":fx_decimal}
add("FinancialValuationCurrency",fx_group,tuple(fx_group))
fx_valuation = {"source_coverage_complete":{"type":"boolean"},"book":ref("FinanceFXBook"),"exponent":{"type":"integer","minimum":0,"maximum":6},"method":enum("manual_interval"),"rounding":enum("scope_line_half_up"),
    "coverage":enum("complete","partial","none"),"covered_amount":fx_decimal,"total_amount":nullable(fx_decimal),
    "covered_count":fx_count,"missing_rate_count":fx_count,"unpriced_count":fx_count,"unconfirmed_count":fx_count,"currencies":array(ref("FinancialValuationCurrency"))}
add("FinancialReportValuation",fx_valuation,tuple(fx_valuation))
operation("/finance/fx-policy","get","readFinanceFXPolicy","FinanceFXPolicy")
operation("/finance/fx-books","get","listFinanceFXBooks","FinanceFXBook",listing=True)
operation("/finance/fx-books/{id}","get","readFinanceFXBook","FinanceFXBook")
operation("/finance/fx-books","post","publishFinanceFXBook","FinanceFXBook","FinanceFXBookCreate",201)
for fx_path in ("/finance/fx-policy","/finance/fx-books","/finance/fx-books/{id}"):
    PATHS[fx_path]["get"]["parameters"] += [{"name":"project_id","in":"query","schema":ID}]

report_query_fields = {"kind": enum("incurred", "cash", "forecast"), "from": DATE, "to": DATE,
    "currency": {"type":"string","pattern":"^([A-Z]{3})?$"}, "category":{"type":"string","maxLength":50},
    "q":{"type":"string","maxLength":200}, "project_id":nullable(ID), "company_only":{"type":"boolean"},
    "supplier_id":nullable(ID), "owner_id":nullable(ID), "fx_book_id":nullable(ID)}
add("FinancialReportQuery", report_query_fields, ("kind","from","to","company_only"))
report_source_fields = {"source_type":enum("cost","payment","subscription_period"), "id":ID,"resource_id":ID,"revision_id":ID,
    "version":VERSION,"title":{"type":"string","maxLength":20000},"from":DATE,"to":nullable(DATE),"state":NAME,"entry_kind":enum("expense","reversal","payment","forecast"),
    "currency":nullable(CURRENCY),"original_amount":nullable(MONEY),"contribution_amount":nullable({"type":"string","pattern":r"^-?(0|[1-9][0-9]*)(\.[0-9]{1,6})?$"}),
    "category":{"type":"string","maxLength":50},"supplier_id":nullable(ID),"owner_id":ID,"project_id":nullable(ID),"created_at":TIME}
report_source_fields["timezone"] = {"type":"string","minLength":1,"maxLength":100}
add("FinancialReportSource", {**report_source_fields,"valuation":ref("FinancialSourceValuation")}, tuple(report_source_fields))
add("FinancialReport", {"filters":ref("FinancialReportQuery"),"scope":enum("authorized_records_only"),
    "summary":{"anyOf":[ref("CostReport"),ref("CashReport"),ref("ForecastReport")]},
    "items":array(ref("FinancialReportSource")),"next_cursor":nullable(CURSOR),"valuation":ref("FinancialReportValuation")}, ("filters","scope","summary","items","next_cursor"))
export_fields = {"id":ID,"job_id":ID,"filters":ref("FinancialReportQuery"),"state":enum("queued","running","retry_wait","succeeded","dead","cancelled","expired"),
    "attempt":{"type":"integer","minimum":0,"maximum":4},"error_code":nullable(TEXT),"created_at":TIME,"expires_at":TIME,"as_of":nullable(TIME),
    "row_count":nullable({"type":"integer","minimum":0,"maximum":50000}),"csv_bytes":nullable({"type":"integer","minimum":1,"maximum":16777216}),
    "csv_sha256":nullable(HASH),"can_cancel":{"type":"boolean"},"can_download":{"type":"boolean"}}
add("FinancialReportExport",export_fields,tuple(export_fields))
report_reason = {"type":"string","minLength":1,"maxLength":2000}
add("FinancialReportExportCreate",{"filters":ref("FinancialReportQuery"),"reason":report_reason},("filters","reason"))
add("FinancialReportExportCancel",{"reason":report_reason},("reason",))
chunk_fields={"export_id":ID,"filename":NAME,"sha256":HASH,"total_size_bytes":BYTES,"offset":BYTES,"length":BYTES,"chunk_sha256":HASH,"data_base64":{"type":"string","maxLength":699052}}
add("FinancialReportExportChunk",chunk_fields,tuple(chunk_fields))
operation("/finance/reports","get","readFinancialReport","FinancialReport")
PATHS["/finance/reports"]["get"]["parameters"] += [{"name":key,"in":"query","required":key in ("kind","from","to"),"schema":schema if key not in ("project_id","supplier_id","owner_id","fx_book_id") else ID} for key,schema in report_query_fields.items()]
PATHS["/finance/reports"]["get"]["parameters"] += [{"name":"cursor","in":"query","schema":CURSOR},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":100,"default":20}}]
operation("/finance/report-exports","post","createFinancialReportExport","FinancialReportExport","FinancialReportExportCreate",202)
operation("/finance/report-exports","get","listFinancialReportExports","FinancialReportExport",listing=True)
PATHS["/finance/report-exports"]["get"]["parameters"] += [{"name":"kind","in":"query","schema":enum("incurred","cash","forecast")},{"name":"state","in":"query","schema":export_fields["state"]}]
operation("/finance/report-exports/{id}","get","readFinancialReportExport","FinancialReportExport")
operation("/finance/report-exports/{id}/cancel","post","cancelFinancialReportExport","FinancialReportExport","FinancialReportExportCancel")
operation("/finance/report-exports/{id}/chunks","get","readFinancialReportExportChunk","FinancialReportExportChunk")
PATHS["/finance/report-exports/{id}/chunks"]["get"]["parameters"] += [{"name":"offset","in":"query","required":True,"schema":BYTES},{"name":"length","in":"query","required":True,"schema":BYTES}]
operation("/finance/report-exports/{id}/download","get","downloadFinancialReportCSV","FinancialReportExport")
PATHS["/finance/report-exports/{id}/download"]["get"]["responses"]["200"]={"description":"Reauthorized UTF-8 CSV; fixed source manifest, no durable server file.","headers":{"X-Content-SHA256":{"schema":HASH}},"content":{"text/csv":{"schema":{"type":"string","format":"binary"}}}}

add("FinancialReportOption",{"id":ID,"value":{"type":"string","maxLength":50},"label":{"type":"string","minLength":1,"maxLength":500}},("id","value","label"))
operation("/finance/report-options","get","readFinancialReportOptions","FinancialReportOption",listing=True)
PATHS["/finance/report-options"]["get"]["parameters"] += [{"name":key,"in":"query","required":key in ("kind","from","to"),"schema":schema if key not in ("project_id","supplier_id","owner_id","fx_book_id") else ID} for key,schema in report_query_fields.items()]
PATHS["/finance/report-options"]["get"]["parameters"] += [{"name":"field","in":"query","required":True,"schema":enum("category","supplier","owner")},{"name":"option_search","in":"query","schema":{"type":"string","maxLength":200}}]

audit_filter_fields = {"from": TIME, "to": TIME, **{key: {"type": "string", "maxLength": 128} for key in (
    "actor_id", "executor_id", "channel", "action", "result", "resource_type", "resource_id", "request_id", "run_id", "job_id")}}
add("AuditFilter", audit_filter_fields, tuple(audit_filter_fields))
audit_event_fields = {"id": ID, "occurred_at": TIME, "actor_id": nullable(ID), "executor_id": nullable(ID),
    "channel": {"type": "string"}, "action": {"type": "string"}, "result": {"type": "string"}, "resource_type": {"type": "string"},
    "resource_id": nullable(ID), "resource_version": nullable({"type": "string", "pattern": r"^-?[0-9]+$"}), "request_id": {"type": "string"},
    "run_id": nullable(ID), "job_id": nullable(ID), "before_digest": nullable({"type": "string"}), "after_digest": nullable({"type": "string"})}
add("AuditEvent", audit_event_fields, tuple(audit_event_fields))
add("AuditEventPage", {"items": array(ref("AuditEvent"), maxItems=100), "next_cursor": nullable(CURSOR), "filters": ref("AuditFilter"), "disclosure": enum("organization_metadata_v1")}, ("items", "next_cursor", "filters", "disclosure"))
audit_export_fields = {"id": ID, "filters": ref("AuditFilter"), "disclosure": enum("organization_metadata_v1"), "created_at": TIME, "expires_at": TIME,
    "row_count": {"type": "integer", "minimum": 0, "maximum": 10000}, "csv_bytes": {"type": "integer", "minimum": 1, "maximum": 8388608}, "csv_sha256": HASH,
    "state": enum("ready", "expired", "authorization_changed"), "can_download": {"type": "boolean"}}
add("AuditExport", audit_export_fields, tuple(audit_export_fields))
add("AuditExportCreate", {"filters": ref("AuditFilter"), "reason": NAME}, ("filters", "reason"))
audit_chunk_fields = {"export_id": ID, "filename": NAME, "total_size_bytes": BYTES, "sha256": HASH, "offset": BYTES, "length": BYTES,
    "chunk_sha256": HASH, "data_base64": {"type": "string", "maxLength": 699052}}
add("AuditExportChunk", audit_chunk_fields, tuple(audit_chunk_fields))
operation("/audit/events", "get", "listAuditEvents", "AuditEventPage")
PATHS["/audit/events"]["get"]["parameters"] += [{"name": key, "in": "query", "required": key in ("from", "to"), "schema": schema} for key, schema in audit_filter_fields.items()]
PATHS["/audit/events"]["get"]["parameters"] += [{"name": "cursor", "in": "query", "schema": CURSOR}, {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 100, "default": 20}}]
operation("/audit/events/{id}", "get", "readAuditEvent", "AuditEvent")
operation("/audit/exports", "get", "listAuditExports", "AuditExport", listing=True)
operation("/audit/exports", "post", "createAuditExport", "AuditExport", "AuditExportCreate", 201)
operation("/audit/exports/{id}", "get", "readAuditExport", "AuditExport")
operation("/audit/exports/{id}/chunks", "get", "readAuditExportChunk", "AuditExportChunk")
PATHS["/audit/exports/{id}/chunks"]["get"]["parameters"] += [{"name": "offset", "in": "query", "required": True, "schema": BYTES}, {"name": "length", "in": "query", "required": True, "schema": BYTES}]
operation("/audit/exports/{id}/download", "get", "downloadAuditCSV", "AuditExport")
PATHS["/audit/exports/{id}/download"]["get"]["responses"]["200"] = {"description": "Reauthorized immutable metadata CSV; contains no free-form reasons, sessions or business payloads.", "headers": {"X-Content-SHA256": {"schema": HASH}}, "content": {"text/csv": {"schema": {"type": "string", "format": "binary"}}}}

scope_transfer_create = {"kind": enum("file", "folder"), "resource_id": ID, "expected_version": VERSION,
    "target_folder_id": ID, "expected_target_version": VERSION, "new_name": nullable(NAME), "reason": NAME}
add("ScopeTransferCreate", scope_transfer_create, tuple(scope_transfer_create))
add("ScopeTransferDecision", {"expected_version": VERSION, "plan_hash": HASH, "reason": NAME}, ("expected_version", "plan_hash", "reason"))
scope_visibility = {"user_id": ID, "display_name": NAME, "channel": POLICY_CHANNEL,
    "ai": {"type": "boolean"}, "before": {"type": "boolean"}, "after": {"type": "boolean"}}
add("ScopeVisibilityChange", scope_visibility, tuple(scope_visibility))
scope_item = {"kind": enum("file", "folder"), "id": ID, "version": VERSION, "name": NAME,
    "state": enum("active", "trashed"), "parent_id": nullable(ID), "project_id": nullable(ID), "creator_id": nullable(ID),
    "classification": CLASS, "new_classification": CLASS, "current_version_id": nullable(ID), "content_versions": COUNT,
    "content_sha256": {"type": "string", "maxLength": 64}, "human_before": COUNT, "human_after": COUNT,
    "ai_before": COUNT, "ai_after": COUNT, "audience_sha256": HASH,
    "visibility_changes": array(ref("ScopeVisibilityChange"), maxItems=1000), "blocking_reasons": array(NAME, maxItems=10)}
add("ScopeTransferItem", scope_item, tuple(scope_item))
scope_snapshot = {"source": ref("AgentOrganizationEntry"), "source_path": array(ref("AgentOrganizationEntry"), maxItems=32),
    "target": ref("AgentOrganizationEntry"), "target_path": array(ref("AgentOrganizationEntry"), maxItems=32),
    "new_name": NAME, "items": array(ref("ScopeTransferItem"), minItems=1, maxItems=100)}
add("ScopeTransferSnapshot", scope_snapshot, tuple(scope_snapshot))
scope_transfer = {"id": ID, "version": VERSION, "state": enum("pending", "executed", "cancelled"), "creator_id": ID,
    "input": ref("ScopeTransferCreate"), "snapshot": ref("ScopeTransferSnapshot"), "plan_hash": HASH,
    "created_at": TIME, "expires_at": TIME, "completed_at": nullable(TIME), "decision_reason": nullable(NAME),
    "current_basis_matches": {"type": "boolean"}, "can_execute": {"type": "boolean"}}
add("ScopeTransfer", scope_transfer, tuple(scope_transfer))
operation("/file-scope-transfers", "post", "createFileScopeTransfer", "ScopeTransfer", "ScopeTransferCreate", 201)
operation("/file-scope-transfers/{id}", "get", "readFileScopeTransfer", "ScopeTransfer")
operation("/file-scope-transfers/{id}/execute", "post", "executeFileScopeTransfer", "ScopeTransfer", "ScopeTransferDecision")
operation("/file-scope-transfers/{id}/cancel", "post", "cancelFileScopeTransfer", "ScopeTransfer", "ScopeTransferDecision")
operation("/files/{id}/scope-transfers", "get", "listFileScopeTransfers", "ScopeTransfer", listing=True)
operation("/folders/{id}/scope-transfers", "get", "listFolderScopeTransfers", "ScopeTransfer", listing=True)

document = {
    "openapi": "3.1.0",
    "info": {"title": "CopyPaste internal platform core contract", "version": "1.0.0-draft",
        "description": "Versioned contract. Implementation and external verification status are recorded in docs/delivery/02-implementation-ledger.md."},
    "servers": [{"url": "/v1", "description": "Actual domain pending Q-01"}],
    "x-contract-scope": {"status": "draft", "covered": "Projects, tasks, deliverables, risks/changes, knowledge, contracts, purchases, claims, upload/commit, approval, subscriptions, costs/payments, agent runs/plans and incidents.",
        "extensions-required-before-implementation": [
            "Platform identity binding and independent recovery delivery",
            "Role/policy/member management; workflow and project template publishing",
            "Comments, budget revisions and specialized project closure exceptions",
            "Contract amendment activation, milestone fulfillment and advanced purchase exception rules",
            "Request revision editing and OCR review",
            "Custom subscription cycles, effective cancellation, actual charge confirmation",
            "Counterparty CRUD, report snapshots and exports",
            "Telegram webhook/binding, notifications, probe configuration",
            "Retention, backup, audit, skill publishing and operations"]},
    "paths": PATHS,
    "components": {
        "securitySchemes": {"bearerAuth": {"type": "http", "scheme": "bearer"},
            "cookieAuth": {"type": "apiKey", "in": "cookie", "name": "cp_session"}},
        "parameters": {
            "IdempotencyKey": {"name": "Idempotency-Key", "in": "header", "required": True,
                "schema": {"type": "string", "minLength": 16, "maxLength": 200},
                "description": "Stable across retries, scoped to actor and route."},
            "CsrfToken": {"name": "X-CSRF-Token", "in": "header", "required": False,
                "schema": NAME, "description": "Server requires for every cookie-authenticated write; Bearer-only excluded."}},
        "responses": {"Failure": {"description": "Safe structured error; see 04-api-contract.md",
            "content": {"application/json": {"schema": ref("Error")}}}},
        "schemas": SCHEMAS}}

if __name__ == "__main__":
    root = Path(__file__).resolve().parents[2]
    target = root / "docs/development/openapi.json"
    content = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    target.write_text(content, encoding="utf-8")
    embedded = root / "app/backend/internal/contract/openapi.json"
    embedded.parent.mkdir(parents=True, exist_ok=True)
    embedded.write_text(content, encoding="utf-8")
    # Keep independently maintained business extensions in the canonical build.
    # Rebuilding the base alone used to erase the published amortization API.
    from update_amortization_contract import main as add_amortization
    add_amortization()
    complete = json.loads(target.read_text(encoding="utf-8"))
    print(json.dumps({"artifact": str(target), "paths": len(complete["paths"]),
        "operations": sum(len(value) for value in complete["paths"].values()), "schemas": len(complete["components"]["schemas"])}))
