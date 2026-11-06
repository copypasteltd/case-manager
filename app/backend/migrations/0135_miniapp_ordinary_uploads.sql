-- Ordinary bounded human miniapp uploads share the existing immutable cloud workflow.
ALTER TABLE upload_sessions DROP CONSTRAINT upload_sessions_source_channel_check;
ALTER TABLE upload_sessions ADD CONSTRAINT upload_sessions_source_channel_check
 CHECK(source_channel IN ('web','android','wechat','douyin','agent'));
ALTER TABLE upload_sessions ADD CONSTRAINT miniapp_upload_bound CHECK
 (source_channel NOT IN ('wechat','douyin') OR (size_bytes<=20971520 AND source_fingerprint ~ '^source-sha256-v2:[a-f0-9]{64}$'));
