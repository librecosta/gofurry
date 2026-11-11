-- +goose NO TRANSACTION
-- +goose Up
DROP INDEX CONCURRENTLY IF EXISTS site_daily_coverage_by_day;
CREATE INDEX CONCURRENTLY site_daily_coverage_by_day ON site_daily_coverage (utc_day,site_id);
