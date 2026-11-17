\set ON_ERROR_STOP on
BEGIN;
SELECT current_database() = 'librecosta_admin_cn' AS admin_db,
       current_database() = 'gateway'             AS gateway_db,
       current_database() = 'gofurry_cn'          AS gofurry_db,
       current_database() = 'harvest'             AS harvest_db,
       current_database() = 'identity'            AS identity_db
\gset

DO $$
DECLARE
    expected record;
    applied_version bigint;
    is_applied boolean;
    owner_oid oid;
BEGIN
    SELECT * INTO expected FROM (VALUES
        ('librecosta_admin_cn', 'librecosta_admin_migrator', 'librecosta_admin_runtime', 202611050002),
        ('gateway',             'gateway_owner',             'gateway_runtime',          4),
        ('gofurry_cn',          'gofurry_cn_migrator',        'gofurry_cn_runtime',       202611150003),
        ('harvest',             'harvest_migrator',           'harvest_runtime',          7),
        ('identity',            'identity_migrator',          'identity_runtime',        10)
    ) AS p(db, migrator, runtime, version) WHERE p.db = current_database();
    IF NOT FOUND OR current_setting('server_version_num')::integer / 10000 <> 18
       OR current_user <> expected.migrator OR session_user <> expected.migrator THEN
        RAISE EXCEPTION 'Use the designated migrator on its PG18 CN database';
    END IF;

    SELECT oid INTO owner_oid FROM pg_roles WHERE rolname = current_user;
    IF NOT EXISTS (
        SELECT 1 FROM pg_database d WHERE d.datname = current_database() AND d.datdba = owner_oid
          AND shobj_description(d.oid, 'pg_database') = 'librecosta-cn-ops-s2-c/v1'
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_roles r WHERE r.rolname = expected.runtime AND r.rolcanlogin AND NOT r.rolsuper
          AND shobj_description(r.oid, 'pg_authid') = 'librecosta-cn-ops-s2-c/v1'
    ) THEN
        RAISE EXCEPTION 'Database or runtime role is outside the provisioning contract';
    END IF;

    SELECT g.version_id, g.is_applied INTO applied_version, is_applied
    FROM public.goose_db_version g ORDER BY g.id DESC LIMIT 1;
    IF applied_version IS DISTINCT FROM expected.version OR is_applied IS DISTINCT FROM true THEN
        RAISE EXCEPTION 'Expected applied Goose version %', expected.version;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
               WHERE n.nspname IN ('public', 'harvest') AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
                 AND c.relowner <> owner_oid)
       OR EXISTS (SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
                  WHERE n.nspname IN ('public', 'harvest') AND p.proowner <> owner_oid) THEN
        RAISE EXCEPTION 'Unexpected object ownership; check migrations or restore ownership';
    END IF;
END
$$;

SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', current_database()),
       format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), CASE current_database()
           WHEN 'librecosta_admin_cn' THEN 'librecosta_admin_runtime'
           WHEN 'gateway'             THEN 'gateway_runtime'
           WHEN 'gofurry_cn'          THEN 'gofurry_cn_runtime'
           WHEN 'harvest'             THEN 'harvest_runtime'
           WHEN 'identity'            THEN 'identity_runtime' END)
\gexec
REVOKE ALL ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC;
REVOKE EXECUTE ON ALL ROUTINES IN SCHEMA public FROM PUBLIC;
ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON ROUTINES FROM PUBLIC;

\if :admin_db
    GRANT USAGE ON SCHEMA public TO librecosta_admin_runtime;
    GRANT SELECT, INSERT, UPDATE ON public.admin_accounts, public.admin_sessions TO librecosta_admin_runtime;
    GRANT SELECT, INSERT ON public.admin_audit_logs TO librecosta_admin_runtime;
\endif

\if :gateway_db
    GRANT USAGE ON SCHEMA public TO gateway_runtime;
    GRANT SELECT ON public.providers, public.provider_accounts, public.provider_endpoints, public.provider_credentials,
        public.capability_profiles, public.physical_models, public.deployments, public.routing_policies,
        public.logical_models, public.route_candidates TO gateway_runtime;
    GRANT SELECT, INSERT, UPDATE ON public.gateway_instances, public.gateway_requests, public.gateway_attempts,
        public.gateway_attempt_usage, public.gateway_jobs, public.gateway_job_identities, public.gateway_buckets,
        public.gateway_rollups, public.gateway_rollup_archive, public.gateway_usage_adjustments TO gateway_runtime;
    GRANT DELETE ON public.gateway_requests, public.gateway_buckets, public.gateway_rollups,
        public.gateway_usage_adjustments TO gateway_runtime;
    GRANT SELECT, INSERT ON public.gateway_audit_events TO gateway_runtime;
    GRANT EXECUTE ON FUNCTION public.aigw_touch(timestamptz) TO gateway_runtime;
\endif

\if :gofurry_db
    GRANT USAGE ON SCHEMA public TO gofurry_cn_runtime;
    GRANT EXECUTE ON ALL ROUTINES IN SCHEMA public TO gofurry_cn_runtime;
    -- Exact schema version is checked above. Import provenance belongs to the offline importer.
    SELECT format('GRANT SELECT ON %I.%I TO gofurry_cn_runtime', n.nspname, c.relname),
           CASE WHEN c.relkind IN ('r', 'p') AND c.relname NOT IN
                ('management_operations', 'catalog_publication_decisions', 'wallpaper_audit_logs', 'site_navigation_reviews')
                THEN format('GRANT INSERT, UPDATE, DELETE ON %I.%I TO gofurry_cn_runtime', n.nspname, c.relname)
                WHEN c.relkind IN ('r', 'p')
                THEN format('GRANT INSERT ON %I.%I TO gofurry_cn_runtime', n.nspname, c.relname)
           END
    FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm')
      AND c.relname NOT IN ('goose_db_version', 'legacy_import_batches', 'legacy_import_receipts',
                            'legacy_site_ids', 'legacy_game_ids', 'legacy_editorial_ids', 'legacy_editorial_mappings')
    ORDER BY c.relname
    \gexec
\endif

\if :harvest_db
    REVOKE ALL ON SCHEMA harvest FROM PUBLIC;
    REVOKE ALL ON ALL TABLES IN SCHEMA harvest FROM PUBLIC;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA harvest FROM PUBLIC;
    REVOKE EXECUTE ON ALL ROUTINES IN SCHEMA harvest FROM PUBLIC;
    GRANT USAGE ON SCHEMA harvest, public TO harvest_runtime;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA harvest TO harvest_runtime;
    GRANT EXECUTE ON ALL ROUTINES IN SCHEMA harvest TO harvest_runtime;
    GRANT SELECT ON public.goose_db_version TO harvest_runtime;
\endif

\if :identity_db
    GRANT CONNECT ON DATABASE identity TO identity_application_admin;
    GRANT USAGE ON SCHEMA public TO identity_runtime, identity_application_admin;
    GRANT SELECT, INSERT, UPDATE ON public.users, public.password_credentials, public.user_contacts,
        public.identity_sessions, public.app_sessions, public.refresh_tokens, public.authorization_codes,
        public.verification_challenges TO identity_runtime;
    GRANT SELECT, INSERT, UPDATE, DELETE ON public.security_notifications TO identity_runtime;
    GRANT SELECT, INSERT ON public.external_identities TO identity_runtime;
    GRANT UPDATE (unlinked_at) ON public.external_identities TO identity_runtime;
    GRANT SELECT ON public.identity_deployment, public.applications, public.application_redirect_uris,
        public.application_credentials TO identity_runtime;
    -- Row locking of an application requires UPDATE on at least one column.
    GRANT UPDATE (updated_at) ON public.applications TO identity_runtime;
    GRANT SELECT ON public.identity_deployment TO identity_application_admin;
    GRANT SELECT, INSERT, UPDATE ON public.applications, public.application_credentials,
        public.application_redirect_uris TO identity_application_admin;
    GRANT SELECT, UPDATE ON public.authorization_codes, public.app_sessions, public.refresh_tokens
        TO identity_application_admin;
\endif

COMMIT;
