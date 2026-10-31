-- Organization delivery operations grant metadata authority, not message-body
-- access, group membership, model authority or permission to disclose results.
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'telegram_operations_reader','TG投递运维查阅人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'telegram_operations_manager','TG投递对账经办人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'telegram.operations.read' FROM roles WHERE key IN ('telegram_operations_reader','telegram_operations_manager','system_admin');
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'telegram.operations.reconcile' FROM roles WHERE key IN ('telegram_operations_manager','system_admin');
CREATE INDEX telegram_operations_page ON telegram_deliveries(org_id,created_at DESC,id DESC);
CREATE INDEX telegram_operations_state_page ON telegram_deliveries(org_id,state,created_at DESC,id DESC);
CREATE INDEX telegram_operations_channel_page ON telegram_deliveries(org_id,channel_id,created_at DESC,id DESC);
-- Invalidate cached authority after backfilling the explicit system-admin actions.
UPDATE organizations SET authorization_version=authorization_version+1;
