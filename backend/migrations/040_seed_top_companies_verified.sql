-- +goose Up
-- Newly verified companies with correct ATS slugs (2026-04-27).
-- ON CONFLICT upserts: safe to replay on any environment, any number of times.

INSERT INTO companies
    (id, name, url, adapter_type, active)
VALUES
    ('character',     'Character.AI',           'https://jobs.ashbyhq.com/character',                                          'ashby',      true),
    ('cobot',         'Collaborative Robotics',  'https://jobs.ashbyhq.com/cobot',                                              'ashby',      true),
    ('perplexity',    'Perplexity AI',           'https://jobs.ashbyhq.com/perplexity',                                         'ashby',      true),
    ('dbtlabsinc',    'dbt Labs',                'https://boards-api.greenhouse.io/v1/boards/dbtlabsinc/jobs',                  'greenhouse', true),
    ('gleanwork',     'Glean',                   'https://boards-api.greenhouse.io/v1/boards/gleanwork/jobs',                   'greenhouse', true),
    ('gongio',        'Gong',                    'https://boards-api.greenhouse.io/v1/boards/gongio/jobs',                      'greenhouse', true),
    ('mercury',       'Mercury',                 'https://boards-api.greenhouse.io/v1/boards/mercury/jobs',                     'greenhouse', true),
    ('sourcegraph91', 'Sourcegraph',             'https://boards-api.greenhouse.io/v1/boards/sourcegraph91/jobs',               'greenhouse', true),
    ('wizinc',        'Wiz',                     'https://boards-api.greenhouse.io/v1/boards/wizinc/jobs',                      'greenhouse', true),
    ('flyzipline',    'Zipline',                 'https://boards-api.greenhouse.io/v1/boards/flyzipline/jobs',                  'greenhouse', true),
    ('mistral',       'Mistral AI',              'https://jobs.lever.co/mistral',                                               'lever',      true),
    ('teleport',      'Teleport',                'https://jobs.lever.co/teleport',                                              'lever',      true)
ON CONFLICT (id) DO UPDATE SET
    name         = EXCLUDED.name,
    url          = EXCLUDED.url,
    adapter_type = EXCLUDED.adapter_type,
    active       = EXCLUDED.active;

-- +goose Down
DELETE FROM companies WHERE id IN (
    'character', 'cobot', 'perplexity', 'dbtlabsinc', 'gleanwork', 'gongio',
    'mercury', 'sourcegraph91', 'wizinc', 'flyzipline', 'mistral', 'teleport'
);
