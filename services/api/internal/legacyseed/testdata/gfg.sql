CREATE TABLE gfg_game (
    id bigint PRIMARY KEY, name varchar(255), name_en varchar(255), info text, info_en text,
    create_time timestamp(0), update_time timestamp(0), resources jsonb, groups jsonb, release_date varchar(255),
    developers jsonb, publishers jsonb, appid bigint, links jsonb, weight bigint, showcase_eligible boolean
);
CREATE TABLE gfg_tag_category (
    id bigint PRIMARY KEY, code text, name text, name_en text, info text, info_en text,
    sort_order integer, archived_at timestamp(0), create_time timestamp(0), update_time timestamp(0)
);
CREATE TABLE gfg_tag (
    id bigint PRIMARY KEY, code text, name text, name_en text, info text, info_en text,
    category_id bigint, archived_at timestamp(0), create_time timestamp(0), update_time timestamp(0)
);
CREATE TABLE gfg_game_tag (
    game_id bigint, tag_id bigint, role text, create_time timestamp(0), update_time timestamp(0),
    PRIMARY KEY (game_id,tag_id)
);
CREATE TABLE gfg_game_collection (
    id bigint PRIMARY KEY, code text, name text, name_en text, info text, info_en text,
    status text, version bigint, published_at timestamptz, archived_at timestamptz,
    created_at timestamptz, updated_at timestamptz
);
CREATE TABLE gfg_game_collection_item (
    collection_id bigint, game_id bigint, created_at timestamptz, updated_at timestamptz,
    PRIMARY KEY (collection_id,game_id)
);
CREATE TABLE gfg_game_collection_tag (
    collection_id bigint, tag_id bigint, created_at timestamptz, PRIMARY KEY (collection_id,tag_id)
);
CREATE TABLE gfg_game_collection_exclusion (
    collection_id bigint, game_id bigint, created_at timestamptz, PRIMARY KEY (collection_id,game_id)
);
CREATE TABLE gfg_game_collection_home_slot (
    slot smallint PRIMARY KEY, collection_id bigint, created_at timestamptz, updated_at timestamptz
);
CREATE TABLE gfg_game_first_available (
    game_id bigint PRIMARY KEY, precision text, exact_date date, release_year integer,
    release_month integer, release_quarter integer, window_start date, window_end date,
    source text, inferred boolean, source_raw text, source_observed_at timestamptz, normalizer_version text,
    established_at timestamptz, updated_at timestamptz
);
CREATE TABLE gfg_game_release_state (
    game_id bigint PRIMARY KEY, availability text, precision text, exact_date date, release_year integer,
    release_month integer, release_quarter integer, window_start date, window_end date, raw_text text,
    source text, source_region text, source_locale text, normalizer_version text,
    observed_at timestamptz, updated_at timestamptz
);
INSERT INTO gfg_game VALUES
    (1,'狐狸游戏','Fox Game','中文','English','2020-04-03 08:00:00','2022-05-05','[{"key":"Guide","value":"https://example.com/guide"}]',
    '[{"key":"Unsafe","value":"https://example.com/?token=fixture-private"}]','2020 年 6 月','["Dev A"]','["Publisher A"]',20,NULL,3,true),
    (2,'另一游戏','','中文','','2020-05-03','2020-05-03',NULL,NULL,'','{}','["Publisher B"]',20,NULL,4,false),
    (3,'独立游戏','Indie','中文','English','2020-05-03','2020-05-03',NULL,NULL,'','[]','[]',0,'[{"key":"Store","value":"https://indie.example.net"}]',1,false),
    (4,'唯一来源','Unique','中文','English','2022-01-03','2022-01-03',NULL,NULL,'','[]','[]',100,NULL,1,false);
INSERT INTO gfg_tag_category VALUES (1,'genre','类型','Genre','分类','Category',1,NULL,'2020-05-03','2020-05-03'),(2,'archive','归档','Archive','归档','Archived',2,'2021-05-03','2020-05-03','2021-05-03');
INSERT INTO gfg_tag VALUES
    (1,'adventure','冒险','Adventure','分类','Category',1,NULL,'2020-05-03','2020-05-03'),
    (2,'adult','成人','Adult','成人','Adult',2,'2021-05-03','2020-05-03','2021-05-03'),
    (3,'adventure','另一个冒险','Adventure','分类','Category',1,NULL,'2020-05-03','2020-05-03');
INSERT INTO gfg_game_tag VALUES
    (1,1,'primary','2020-05-03','2020-05-03'), (2,2,'secondary','2020-05-03','2020-05-03'),
    (3,1,'normal','2020-05-03','2020-05-03'), (999,2,'normal','2020-05-03','2020-05-03');
INSERT INTO gfg_game_collection VALUES
    (1,'curated','精选','Curated','介绍','Intro','published',3,'2025-01-02 03:00Z',NULL,'2024-01-02','2025-01-02'),
    (2,'archive','归档','Archive','介绍','Intro','archived',4,'2024-01-02','2025-01-02','2023-01-02','2025-01-02');
INSERT INTO gfg_game_collection_item VALUES (1,1,'2024-01-02','2024-01-02'), (1,2,'2024-01-02','2024-01-02');
INSERT INTO gfg_game_collection_tag VALUES (1,1,'2024-01-02'), (1,2,'2024-01-02');
INSERT INTO gfg_game_collection_exclusion VALUES (1,1,'2024-01-02'),(1,3,'2024-01-02');
INSERT INTO gfg_game_collection_home_slot VALUES (5,1,'2024-01-02','2024-01-02');
INSERT INTO gfg_game_first_available VALUES
    (1,'month',NULL,2020,6,NULL,'2020-06-01','2020-06-30','legacy_manual',false,'June 2020',NULL,'v1','2025-01-02','2025-01-02'),
    (2,'day','2019-07-20',2019,7,NULL,'2019-07-20','2019-07-20','steam_backfill',true,'ignored steam text','2022-01-01','v1','2025-01-02','2025-01-02');
INSERT INTO gfg_game_release_state (game_id,source,raw_text) VALUES (2,'steam_storefront','ignored release text');
