-- One complete GitHub reading per collection. Immutable asset IDs survive tag
-- reuse and renamed/replaced archives never inherit their predecessor's count.
CREATE TABLE github_download_samples (observed INTEGER PRIMARY KEY, assets TEXT NOT NULL);
CREATE TABLE release_job_fetches (
  job INTEGER PRIMARY KEY, tag TEXT NOT NULL, filename TEXT NOT NULL, fetched INTEGER NOT NULL
);
CREATE TABLE brew_download_report (
  singleton INTEGER PRIMARY KEY CHECK(singleton=1), observed INTEGER NOT NULL,
  start_date TEXT NOT NULL, end_date TEXT NOT NULL, installs INTEGER NOT NULL
);

-- Verified clean-runner "Formula ding (version)" fetches in the official
-- bottles.yml logs before the named fetch step was introduced. The job IDs
-- identify the underlying logs in ding-labs/homebrew-tap.
INSERT INTO release_job_fetches VALUES
(124423961513,'v0.15.3','ding_darwin_amd64.tar.gz',unixepoch('2026-10-31 03:06:40')),
(124423961707,'v0.15.3','ding_darwin_arm64.tar.gz',unixepoch('2026-10-31 03:07:49')),
(114363053164,'v0.15.2','ding_darwin_amd64.tar.gz',unixepoch('2026-10-11 01:51:43')),
(114363053324,'v0.15.2','ding_darwin_arm64.tar.gz',unixepoch('2026-10-11 01:52:14')),
(114355602139,'v0.15.1','ding_darwin_amd64.tar.gz',unixepoch('2026-10-11 01:06:48')),
(114355602302,'v0.15.1','ding_darwin_arm64.tar.gz',unixepoch('2026-10-11 01:06:59')),
(114334770670,'v0.15.0','ding_darwin_arm64.tar.gz',unixepoch('2026-10-10 23:02:08')),
(114334770797,'v0.15.0','ding_darwin_amd64.tar.gz',unixepoch('2026-10-10 23:02:43')),
(114334446411,'v0.15.0','ding_darwin_arm64.tar.gz',unixepoch('2026-10-10 23:00:17'));

CREATE VIEW github_download_report AS
WITH latest AS (SELECT max(observed) AS observed FROM github_download_samples),
period AS (
  SELECT observed, unixepoch(date(observed,'unixepoch','-30 days')) AS cutoff FROM latest
), window AS (
  SELECT period.*, (SELECT min(s.observed) FROM github_download_samples s WHERE s.observed >= cutoff) AS baseline
  FROM period WHERE observed IS NOT NULL
), readings AS (
  SELECT s.observed, a.value ->> 'id' AS id, a.value ->> 'tag' AS tag, a.value ->> 'name' AS filename,
    a.value ->> 'kind' AS kind, a.value ->> 'created' AS created, a.value ->> 'count' AS n
  FROM github_download_samples s, window w, json_each(s.assets) a WHERE s.observed >= w.baseline
), bounds AS (
  SELECT id, min(observed) AS first_seen, max(observed) AS last_seen FROM readings GROUP BY id
), changes AS (
  SELECT first.tag,first.filename,first.kind,b.last_seen,
    CASE WHEN first.created >= w.cutoff AND (date(w.baseline,'unixepoch') > date(w.cutoff,'unixepoch') OR first.created > w.baseline)
      THEN first.created ELSE b.first_seen END AS since,
    last.n - CASE WHEN first.created >= w.cutoff AND (date(w.baseline,'unixepoch') > date(w.cutoff,'unixepoch') OR first.created > w.baseline)
      THEN 0 ELSE first.n END AS gross
  FROM bounds b JOIN readings first ON first.id=b.id AND first.observed=b.first_seen
  JOIN readings last ON last.id=b.id AND last.observed=b.last_seen CROSS JOIN window w
), adjusted AS (
  SELECT kind, max(0,gross) AS gross,
    min(max(0,gross),(SELECT count(*) FROM release_job_fetches f WHERE f.tag=c.tag AND f.filename=c.filename
      AND f.fetched BETWEEN c.since AND c.last_seen)) AS excluded
  FROM changes c
)
SELECT w.observed,w.baseline,w.cutoff,
  coalesce((SELECT sum(gross-excluded) FROM adjusted WHERE kind='archive'),0) AS archives,
  coalesce((SELECT sum(gross-excluded) FROM adjusted WHERE kind='bottle'),0) AS bottles,
  coalesce((SELECT sum(excluded) FROM adjusted),0) AS excluded
FROM window w;
