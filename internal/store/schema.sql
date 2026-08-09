CREATE TABLE IF NOT EXISTS schema_migrations (
  version integer PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY,
  username text NOT NULL UNIQUE,
  display_name text NOT NULL,
  email text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT 'USER' CHECK (role IN ('USER','ADMIN')),
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DISABLED')),
  identity_provider text NOT NULL DEFAULT 'LOCAL',
  external_subject text,
  legacy_system text,
  legacy_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  last_login_at timestamptz,
  UNIQUE(identity_provider, external_subject)
);

CREATE TABLE IF NOT EXISTS local_credentials (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  password_hash text NOT NULL,
  changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  csrf_token text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  remote_addr text NOT NULL DEFAULT '',
  user_agent text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS api_keys (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  key_prefix text NOT NULL,
  token_hash text NOT NULL UNIQUE,
  scopes text[] NOT NULL DEFAULT ARRAY['wiki:read']::text[],
  expires_at timestamptz,
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  rotated_from uuid REFERENCES api_keys(id),
  revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS api_keys_user_idx ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS user_preferences (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  locale text NOT NULL DEFAULT 'ko-KR',
  theme text NOT NULL DEFAULT 'system',
  home_space_id uuid,
  editor_mode text NOT NULL DEFAULT 'rich',
  settings jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS groups (
  id uuid PRIMARY KEY,
  name text NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  legacy_system text,
  legacy_id text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS group_members (
  group_id uuid NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY(group_id, user_id)
);

CREATE TABLE IF NOT EXISTS spaces (
  id uuid PRIMARY KEY,
  space_key text NOT NULL UNIQUE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'ACTIVE',
  legacy_system text,
  legacy_id text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pages (
  id uuid PRIMARY KEY,
  space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  parent_id uuid REFERENCES pages(id) ON DELETE SET NULL,
  title text NOT NULL,
  status text NOT NULL DEFAULT 'CURRENT',
  current_version integer NOT NULL DEFAULT 1,
  legacy_system text,
  legacy_id text,
  created_by uuid REFERENCES users(id),
  updated_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  UNIQUE(legacy_system, legacy_id)
);
CREATE INDEX IF NOT EXISTS pages_space_idx ON pages(space_id);
CREATE INDEX IF NOT EXISTS pages_parent_idx ON pages(parent_id);
CREATE INDEX IF NOT EXISTS pages_title_idx ON pages(lower(title));

CREATE TABLE IF NOT EXISTS page_versions (
  id uuid PRIMARY KEY,
  page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  version integer NOT NULL,
  title text NOT NULL,
  legacy_system text,
  legacy_id text,
  legacy_storage text NOT NULL DEFAULT '',
  canonical_document jsonb NOT NULL DEFAULT '{"type":"document","children":[]}'::jsonb,
  editor_document jsonb NOT NULL DEFAULT '{"type":"doc","content":[]}'::jsonb,
  rendered_text text NOT NULL DEFAULT '',
  change_message text NOT NULL DEFAULT '',
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  content_hash text NOT NULL DEFAULT '',
  UNIQUE(page_id, version)
);
ALTER TABLE page_versions ADD COLUMN IF NOT EXISTS canonical_document jsonb NOT NULL DEFAULT '{"type":"document","children":[]}'::jsonb;
ALTER TABLE page_versions ADD COLUMN IF NOT EXISTS legacy_system text;
ALTER TABLE page_versions ADD COLUMN IF NOT EXISTS legacy_id text;
CREATE UNIQUE INDEX IF NOT EXISTS page_versions_legacy_idx ON page_versions(legacy_system,legacy_id) WHERE legacy_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS page_versions_search_idx ON page_versions USING gin(to_tsvector('simple', title || ' ' || rendered_text));

CREATE TABLE IF NOT EXISTS page_permissions (
  id uuid PRIMARY KEY,
  page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  permission text NOT NULL,
  subject_type text NOT NULL CHECK (subject_type IN ('USER','GROUP')),
  subject_id uuid NOT NULL,
  legacy_system text,
  legacy_id text,
  UNIQUE(page_id, permission, subject_type, subject_id)
);
ALTER TABLE page_permissions ADD COLUMN IF NOT EXISTS legacy_system text;
ALTER TABLE page_permissions ADD COLUMN IF NOT EXISTS legacy_id text;
CREATE UNIQUE INDEX IF NOT EXISTS page_permissions_legacy_idx ON page_permissions(legacy_system,legacy_id) WHERE legacy_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS space_permissions (
  id uuid PRIMARY KEY,
  space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  permission text NOT NULL,
  subject_type text NOT NULL CHECK (subject_type IN ('USER','GROUP')),
  subject_id uuid NOT NULL,
  legacy_system text,
  legacy_id text,
  UNIQUE(space_id, permission, subject_type, subject_id)
);
ALTER TABLE space_permissions ADD COLUMN IF NOT EXISTS legacy_system text;
ALTER TABLE space_permissions ADD COLUMN IF NOT EXISTS legacy_id text;
CREATE UNIQUE INDEX IF NOT EXISTS space_permissions_legacy_idx ON space_permissions(legacy_system,legacy_id) WHERE legacy_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS comments (
  id uuid PRIMARY KEY,
  page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  parent_id uuid REFERENCES comments(id) ON DELETE CASCADE,
  body text NOT NULL,
  resolved_at timestamptz,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  legacy_system text,
  legacy_id text
);

CREATE TABLE IF NOT EXISTS attachments (
  id uuid PRIMARY KEY,
  page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  filename text NOT NULL,
  media_type text NOT NULL,
  size bigint NOT NULL,
  sha256 text NOT NULL,
  storage_key text NOT NULL,
  version integer NOT NULL DEFAULT 1,
  legacy_system text,
  legacy_id text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS labels (
  id uuid PRIMARY KEY,
  name text NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS page_labels (
  page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  label_id uuid NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  PRIMARY KEY(page_id, label_id)
);

CREATE TABLE IF NOT EXISTS page_links (
  id uuid PRIMARY KEY,
  source_page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  target_page_id uuid REFERENCES pages(id) ON DELETE SET NULL,
  legacy_target_id text NOT NULL DEFAULT '',
  display_text text NOT NULL DEFAULT '',
  link_type text NOT NULL,
  target text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(source_page_id,link_type,target)
);
CREATE INDEX IF NOT EXISTS page_links_target_idx ON page_links(target_page_id);

CREATE TABLE IF NOT EXISTS page_macros (
  page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  page_version integer NOT NULL,
  macro_name text NOT NULL,
  supported boolean NOT NULL,
  occurrence_count integer NOT NULL DEFAULT 1,
  PRIMARY KEY(page_id,page_version,macro_name)
);

CREATE TABLE IF NOT EXISTS system_settings (
  key text PRIMARY KEY,
  value jsonb NOT NULL,
  secret boolean NOT NULL DEFAULT false,
  description text NOT NULL DEFAULT '',
  updated_by uuid REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS migration_state (
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  phase text NOT NULL DEFAULT 'LEGACY',
  source_mode text NOT NULL DEFAULT 'LEGACY',
  readiness numeric(5,2) NOT NULL DEFAULT 0,
  cdc_lag_ms bigint NOT NULL DEFAULT 0,
  failed_events bigint NOT NULL DEFAULT 0,
  started_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  details jsonb NOT NULL DEFAULT '{}'::jsonb
);
INSERT INTO migration_state(id) VALUES(true) ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS migration_jobs (
  id uuid PRIMARY KEY,
  kind text NOT NULL,
  status text NOT NULL,
  total_items bigint NOT NULL DEFAULT 0,
  processed_items bigint NOT NULL DEFAULT 0,
  failed_items bigint NOT NULL DEFAULT 0,
  checkpoint jsonb NOT NULL DEFAULT '{}'::jsonb,
  options jsonb NOT NULL DEFAULT '{}'::jsonb,
  current_entity text NOT NULL DEFAULT '',
  cancel_requested boolean NOT NULL DEFAULT false,
  error text NOT NULL DEFAULT '',
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  finished_at timestamptz
);
ALTER TABLE migration_jobs ADD COLUMN IF NOT EXISTS options jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE migration_jobs ADD COLUMN IF NOT EXISTS current_entity text NOT NULL DEFAULT '';
ALTER TABLE migration_jobs ADD COLUMN IF NOT EXISTS cancel_requested boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS migration_jobs_one_active_snapshot ON migration_jobs(kind) WHERE kind='SNAPSHOT' AND status IN ('PENDING','RUNNING','CANCEL_REQUESTED');
CREATE UNIQUE INDEX IF NOT EXISTS migration_jobs_one_active_reconciliation ON migration_jobs(kind) WHERE kind='RECONCILIATION' AND status IN ('PENDING','RUNNING','CANCEL_REQUESTED');

CREATE TABLE IF NOT EXISTS migration_mapping (
  id uuid PRIMARY KEY,
  legacy_system text NOT NULL,
  entity_type text NOT NULL,
  legacy_id text NOT NULL,
  target_id uuid NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(legacy_system, entity_type, legacy_id)
);
CREATE INDEX IF NOT EXISTS migration_mapping_target_idx ON migration_mapping(entity_type,target_id);

CREATE TABLE IF NOT EXISTS migration_items (
  id uuid PRIMARY KEY,
  job_id uuid NOT NULL REFERENCES migration_jobs(id) ON DELETE CASCADE,
  entity_type text NOT NULL,
  legacy_id text NOT NULL,
  target_id uuid,
  status text NOT NULL DEFAULT 'PENDING',
  retry_count integer NOT NULL DEFAULT 0,
  source_hash text NOT NULL DEFAULT '',
  target_hash text NOT NULL DEFAULT '',
  error text NOT NULL DEFAULT '',
  details jsonb NOT NULL DEFAULT '{}'::jsonb,
  started_at timestamptz,
  finished_at timestamptz,
  UNIQUE(job_id, entity_type, legacy_id)
);
CREATE INDEX IF NOT EXISTS migration_items_job_status_idx ON migration_items(job_id,status);

CREATE TABLE IF NOT EXISTS unsupported_content (
  id uuid PRIMARY KEY,
  job_id uuid REFERENCES migration_jobs(id) ON DELETE SET NULL,
  page_id uuid REFERENCES pages(id) ON DELETE CASCADE,
  legacy_id text NOT NULL DEFAULT '',
  kind text NOT NULL,
  name text NOT NULL,
  status text NOT NULL DEFAULT 'OPEN',
  occurrence_count bigint NOT NULL DEFAULT 1,
  sample text NOT NULL DEFAULT '',
  resolution text NOT NULL DEFAULT '',
  resolved_by uuid REFERENCES users(id),
  resolved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(job_id,page_id,kind,name)
);
ALTER TABLE unsupported_content ADD COLUMN IF NOT EXISTS resolved_by uuid REFERENCES users(id);
ALTER TABLE unsupported_content ADD COLUMN IF NOT EXISTS resolved_at timestamptz;
CREATE INDEX IF NOT EXISTS unsupported_content_job_status_idx ON unsupported_content(job_id,status,kind);

CREATE TABLE IF NOT EXISTS macro_compatibility (
  macro_name text PRIMARY KEY,
  support_level text NOT NULL,
  page_count bigint NOT NULL DEFAULT 0,
  occurrence_count bigint NOT NULL DEFAULT 0,
  conversion_rate numeric(5,2) NOT NULL DEFAULT 0,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  details jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS migration_checks (
  id uuid PRIMARY KEY,
  category text NOT NULL,
  check_name text NOT NULL,
  status text NOT NULL,
  source_count bigint,
  target_count bigint,
  mismatch_count bigint NOT NULL DEFAULT 0,
  details jsonb NOT NULL DEFAULT '{}'::jsonb,
  checked_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(category, check_name)
);
CREATE UNIQUE INDEX IF NOT EXISTS migration_checks_unique ON migration_checks(category, check_name);

CREATE TABLE IF NOT EXISTS schema_discovery (
  id uuid PRIMARY KEY,
  database_version text NOT NULL DEFAULT '',
  character_set text NOT NULL DEFAULT '',
  collation_name text NOT NULL DEFAULT '',
  confluence_version text NOT NULL DEFAULT '',
  attachment_mode text NOT NULL DEFAULT 'filesystem',
  summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cdc_events (
  id uuid PRIMARY KEY,
  source_table text NOT NULL,
  operation text NOT NULL,
  primary_key jsonb NOT NULL,
  before_value jsonb,
  after_value jsonb,
  binlog_file text NOT NULL DEFAULT '',
  binlog_position bigint NOT NULL DEFAULT 0,
  transaction_id text NOT NULL DEFAULT '',
  event_time timestamptz NOT NULL,
  processed_at timestamptz,
  status text NOT NULL DEFAULT 'PENDING',
  retry_count integer NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS audit_events (
  id uuid PRIMARY KEY,
  actor_id uuid REFERENCES users(id),
  action text NOT NULL,
  resource_type text NOT NULL DEFAULT '',
  resource_id text NOT NULL DEFAULT '',
  remote_addr text NOT NULL DEFAULT '',
  user_agent text NOT NULL DEFAULT '',
  detail jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_events_time_idx ON audit_events(created_at DESC);

CREATE TABLE IF NOT EXISTS webhooks (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  url text NOT NULL,
  event_types text[] NOT NULL,
  secret_ciphertext text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS wiki_change_journal (
  sequence bigserial PRIMARY KEY,
  entity text NOT NULL,
  entity_id uuid,
  operation text NOT NULL,
  old_value jsonb,
  new_value jsonb,
  user_id uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  legacy_replay_status text NOT NULL DEFAULT 'PENDING'
);
