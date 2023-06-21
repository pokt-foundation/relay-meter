DROP TABLE IF EXISTS relay_counts;
DROP TABLE IF EXISTS todays_relay_counts;
DROP TABLE IF EXISTS daily_app_sums;
DROP TABLE IF EXISTS todays_app_sums;
DROP TABLE IF EXISTS todays_app_latencies;

CREATE TABLE relay_counts (
  id INT GENERATED ALWAYS AS IDENTITY,
  origin VARCHAR NOT NULL,
  portal_app_id VARCHAR,
  count bigint,
  count_success INT,
  count_failure INT,
  time TIMESTAMPTZ
);

CREATE TABLE todays_relay_counts (
  id INT GENERATED ALWAYS AS IDENTITY,
  time TIMESTAMPTZ,
  portal_app_id VARCHAR,
  origin VARCHAR NOT NULL,
  count_success INT,
  count_failure INT,
  count bigint
);

CREATE TABLE daily_app_sums (
  id INT GENERATED ALWAYS AS IDENTITY,
  portal_app_id VARCHAR NOT NULL,
  count_success bigint NOT NULL,
  count_failure bigint NOT NULL,
  time TIMESTAMPTZ
);

CREATE TABLE todays_app_sums (
  id INT GENERATED ALWAYS AS IDENTITY,
  portal_app_id VARCHAR NOT NULL,
  count_success bigint NOT NULL,
  count_failure bigint NOT NULL
);

CREATE TABLE todays_app_latencies (
  id INT GENERATED ALWAYS AS IDENTITY,
  portal_app_id VARCHAR NOT NULL,
  time VARCHAR NOT NULL,
  latency DECIMAL NOT NULL
);

CREATE SEQUENCE success_seq START 1;
CREATE SEQUENCE error_seq START 1;

CREATE TABLE http_source_relay_count (
    portal_app_id char(64) NOT NULL,
    day date NOT NULL,
    success BIGINT DEFAULT nextval('success_seq') NOT NULL,
    error BIGINT DEFAULT nextval('error_seq') NOT NULL,
    PRIMARY KEY (app_public_key, day)
);
