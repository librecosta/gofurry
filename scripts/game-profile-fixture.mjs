import { coverHost } from './game-channel-fixture.mjs'

export const gameProfileFixtureSQL = `
UPDATE game_source_metadata m SET payload=payload||'{"description":"The foxes chart a forgotten coastline.\\nTheir atlas keeps the ports connected.","publishers":["Pine Press"],"supported_languages":"English · 简体中文"}'
FROM game_sources s WHERE s.id=m.game_source_id AND s.external_key='3990000000' AND m.section='details';
UPDATE game_source_metadata m SET payload=payload||jsonb_build_object(
 'header','https://${coverHost}/store_item_assets/steam/apps/3990000000/header.jpg','screenshots',jsonb_build_array('https://${coverHost}/fixture/harbor.jpg','https://${coverHost}/fixture/cove.jpg','https://untrusted.example/fixture.jpg'),
 'movies',jsonb_build_array('https://${coverHost}/fixture/trailer.mp4','https://${coverHost}/fixture/script.html'))
FROM game_sources s WHERE s.id=m.game_source_id AND s.external_key='3990000000' AND m.section='media';

INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url,active)
SELECT uuidv7(),game_id,'steam','3990000040','https://store.steampowered.com/app/3990000040/',false
FROM game_sources WHERE external_key='3990000000';

INSERT INTO collection_sources(id,namespace,provider,resource_type,external_key,target_id)
VALUES (uuidv7(),'local-game-profile-fixture','steam','app','3990000000',uuidv7());

INSERT INTO collection_bindings(id,source_id,resource_type,game_id,game_source_id,purpose,effective_from)
SELECT uuidv7(),c.id,'app',s.game_id,s.id,'steam',now()-interval '20 days'
FROM game_sources s JOIN collection_sources c ON s.external_key=c.external_key WHERE c.namespace='local-game-profile-fixture';

INSERT INTO collection_plans(id,source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy)
SELECT uuidv7(),id,'steam.news',1,'{}','{"egress_region":"cn"}',86400,now(),'state_refresh'
FROM collection_sources WHERE namespace='local-game-profile-fixture';

INSERT INTO collection_plan_bindings(plan_id,binding_id,source_id,attached_at)
SELECT p.id,b.id,p.source_id,now()-interval '20 days' FROM collection_plans p
JOIN collection_bindings b ON b.source_id=p.source_id WHERE p.capability='steam.news';

INSERT INTO game_evidence_news
SELECT s.id,game_insight_scope_hash(s.namespace,'cn','news'),(9000+n)::text,
 now()-make_interval(hours=>n+1),now()-interval '5 minutes',uuidv7(),jsonb_build_object(
  'title','Harbour dispatch '||n,'url',CASE WHEN n=0 THEN 'https://steamcommunity.com.evil.invalid/announcements/detail/9000'
    ELSE 'https://steamcommunity.com/games/3990000000/announcements/detail/'||(9000+n)::text END)
FROM collection_sources s CROSS JOIN generate_series(0,8) n WHERE s.namespace='local-game-profile-fixture';

INSERT INTO game_player_daily
SELECT s.id,'cn',(now() AT TIME ZONE 'UTC')::date-n,jsonb_build_object(
 '0',jsonb_build_array(CASE n WHEN 3 THEN 23 WHEN 2 THEN 14 ELSE 0 END,now()-make_interval(days=>n),uuidv7(),uuidv7()),
 '1',jsonb_build_array(CASE n WHEN 3 THEN 17 WHEN 2 THEN 18 ELSE 0 END,now()-make_interval(days=>n),uuidv7(),uuidv7()))
FROM game_sources s CROSS JOIN (VALUES (0),(2),(3)) days(n) WHERE s.external_key='3990000000';
INSERT INTO game_player_daily
SELECT s.id,'cn',(now() AT TIME ZONE 'UTC')::date,jsonb_build_object('0',jsonb_build_array(
 7,now()-interval '1 second',uuidv7(),uuidv7())) FROM game_sources s WHERE s.external_key='3990000040';

INSERT INTO game_price_observations
SELECT s.id,b.id,uuidv7(),'cn',country,currency,locale,(now() AT TIME ZONE 'UTC')::date-n,now()-make_interval(days=>n),
 state,initial,final,discount FROM game_sources s JOIN collection_bindings b ON b.game_source_id=s.id
 CROSS JOIN (VALUES ('cn','CNY','zh-cn',0,'free',NULL::bigint,NULL::bigint,NULL::int),
  ('us','USD','en',3,'priced',1500,1500,0),('us','USD','en',2,'priced',1500,1500,0),('us','USD','en',0,'zero_price',1500,0,100),
  ('hk','HKD','en',0,'priced',8000,8000,0)) fixture(country,currency,locale,n,state,initial,final,discount)
 WHERE s.external_key='3990000000';
INSERT INTO game_published_localizations (game_id,locale,name,summary,media_source_id,media,media_observed_at,media_store_country,cover_url,revision)
SELECT g.id,l.locale,l.name,CASE WHEN s.external_key='3990000000' AND l.locale='en' THEN
 l.summary || E'\nThe foxes chart a forgotten coastline. Their atlas keeps the ports connected.' ELSE l.summary END,
 s.id,coalesce(m.payload,'{}'),m.observed_at,coalesce(m.store_country,''),coalesce(m.payload->>'header',''),g.revision
FROM games g JOIN game_localizations l ON l.game_id=g.id
LEFT JOIN game_sources s ON s.game_id=g.id AND s.active AND s.platform_code='steam'
LEFT JOIN game_source_metadata m ON m.game_source_id=s.id AND m.section='media'
WHERE g.status='published' AND g.public_id LIKE '2%';
`
