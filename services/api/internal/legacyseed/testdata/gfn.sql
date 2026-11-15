CREATE TABLE gfn_site (
    id bigint PRIMARY KEY, name varchar(255), name_en varchar(255), info text, info_en text,
    create_time timestamp(0), update_time timestamp(0), nsfw varchar(4), icon varchar(255),
    deleted boolean, deleted_at timestamptz, deletion_reason_code text, deletion_reason text
);
CREATE TABLE gfn_site_group (
    id bigint PRIMARY KEY, name varchar(255), name_en varchar(255), info varchar(255), info_en varchar(255),
    priority bigint, create_time timestamp(0), update_time timestamp(0)
);
CREATE TABLE gfn_site_group_map (
    id bigint PRIMARY KEY, site_id bigint, group_id bigint, weight bigint, create_time timestamp(0), update_time timestamp(0)
);
CREATE TABLE gfn_collector_domain (
    id bigint PRIMARY KEY, name varchar(255), prefix varchar(255), tls varchar(4), site_id bigint, deleted boolean
);
CREATE TABLE gfn_target_tracking_periods (
    id bigint PRIMARY KEY, collector_domain_id bigint, site_id bigint, target text,
    tracked_from timestamptz, tracked_until timestamptz, tracking_basis text, opened_reason text, closed_reason text,
    created_at timestamptz, updated_at timestamptz
);
CREATE TABLE gfn_site_primary_target_periods (
    id bigint PRIMARY KEY, site_id bigint, target_tracking_period_id bigint, effective_from timestamptz,
    effective_until timestamptz, basis text, created_at timestamptz, updated_at timestamptz
);
CREATE TABLE gfn_featured_site (
    id bigint PRIMARY KEY, site_id bigint, weight bigint, create_time timestamp, update_time timestamp
);

INSERT INTO gfn_site VALUES
    (1,'示例网站','Example','中文 原文','Example description','2020-05-03 16:00:00','2020-05-03 16:00:00','0',NULL,false,NULL,NULL,NULL),
    (2,'历史站','','原始说明','','2021-05-03 16:00:00','2024-05-03 16:00:00','1',NULL,true,'2024-05-03 08:00:00Z','closed','synthetic-private-note'),
    (3,'空地址','No Domain','说明','Summary','2020-05-03','2020-05-03','',NULL,false,NULL,NULL,NULL),
    (4,'示例镜像','Example Mirror','说明','Summary','2020-05-03','2020-05-03','0',NULL,false,NULL,NULL,NULL),
    (5,'冲突','Conflict','说明','Summary','2020-05-03','2020-05-03','0',NULL,false,NULL,NULL,NULL),
    (6,'异常素材','Bad Logo','说明','Summary','2020-05-03','2020-05-03','0','nav/sites/6/icon/../../bad',false,NULL,NULL,NULL),
    (7,'超长资料','Long Info',repeat('x',10000),'Summary','2020-05-03','2020-05-03','0',NULL,false,NULL,NULL,NULL);
INSERT INTO gfn_site_group VALUES (1,'社区','Community','分组','A group',4,'2020-05-03','2020-05-03');
INSERT INTO gfn_site_group_map VALUES
    (1,1,1,17,'2020-05-03','2020-05-03'), (2,2,1,12,'2020-05-03','2020-05-03'), (3,999,1,0,'2020-05-03','2020-05-03');
INSERT INTO gfn_collector_domain VALUES
    (1,'example.com','www.','1',1,false),(2,'other.example.com',NULL,'0',1,false),
    (3,'mirror.example.net',NULL,'1',4,false),(4,'a.example.org',NULL,'1',5,false),
    (5,'b.example.org',NULL,'1',5,false),(6,'127.0.0.1',NULL,'1',7,false);
INSERT INTO gfn_target_tracking_periods VALUES
    (1,1,1,'www.example.com','2020-05-03 08:00:00Z',NULL,'explicit','configured',NULL,'2020-05-03','2020-05-03'),
    (2,NULL,1,'old.example.com','2019-05-03 08:00:00Z','2020-05-03 08:00:00Z','legacy_observed','history','cutover','2020-05-03','2020-05-03'),
    (3,5,5,'b.example.org','2020-05-03 08:00:00Z',NULL,'explicit','configured',NULL,'2020-05-03','2020-05-03'),
    (4,4,5,'a.example.org','2020-05-03 08:00:00Z',NULL,'explicit','configured',NULL,'2020-05-03','2020-05-03');
INSERT INTO gfn_site_primary_target_periods VALUES
    (1,1,1,'2020-05-03 08:00:00Z',NULL,'explicit','2020-05-03','2020-05-03'),
    (2,1,2,'2019-05-03 08:00:00Z','2020-05-03 08:00:00Z','legacy_backfill','2020-05-03','2020-05-03'),
    (3,5,3,'2020-05-03 08:00:00Z',NULL,'single_target_inferred','2020-05-03','2020-05-03'),
    (4,5,4,'2020-05-03 08:00:00Z',NULL,'explicit','2020-05-03','2020-05-03');
INSERT INTO gfn_featured_site VALUES (1,1,42,'2020-05-03','2020-05-03');
