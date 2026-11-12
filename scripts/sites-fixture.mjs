export function sitesFixtureSQL(region, logo, cover) {
  if (!['CN', 'INTL'].includes(region) || !/^sites\/logos\/[a-f0-9]{64}\.webp$/.test(logo) || !/^sites\/covers\/[a-f0-9]{64}\.avif$/.test(cover)) throw new Error('Invalid test assets')
  return `
UPDATE sites SET status='archived',archived_at=now() WHERE status='published';
WITH fixture AS MATERIALIZED (
 SELECT i,uuidv7() id,'0000000000001'||lpad(to_hex(i),3,'0') AS public_id FROM generate_series(1,40) i
), sites_written AS (
 INSERT INTO sites(id,public_id,slug,status,published_at,first_published_at,archived_at,content_rating,primary_category,secondary_categories,
   navigation_tags,supported_languages,logo_asset_key,featured_cover_asset_key)
 SELECT id,public_id,'field-guide-'||i,CASE WHEN i=40 THEN 'archived' ELSE 'published' END,
  now()-i*interval '1 day',now()-i*interval '1 day',CASE WHEN i=40 THEN now() END,
  CASE WHEN i=39 THEN '${region === 'CN' ? 'unreviewed' : 'adult'}' ELSE 'sfw' END,
  (ARRAY['community','gallery','literature','resources','marketplace','knowledge','gaming','events'])[1+(i-1)%8],
  CASE WHEN i%8=0 THEN ARRAY['community'] ELSE '{}'::text[] END,
  CASE WHEN i%2=0 THEN ARRAY['portfolio','tutorial'] ELSE ARRAY['forum'] END,
  CASE WHEN i%3=0 THEN ARRAY['ja'] WHEN i%3=1 THEN ARRAY['en','zh'] ELSE ARRAY['en'] END,
  CASE WHEN i IN (1,4,13,14,15) THEN '${logo}' END,CASE WHEN i=1 THEN '${cover}' END
 FROM fixture
), names AS (
 INSERT INTO site_localizations
 SELECT id,'en',CASE i WHEN 1 THEN '${region === 'CN' ? 'Pinewood Commons' : 'Cedar Commons'}' WHEN 2 THEN 'Lantern Sketchbook'
  WHEN 3 THEN 'Moontrail Stories' WHEN 39 THEN 'Restricted specimen' WHEN 40 THEN 'Archived specimen' ELSE 'Field Guide '||lpad(i::text,2,'0') END,
  CASE i WHEN 1 THEN 'A gathering place for furry makers, illustrators and friends, with community projects and regular events.'
  WHEN 2 THEN 'An independent collection of illustration and character design.'
  WHEN 3 THEN 'Short fiction and longer journeys written by the community.'
  ELSE 'A furry web index fixture for browsing, local projects and practical guides.' END
 FROM fixture
 UNION ALL
 SELECT id,'zh',CASE i WHEN 1 THEN '松林兽圈' WHEN 2 THEN '灯笼画册' WHEN 3 THEN '月径故事' ELSE '原野手册 '||lpad(i::text,2,'0') END,
  CASE i WHEN 1 THEN '兽圈创作者和朋友们的交流空间，分享作品、协作项目与线下活动。'
  WHEN 2 THEN '收录插画、角色设计和创作过程的独立画册。'
  WHEN 3 THEN '从短篇小说到长篇旅途，阅读同好们写下的故事。'
  ELSE '面向兽圈社区的网站索引，收录本地项目和实用指南。' END
 FROM fixture WHERE i<>2
), addresses AS (
 INSERT INTO site_addresses(id,site_id,kind,url)
 SELECT uuidv7(),id,'primary',CASE i WHEN 16 THEN 'https://user:fixture@unsafe.example/'
  ELSE 'https://field-guide.example/entry/'||i||'?view=guide' END FROM fixture
), sample AS (
 INSERT INTO site_interaction_days(site_id,utc_day,detail_views,outbound_clicks)
 SELECT id,(now() AT TIME ZONE 'UTC')::date-d,40-i,2 FROM fixture CROSS JOIN generate_series(0,1) d
 WHERE i=1 OR i BETWEEN 4 AND 11
)
UPDATE site_navigation_features slot
SET site_id=f.id,revision=slot.revision+1,actor='fixture',reason='D2-B reviewed visual evidence',updated_at=now()
FROM fixture f WHERE (slot.slot,f.i) IN (('primary',1),('secondary_1',2),('secondary_2',3));
`
}
