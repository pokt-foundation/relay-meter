DROP TABLE IF EXISTS relay_counts;
DROP TABLE IF EXISTS todays_relay_counts;
DROP TABLE IF EXISTS daily_app_sums;
DROP TABLE IF EXISTS todays_app_sums;
DROP TABLE IF EXISTS todays_app_latencies;
CREATE TABLE relay_counts (
  id INT GENERATED ALWAYS AS IDENTITY,
  origin VARCHAR NOT NULL,
  application VARCHAR,
  count bigint,
  count_success INT,
  count_failure INT,
  time TIMESTAMPTZ
);
CREATE TABLE todays_relay_counts (
  id INT GENERATED ALWAYS AS IDENTITY,
  time TIMESTAMPTZ,
  application VARCHAR,
  origin VARCHAR NOT NULL,
  count_success INT,
  count_failure INT,
  count bigint
);
CREATE TABLE daily_app_sums (
  id INT GENERATED ALWAYS AS IDENTITY,
  application VARCHAR NOT NULL,
  count_success bigint NOT NULL,
  count_failure bigint NOT NULL,
  time TIMESTAMPTZ
);
CREATE TABLE todays_app_sums (
  id INT GENERATED ALWAYS AS IDENTITY,
  application VARCHAR NOT NULL,
  count_success bigint NOT NULL,
  count_failure bigint NOT NULL
);
CREATE TABLE todays_app_latencies (
  id INT GENERATED ALWAYS AS IDENTITY,
  application VARCHAR NOT NULL,
  time VARCHAR NOT NULL,
  latency DECIMAL NOT NULL
);
CREATE SEQUENCE success_seq START 1;
CREATE SEQUENCE error_seq START 1;
CREATE TABLE http_source_relay_count (
  app_public_key char(64) NOT NULL,
  day date NOT NULL,
  success BIGINT DEFAULT nextval('success_seq') NOT NULL,
  error BIGINT DEFAULT nextval('error_seq') NOT NULL,
  PRIMARY KEY (app_public_key, day)
);
-- Seed HTTP Source DB with test relays
INSERT INTO http_source_relay_count(app_public_key, day, success, error)
VALUES (
    'test_34715cae753e67c75fbb340442e7de8e',
    current_date - INTERVAL '1 day',
    1750000,
    2000
  ),
  (
    'test_8237c72345f12d1b1a8b64a1a7f66fa4',
    current_date - INTERVAL '1 day',
    7850000,
    5000
  ),
  (
    'test_f608500e4fe3e09014fe2411b4a560b5',
    current_date - INTERVAL '1 day',
    12850000,
    12000
  ),
  (
    'test_f6a5d8690ecb669865bd752b7796a920',
    current_date - INTERVAL '1 day',
    1000,
    500
  );
