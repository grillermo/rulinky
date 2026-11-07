-- Mirror of the production schema (db/schema.rb) for the app's own tables.
-- Only tests load this file; production keeps its existing tables.
CREATE TABLE users (
  id bigserial PRIMARY KEY,
  api_token varchar,
  confirmed_at timestamp(6),
  created_at timestamp(6) NOT NULL,
  current_sign_in_at timestamp(6),
  current_sign_in_ip varchar,
  email varchar NOT NULL DEFAULT '',
  last_sign_in_at timestamp(6),
  last_sign_in_ip varchar,
  otp_code varchar,
  otp_expires_at timestamp(6),
  remember_created_at timestamp(6),
  sign_in_count integer NOT NULL DEFAULT 0,
  updated_at timestamp(6) NOT NULL
);
CREATE UNIQUE INDEX index_users_on_api_token ON users (api_token);
CREATE UNIQUE INDEX index_users_on_email ON users (email);

CREATE TABLE links (
  id varchar PRIMARY KEY,
  content text,
  note text,
  raw_title text,
  read integer NOT NULL DEFAULT 0,
  "timestamp" bigint NOT NULL,
  updated_at bigint NOT NULL,
  url text NOT NULL,
  user_id bigint NOT NULL REFERENCES users (id)
);
CREATE UNIQUE INDEX index_links_on_id ON links (id);
CREATE INDEX index_links_on_read ON links (read);
CREATE INDEX index_links_on_updated_at ON links (updated_at);
CREATE INDEX index_links_on_user_id ON links (user_id);

CREATE TABLE link_content_jobs (
  id varchar PRIMARY KEY,
  created_at timestamp(6) NOT NULL,
  error_message text,
  link_id varchar NOT NULL,
  status varchar NOT NULL DEFAULT 'queued',
  updated_at timestamp(6) NOT NULL
);
CREATE INDEX index_link_content_jobs_on_link_id ON link_content_jobs (link_id);
CREATE INDEX index_link_content_jobs_on_status ON link_content_jobs (status);
