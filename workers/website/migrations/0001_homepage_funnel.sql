-- Browser IDs are random. Account keys are SHA-256 digests of WorkOS user IDs.
CREATE TABLE homepage_landings (
  browser TEXT NOT NULL,
  day TEXT NOT NULL,
  PRIMARY KEY (day, browser)
) WITHOUT ROWID;

CREATE TABLE homepage_clicks (
  handoff TEXT PRIMARY KEY,
  browser TEXT NOT NULL,
  clicked_at INTEGER NOT NULL,
  account TEXT,
  verified_at INTEGER
);
CREATE INDEX homepage_click_time ON homepage_clicks(clicked_at);
CREATE INDEX homepage_click_account ON homepage_clicks(account, clicked_at);

CREATE TABLE cloud_signups (account TEXT PRIMARY KEY, created_at INTEGER NOT NULL);
CREATE TABLE excluded_accounts (account TEXT PRIMARY KEY);
CREATE TABLE excluded_browsers (browser TEXT PRIMARY KEY, last_seen INTEGER NOT NULL);

-- Thirty UTC calendar days, including today. Count each browser/account once.
CREATE VIEW homepage_funnel_30d AS
WITH period AS (SELECT unixepoch(date('now','-29 days')) AS start),
landings AS (
  SELECT DISTINCT browser FROM homepage_landings
  WHERE day >= date('now','-29 days') AND browser NOT IN (SELECT browser FROM excluded_browsers)
), clicks AS (
  SELECT c.* FROM homepage_clicks c, period
  WHERE c.clicked_at >= start AND c.browser IN (SELECT browser FROM landings)
), signups AS (
  SELECT s.* FROM cloud_signups s, period
  WHERE s.created_at >= start AND s.account NOT IN (SELECT account FROM excluded_accounts)
)
SELECT (SELECT count(*) FROM landings) AS homepage_browsers,
       (SELECT count(DISTINCT browser) FROM clicks) AS browsers_clicked_get_started,
       (SELECT count(*) FROM signups s WHERE EXISTS (
         SELECT 1 FROM clicks c WHERE c.account = s.account
         AND s.created_at BETWEEN c.clicked_at AND c.verified_at
       )) AS signed_up_from_button,
       (SELECT count(*) FROM signups) AS all_outside_signups;
