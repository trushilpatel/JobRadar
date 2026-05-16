-- +goose Up
-- Seed verified Deel-hosted (https://jobs.deel.com/<slug>) careers boards —
-- tech-only cut.
--
-- Generated 2026-05-15. Source list verified end-to-end against the live
-- jobs.deel.com RSC stream via the existing apps/crawler/ats/deel adapter:
-- every slug below returned >0 job records on its last probe. Boards that
-- rendered "Job Board Not Found" (e.g. EmployGlobalTalent) or returned 0
-- records (e.g. the GovAssist UUID slug) were excluded.
--
-- The 'deel' slug already exists in migration 038 (ashby) and is intentionally
-- NOT re-seeded here to preserve adapter history; admins can switch its
-- adapter via the admin UI if Deel has fully migrated to its own ATS.
--
-- Companies count: 21 (tech subset of the 41 publicly indexed
-- jobs.deel.com customer boards; non-tech/nonprofit/agency boards were
-- filtered out per JobRadar's audience focus).
--
-- Schema notes (post-044):
--   companies              (id, name, slug, website, careers_url, logo_url)
--   company_scan_settings  (company_id, adapter_type, active)
--   Adapter type 'deel' is registered in adapter_configs via migration 051.

INSERT INTO companies (id, name, slug, website, careers_url, logo_url) VALUES
    ('airbase', 'Airbase', 'airbase', 'https://jobs.deel.com/airbase', 'https://jobs.deel.com/airbase', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/b24b3d59-0b92-4e05-82db-d6671d0829dd/light_logo/XrJc__F36Qr68FjyNo-bj'),
    ('cardo', 'Cardo AI', 'cardo', 'https://jobs.deel.com/cardo', 'https://jobs.deel.com/cardo', 'https://media.letsdeel.com/avatar/logos/organizations/custom_branding/QElZ34wq-light?q=1762788759147'),
    ('cellpoint-digital', 'CellPoint Digital', 'cellpoint-digital', 'https://jobs.deel.com/cellpoint-digital', 'https://jobs.deel.com/cellpoint-digital', NULL),
    ('compliancequest', 'ComplianceQuest', 'compliancequest', 'https://jobs.deel.com/compliancequest', 'https://jobs.deel.com/compliancequest', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/6226ef56-ca5e-42d2-a247-936e1772c72e/light_logo/JhvNwF2Zl1nwMJUwkA_gD'),
    ('dfns', 'Dfns', 'dfns', 'https://jobs.deel.com/dfns', 'https://jobs.deel.com/dfns', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/890260e5-1530-48b3-8879-ebcf332893fa/light_logo/TBSABfbAsjuhwjozuks53'),
    ('domyn-spa', 'Domyn', 'domyn-spa', 'https://jobs.deel.com/domyn-spa', 'https://jobs.deel.com/domyn-spa', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/86b555d1-252a-482f-8e1f-ef1ac7e0707d/light_logo/8WLO1wb9KrF6sAp0fvUkX'),
    ('dott', 'dott', 'dott', 'https://jobs.deel.com/dott', 'https://jobs.deel.com/dott', 'https://download.deel.com/ats/career-page/light-logo/Hk0cab_-t9FcJ5QPWYFg3/BA4aOj8DkOc1s8fcdC4cl.png'),
    ('evident-insights', 'Evident Insights', 'evident-insights', 'https://jobs.deel.com/evident-insights', 'https://jobs.deel.com/evident-insights', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/dc1173ba-427c-48da-b276-b5223bcf44b5/light_logo/Y4k83DXHec9tgwUcPKI_5'),
    ('kangaroohealth', 'KangarooHealth', 'kangaroohealth', 'https://jobs.deel.com/kangaroohealth', 'https://jobs.deel.com/kangaroohealth', NULL),
    ('keyway-ai', 'Keyway.ai', 'keyway-ai', 'https://jobs.deel.com/keyway-ai', 'https://jobs.deel.com/keyway-ai', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/23e10ff1-a890-43aa-b109-0038f7fd1248/banner_image/AwM7yUAwO4iqNcZvu8JCd'),
    ('klarna', 'Klarna', 'klarna', 'https://jobs.deel.com/klarna', 'https://jobs.deel.com/klarna', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/7e2ccf5c-ce28-451a-adf0-625015848270/light_logo/C4bjpIXSOrrm5d38Vk7fp'),
    ('mako', 'MAKO', 'mako', 'https://jobs.deel.com/mako', 'https://jobs.deel.com/mako', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/a4a15973-4987-4863-9e10-b7b622c8a8a3/light_logo/0ICJ5Wa91OOaW9FY7R1mI'),
    ('mattilda', 'Mattilda', 'mattilda', 'https://jobs.deel.com/mattilda', 'https://jobs.deel.com/mattilda', 'https://media.letsdeel.com/avatar/logos/organizations/custom_branding/ajxeOKyE-light?q=1762790694439'),
    ('metarouter', 'MetaRouter', 'metarouter', 'https://jobs.deel.com/metarouter', 'https://jobs.deel.com/metarouter', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/7b882c67-b129-4f7f-ae6d-cb859b431e90/light_logo/YMz9D0xT47sHrZJ-zqhHF'),
    ('offsec', 'OffSec', 'offsec', 'https://jobs.deel.com/offsec', 'https://jobs.deel.com/offsec', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/a2a48ffa-7b52-4749-90fe-dd72f2363f63/light_logo/nnGRlA-6zFxSptbtNa6Ll'),
    ('qubitra', 'Qubitra', 'qubitra', 'https://jobs.deel.com/qubitra', 'https://jobs.deel.com/qubitra', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/b9ec925c-a3aa-4271-841f-3f372cb9b248/light_logo/EJ789A1363PrRy_a98yL_'),
    ('sendfx', 'SendFX', 'sendfx', 'https://jobs.deel.com/sendfx', 'https://jobs.deel.com/sendfx', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/2501dc1b-5446-446e-b19c-772cc037fa4a/light_logo/WKhXwPKOltZYiAqIBNjTw'),
    ('sequence', 'Sequence', 'sequence', 'https://jobs.deel.com/sequence', 'https://jobs.deel.com/sequence', 'https://www.sequencehq.com/favicon.png'),
    ('shufti', 'Shufti', 'shufti', 'https://jobs.deel.com/shufti', 'https://jobs.deel.com/shufti', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/4b3cbdb5-5f51-4fd6-a533-053744c68afc/banner_image/0W9labhOOr58LccTmUq5A'),
    ('zema.global', 'Zema Global', 'zema.global', 'https://jobs.deel.com/zema.global', 'https://jobs.deel.com/zema.global', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/bbf5c5e2-ab26-47a9-aa75-ad9bfefcf76c/light_logo/4dpSrmq4VvefQ9oXTtTby'),
    ('zonos', 'Zonos', 'zonos', 'https://jobs.deel.com/zonos', 'https://jobs.deel.com/zonos', 'https://s3.eu-west-1.amazonaws.com/api-prod.letsdeel.com/public/ats/aa322749-cf17-4c7a-b7f3-4492d24bf0e8/light_logo/YA-9NtfLrdxU9YcM8BtQc')
ON CONFLICT (id) DO UPDATE SET
    name        = EXCLUDED.name,
    slug        = EXCLUDED.slug,
    website     = EXCLUDED.website,
    careers_url = EXCLUDED.careers_url,
    logo_url    = COALESCE(EXCLUDED.logo_url, companies.logo_url),
    updated_at  = NOW();

INSERT INTO company_scan_settings (company_id, adapter_type, active) VALUES
    ('airbase', 'deel', true),
    ('cardo', 'deel', true),
    ('cellpoint-digital', 'deel', true),
    ('compliancequest', 'deel', true),
    ('dfns', 'deel', true),
    ('domyn-spa', 'deel', true),
    ('dott', 'deel', true),
    ('evident-insights', 'deel', true),
    ('kangaroohealth', 'deel', true),
    ('keyway-ai', 'deel', true),
    ('klarna', 'deel', true),
    ('mako', 'deel', true),
    ('mattilda', 'deel', true),
    ('metarouter', 'deel', true),
    ('offsec', 'deel', true),
    ('qubitra', 'deel', true),
    ('sendfx', 'deel', true),
    ('sequence', 'deel', true),
    ('shufti', 'deel', true),
    ('zema.global', 'deel', true),
    ('zonos', 'deel', true)
ON CONFLICT (company_id) DO UPDATE SET
    adapter_type = EXCLUDED.adapter_type,
    active       = EXCLUDED.active;

-- +goose Down
DELETE FROM company_scan_settings WHERE company_id IN (
    'airbase', 'cardo', 'cellpoint-digital', 'compliancequest', 'dfns', 'domyn-spa', 'dott', 'evident-insights', 'kangaroohealth', 'keyway-ai', 'klarna', 'mako', 'mattilda', 'metarouter', 'offsec', 'qubitra', 'sendfx', 'sequence', 'shufti', 'zema.global', 'zonos'
);
DELETE FROM companies WHERE id IN (
    'airbase', 'cardo', 'cellpoint-digital', 'compliancequest', 'dfns', 'domyn-spa', 'dott', 'evident-insights', 'kangaroohealth', 'keyway-ai', 'klarna', 'mako', 'mattilda', 'metarouter', 'offsec', 'qubitra', 'sendfx', 'sequence', 'shufti', 'zema.global', 'zonos'
);
