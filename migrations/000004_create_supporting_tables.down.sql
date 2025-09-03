-- Activity logs
DROP INDEX IF EXISTS idx_activity_logs_created;
DROP INDEX IF EXISTS idx_activity_logs_action;
DROP INDEX IF EXISTS idx_activity_logs_lot;
DROP TABLE IF EXISTS activity_logs;

-- Async jobs
DROP INDEX IF EXISTS idx_async_jobs_type;
DROP INDEX IF EXISTS idx_async_jobs_status;
DROP TABLE IF EXISTS async_jobs;
