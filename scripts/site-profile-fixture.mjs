export const profileID = '0000000000001001'

export function profileFixtureSQL(region) {
  if (region !== 'CN' && region !== 'INTL') throw new Error('Invalid profile fixture region')
  const fixtures = `
UPDATE sites SET secondary_categories=ARRAY['knowledge'],navigation_tags=ARRAY['forum','wiki'] WHERE public_id='${profileID}';
INSERT INTO site_addresses (id,site_id,kind,url)
 SELECT uuidv7(),id,'alternate','https://mirror.field-guide.example/browse?view=community' FROM sites WHERE public_id='${profileID}';
INSERT INTO site_addresses (id,site_id,kind,url)
 SELECT uuidv7(),id,'alternate','https://field-guide.example/private?token=PRIVATE-FIXTURE' FROM sites WHERE public_id='${profileID}';
`
  if (region === 'INTL') return fixtures
  return fixtures + `
UPDATE sites SET legacy_detail=true WHERE public_id='0000000000001027';
UPDATE site_localizations SET body='PRIVATE-LEGACY-BODY'
 WHERE site_id=(SELECT id FROM sites WHERE public_id='0000000000001027');
WITH target AS (
 SELECT s.id AS site_id,a.id AS address_id FROM sites s JOIN site_addresses a ON a.site_id=s.id AND a.kind='primary'
 WHERE s.public_id='${profileID}'
), src AS (
 INSERT INTO collection_sources(id,namespace,provider,resource_type,external_key,target_id)
 VALUES(uuidv7(),'profile-fixture','web','origin','https://field-guide.example',uuidv7()) RETURNING *
), bound AS (
 INSERT INTO collection_bindings(id,source_id,site_id,address_id,resource_type,purpose,effective_from)
 SELECT uuidv7(),src.id,t.site_id,t.address_id,'origin','primary',now()-interval '2 days' FROM target t CROSS JOIN src
 RETURNING *
), plan AS (
 INSERT INTO collection_plans(id,source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy)
 SELECT uuidv7(),id,'web.http',1,'{}','{"egress_region":"cn"}',86400,now(),'state_refresh' FROM src RETURNING *
), assigned AS (
 INSERT INTO collection_plan_bindings(plan_id,binding_id,source_id,attached_at)
 SELECT p.id,b.id,p.source_id,now()-interval '2 days' FROM plan p JOIN bound b USING(source_id)
), consumer AS (
 INSERT INTO collection_consumer(namespace,consumer_id,generation,epoch,cursor)
 VALUES('profile-fixture',uuidv7(),uuidv7(),uuidv7(),'profile-fixture-cursor') RETURNING *
), facts(section,facts,ruleset) AS (
 VALUES ('http','{"http.response":{"origin":"https://field-guide.example","status":200,"final_url":"https://other.example/?secret=PRIVATE-FIXTURE"}}'::jsonb,'web-passive-1'),
 ('html','{"tech.detected.astro":{"id":"astro","confidence":"high","evidence":["generator:astro"]},"rdap.registration":{"contact":"PRIVATE-FIXTURE"}}'::jsonb,'web-passive-1'),
 ('tls',jsonb_build_object('tls.certificate',jsonb_build_object('origin','https://field-guide.example','hostname','field-guide.example',
  'verified',true,'certificate_sha256',repeat('a',64),'spki_sha256',repeat('b',64),'not_before',now()-interval '10 days',
  'not_after',now()+interval '30 days','issuer','PRIVATE-FIXTURE')),'gofurry-evidence-1')
), current AS (
 INSERT INTO collection_current(source_id,generation,capability,output_schema,scope_key,scope,position,state,observation_id,
  observed_at,received_at,confirmed_at,content_hash,evidence,ruleset,data,facts)
 SELECT src.id,c.generation,'web.http',1,encode(sha256(convert_to(f.section,'UTF8')),'hex'),
  jsonb_build_object('namespace',c.namespace,'egress_region','cn','dimensions',jsonb_build_object('section',f.section)),
  1,'present',uuidv7(),now()-interval '1 day',now()-interval '1 day',now()-interval '2 hours',repeat('a',64),'',
  f.ruleset,'{}',f.facts
 FROM src CROSS JOIN consumer c CROSS JOIN facts f RETURNING *
)
INSERT INTO site_evidence_changes(id,site_id,binding_id,target_id,epoch,position,capability,output_schema,scope_key,scope,fact_key,
 event_kind,value,ruleset,prior_ruleset,source_observation_id,content_hash,evidence,first_observed_at,source_observed_at,received_at)
SELECT uuidv7(),b.site_id,b.id,src.target_id,consumer.epoch,c.position,c.capability,c.output_schema,c.scope_key,c.scope,
 'tech.detected.astro','baseline',c.facts->'tech.detected.astro',c.ruleset,'',c.observation_id,c.content_hash,c.evidence,c.observed_at,c.observed_at,c.received_at
FROM current c JOIN src ON src.id=c.source_id JOIN bound b USING (source_id) CROSS JOIN consumer
WHERE c.facts ? 'tech.detected.astro';
`
}
