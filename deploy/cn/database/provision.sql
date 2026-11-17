\set ON_ERROR_STOP on
\if :{?confirm_cn}
\else
  \set confirm_cn ''
\endif
SELECT :'confirm_cn' = 'librecosta-cn' AS cn_confirmed \gset
\if :cn_confirmed
\else
  DO $$ BEGIN RAISE EXCEPTION 'Pass -v confirm_cn=librecosta-cn after checking the target cluster'; END $$;
\endif

DO $$
BEGIN
    IF current_database() <> 'postgres'
       OR current_setting('server_version_num')::integer / 10000 <> 18
       OR NOT (SELECT rolsuper FROM pg_roles WHERE rolname = current_user) THEN
        RAISE EXCEPTION 'Connect to the PG18 postgres maintenance database as the cluster administrator';
    END IF;
END
$$;

SELECT pg_advisory_lock(hashtextextended('librecosta-cn-ops-s2-c', 0));

CREATE TEMP TABLE cn_databases AS
SELECT * FROM (VALUES
    ('librecosta_admin_cn', 'librecosta_admin_migrator', 'librecosta_admin_runtime'),
    ('gateway',             'gateway_owner',             'gateway_runtime'),
    ('gofurry_cn',          'gofurry_cn_migrator',        'gofurry_cn_runtime'),
    ('harvest',             'harvest_migrator',           'harvest_runtime'),
    ('identity',            'identity_migrator',          'identity_runtime')
) AS planned(db, migrator, runtime);

CREATE TEMP TABLE cn_roles AS
SELECT migrator AS role_name, true AS login FROM cn_databases
UNION ALL SELECT runtime, true FROM cn_databases
UNION ALL SELECT 'identity_app_operator', true
UNION ALL SELECT 'identity_application_admin', false;

DO $$
DECLARE
    collision text;
BEGIN
    SELECT COALESCE(
        (SELECT r.rolname
         FROM cn_roles p JOIN pg_roles r ON r.rolname = p.role_name
         WHERE shobj_description(r.oid, 'pg_authid') IS DISTINCT FROM 'librecosta-cn-ops-s2-c/v1'
            OR r.rolcanlogin <> p.login OR NOT r.rolinherit OR r.rolsuper
            OR r.rolcreaterole OR r.rolcreatedb OR r.rolreplication OR r.rolbypassrls
         LIMIT 1),
        (SELECT d.datname
         FROM cn_databases p JOIN pg_database d ON d.datname = p.db
         WHERE (shobj_description(d.oid, 'pg_database') IS DISTINCT FROM 'librecosta-cn-ops-s2-c/v1'
                AND (shobj_description(d.oid, 'pg_database') IS DISTINCT FROM 'librecosta-cn-ops-s2-c/v1-pending'
                     OR d.datallowconn))
            OR pg_get_userbyid(d.datdba) <> p.migrator OR d.datistemplate
            OR pg_encoding_to_char(d.encoding) <> 'UTF8'
         LIMIT 1),
        (SELECT member.rolname
         FROM pg_auth_members m
         JOIN pg_roles granted ON granted.oid = m.roleid
         JOIN pg_roles member ON member.oid = m.member
         WHERE (granted.rolname IN (SELECT role_name FROM cn_roles)
                OR member.rolname IN (SELECT role_name FROM cn_roles))
           AND NOT (granted.rolname = 'identity_application_admin'
                    AND member.rolname = 'identity_app_operator'
                    AND m.inherit_option AND NOT m.admin_option AND NOT m.set_option)
         LIMIT 1)) INTO collision;
    IF collision IS NOT NULL THEN
        RAISE EXCEPTION 'Existing database, role or membership needs manual review: %', collision;
    END IF;
END
$$;

BEGIN;
SELECT format('CREATE ROLE %I %s INHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',
              p.role_name, CASE WHEN p.login THEN 'LOGIN' ELSE 'NOLOGIN' END),
       format('COMMENT ON ROLE %I IS %L', p.role_name, 'librecosta-cn-ops-s2-c/v1')
FROM cn_roles p
WHERE NOT EXISTS (SELECT 1 FROM pg_roles r WHERE r.rolname = p.role_name)
ORDER BY p.role_name
\gexec
GRANT identity_application_admin TO identity_app_operator WITH INHERIT TRUE, SET FALSE, ADMIN FALSE;
COMMIT;

-- CREATE DATABASE runs outside a transaction. A new DB starts closed.
SELECT format('CREATE DATABASE %I OWNER %I TEMPLATE template0 ENCODING %L ALLOW_CONNECTIONS false',
              p.db, p.migrator, 'UTF8'),
       format('COMMENT ON DATABASE %I IS %L', p.db, 'librecosta-cn-ops-s2-c/v1-pending')
FROM cn_databases p
WHERE NOT EXISTS (SELECT 1 FROM pg_database d WHERE d.datname = p.db)
ORDER BY p.db
\gexec

BEGIN;
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', db)
FROM cn_databases ORDER BY db
\gexec

-- Only open databases that were staged by this script. Keep closures made by operators.
SELECT format('ALTER DATABASE %I ALLOW_CONNECTIONS true', p.db),
       format('COMMENT ON DATABASE %I IS %L', p.db, 'librecosta-cn-ops-s2-c/v1')
FROM cn_databases p JOIN pg_database d ON d.datname = p.db
WHERE shobj_description(d.oid, 'pg_database') = 'librecosta-cn-ops-s2-c/v1-pending'
ORDER BY p.db
\gexec
COMMIT;
SELECT pg_advisory_unlock(hashtextextended('librecosta-cn-ops-s2-c', 0));
