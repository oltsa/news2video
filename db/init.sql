-- =============== SCHEMA ===============
-- Use gen_random_uuid from pgcrypto, which is more standard than uuid-ossp
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS plans (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    daily_video_limit INT NOT NULL DEFAULT 10
);

CREATE TABLE IF NOT EXISTS features (
    id SERIAL PRIMARY KEY,
    feature_key VARCHAR(100) UNIQUE NOT NULL,
    description TEXT
);

CREATE TABLE IF NOT EXISTS plan_features (
    plan_id INT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    feature_id INT NOT NULL REFERENCES features(id) ON DELETE CASCADE,
    PRIMARY KEY (plan_id, feature_id)
);

CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_key VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_key VARCHAR(255) NOT NULL,
    api_key UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    plan_id INT NOT NULL REFERENCES plans(id),
    jobs_today INT NOT NULL DEFAULT 0,
    jobs_last_reset_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, project_key)
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    email VARCHAR(255) UNIQUE NOT NULL,
    hashed_password TEXT NOT NULL,
    is_customer_admin BOOLEAN DEFAULT FALSE,
    is_system_admin BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS roles (
    id INT PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS user_project_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role_id INT NOT NULL REFERENCES roles(id),
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE IF NOT EXISTS templates (
    template_uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    id VARCHAR(255) NOT NULL,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    template_data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS templates_project_and_id_idx ON templates (project_id, id) WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS templates_id_idx ON templates (id) WHERE project_id IS NULL;

CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    user_id UUID REFERENCES users(id),
    template_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    output_storage_key TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    render_duration_ms INT NULL
);

CREATE TABLE IF NOT EXISTS connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    platform TEXT NOT NULL,
    config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One connection per (project, platform): s3 / youtube / tiktok / webhook per project
ALTER TABLE connections
    ADD CONSTRAINT connections_project_platform_unique
    UNIQUE (project_id, platform);

-- =============== SEED DATA ===============

INSERT INTO plans (id, name, daily_video_limit)
VALUES
(1, 'Basic', 20),
(2, 'Pro', 200)
ON CONFLICT(id) DO NOTHING;

INSERT INTO features (id, feature_key, description)
VALUES
(1, 'CAN_ADD_TEMPLATES', 'Allows users to upload their own templates.')
ON CONFLICT(id) DO NOTHING;

INSERT INTO plan_features (plan_id, feature_id)
VALUES (2, 1)
ON CONFLICT(plan_id, feature_id) DO NOTHING;

-- Demo organization (no longer has a plan_id)
INSERT INTO organizations (id, organization_key)
VALUES ('10000000-0000-0000-0000-000000000001', 'demo_organization')
ON CONFLICT(id) DO NOTHING;

-- Projects (now include a plan_id)
INSERT INTO projects (id, organization_id, project_key, api_key, plan_id)
VALUES
('20000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000001', 'project_a', '11111111-1111-1111-1111-111111111111', 2),
('20000000-0000-0000-0000-000000000002', '10000000-0000-0000-0000-000000000001', 'project_b', '22222222-2222-2222-2222-222222222222', 1)
ON CONFLICT(id) DO NOTHING;

-- Seed a default S3 connection for 'project_a'
-- This is safe because of the UNIQUE(project_id, platform) constraint.
INSERT INTO connections (project_id, platform, config)
VALUES ('20000000-0000-0000-0000-000000000001', 's3', '{}')
ON CONFLICT DO NOTHING;

-- Seed roles for RBAC
INSERT INTO roles (id, name)
VALUES (1, 'Editor'), (2, 'Viewer')
ON CONFLICT(id) DO NOTHING;

-- NOTE: All user seeding has been removed. Users should now be managed via the admin-cli.

-- =============== TEMPLATE LOADER ===============
-- This function reads a JSON file from the mounted templates directory and inserts it.
CREATE OR REPLACE PROCEDURE insert_template_from_file(
    p_id VARCHAR(255),
    p_project_id UUID,
    p_filename VARCHAR(255)
)
LANGUAGE plpgsql
AS $$
DECLARE
    file_content TEXT;
BEGIN
    file_content := pg_read_file('/docker-entrypoint-initdb.d/templates/' || p_filename);
    INSERT INTO templates (id, project_id, template_data)
    VALUES (p_id, p_project_id, file_content::jsonb)
    ON CONFLICT (project_id, id) WHERE project_id IS NOT NULL DO UPDATE SET template_data = EXCLUDED.template_data;
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'Could not load template %: %', p_filename, SQLERRM;
END;
$$;

-- Call the loader for each template
CALL insert_template_from_file('demo_template', '20000000-0000-0000-0000-000000000001', 'demo_template.json');
CALL insert_template_from_file('kitchen_sink', '20000000-0000-0000-0000-000000000001', 'kitchen_sink.json');
CALL insert_template_from_file('news_culture', '20000000-0000-0000-0000-000000000001', 'news_culture.json');
CALL insert_template_from_file('news_editorial', '20000000-0000-0000-0000-000000000001', 'news_editorial.json');
CALL insert_template_from_file('news_feature', '20000000-0000-0000-0000-000000000001', 'news_feature.json');
CALL insert_template_from_file('news_versus', '20000000-0000-0000-0000-000000000001', 'news_versus.json');
