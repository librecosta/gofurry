// This data is only loaded by check-catalog into its disposable PostgreSQL.
export const collectionGame = '2000000000000000'
export const memberTwo = '2000000000000001'
export const coverHost = 'cdn.cloudflare.steamstatic.com'

export const gamesFixtureSQL = (region) => {
  const title = region === 'CN' ? 'Fox Atlas' : 'Fox Atlas International'
  return `
WITH rows AS (
 INSERT INTO games (id,public_id,slug,status,published_at,first_published_at,publication_blocked)
 SELECT uuidv7(),'2'||lpad(n::text,15,'0'),'game-'||n,
   CASE WHEN n=30 THEN 'draft' ELSE 'published' END,
   CASE WHEN n<>30 THEN statement_timestamp()-make_interval(hours=>n) END,
   CASE WHEN n<>30 THEN statement_timestamp()-make_interval(hours=>n) END,n=29
 FROM generate_series(0,30) n RETURNING id,public_id
)
INSERT INTO game_localizations (game_id,locale,name,summary)
SELECT id,'en',CASE right(public_id,2)::int
 WHEN 0 THEN '${title}' WHEN 1 THEN 'Fox Signal' WHEN 2 THEN 'Fox Cartographer'
 WHEN 29 THEN 'Blocked Fox' WHEN 30 THEN 'Draft Fox' ELSE 'Fox Record '||right(public_id,2) END,
 'An anthropomorphic field guide.' FROM rows;

INSERT INTO game_localizations SELECT id,'zh','狐狸图册','双语合集的第一款游戏。' FROM games WHERE public_id='${collectionGame}';
INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url)
SELECT uuidv7(),id,'steam',(3990000000+right(public_id,2)::bigint)::text,
 'https://store.steampowered.com/app/'||(3990000000+right(public_id,2)::bigint)||'/'
FROM games WHERE public_id IN ('${collectionGame}','${memberTwo}','2000000000000002','2000000000000029');
INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url)
SELECT uuidv7(),id,'itch_io','fixture/fox-atlas','https://fixture.itch.io/fox-atlas' FROM games WHERE public_id='${collectionGame}';

INSERT INTO game_source_metadata
 (game_source_id,locale,store_country,egress_region,section,binding_id,scope_key,epoch,position,event_at,state,output_schema,
 ruleset,observation_id,content_hash,observed_at,confirmed_at,payload)
SELECT s.id,'en','us','cn','details',uuidv7(),repeat('0',64),uuidv7(),1,now(),'present',1,'gofurry-steam-1',
 uuidv7(),repeat('0',64),now(),now(),jsonb_build_object('name','Fixture details',
 'genres',jsonb_build_array(jsonb_build_object('id',CASE WHEN s.external_key='3990000001' THEN '3' ELSE '25' END,'name','Untrusted label')),
 'platforms','{"linux":true,"windows":false}'::jsonb,'developers',jsonb_build_array('Burrow Works'),
 'release',jsonb_build_object('coming_soon',s.external_key IN ('3990000001','3990000002'),'date_text',
 CASE s.external_key WHEN '3990000000' THEN (current_date-10)::text WHEN '3990000001' THEN 'Jan 2, 2021'
 WHEN '3990000002' THEN 'Q3 2027' ELSE '2026-02-31' END))
FROM game_sources s WHERE s.platform_code='steam' AND s.external_key LIKE '39900000%';

INSERT INTO game_source_metadata
 (game_source_id,locale,store_country,egress_region,section,binding_id,scope_key,epoch,position,event_at,state,output_schema,
 ruleset,observation_id,content_hash,observed_at,confirmed_at,payload)
SELECT s.id,'en','us','cn','media',uuidv7(),repeat('1',64),uuidv7(),1,now(),'present',1,'gofurry-steam-1',
 uuidv7(),repeat('1',64),now(),now(),jsonb_build_object('header',
 CASE WHEN external_key='3990000000' THEN 'https://${coverHost}/store_item_assets/steam/apps/3990000000/header.jpg'
 ELSE 'https://media.steamstatic.com.evil.invalid/header.jpg' END,'screenshots',jsonb_build_array('unused'))
FROM game_sources s WHERE external_key IN ('3990000000','3990000001');

UPDATE game_source_metadata SET payload='{"screenshots":["unused"]}' WHERE section='media' AND game_source_id IN (
 SELECT id FROM game_sources WHERE external_key='3990000000');
`
}
