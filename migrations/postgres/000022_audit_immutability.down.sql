DROP FUNCTION IF EXISTS audit_prune(timestamptz);
DROP TRIGGER IF EXISTS audit_log_no_mutation ON audit_log;
DROP FUNCTION IF EXISTS audit_block_mutation();
