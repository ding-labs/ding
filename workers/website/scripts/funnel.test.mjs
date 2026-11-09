import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { DatabaseSync } from "node:sqlite";
import test from "node:test";

const migration = readFileSync(new URL("../migrations/0001_homepage_funnel.sql", import.meta.url), "utf8");

test("the rolling funnel deduplicates browsers and accounts and excludes owner and repeat sign-ins", () => {
  const db = new DatabaseSync(":memory:");
  db.exec(migration);
  db.exec(`
    INSERT INTO homepage_landings VALUES ('visitor',date('now')),('visitor',date('now','-1 day')),('owner',date('now')),('reader',date('now'));
    INSERT INTO homepage_clicks VALUES
      ('a','visitor',unixepoch()-80,'new',unixepoch()-10),('b','visitor',unixepoch()-90,'new',unixepoch()-11),
      ('c','owner',unixepoch()-90,'me',unixepoch()),('d','visitor',unixepoch()-20,'returning',unixepoch());
    INSERT INTO cloud_signups VALUES ('new',unixepoch()-35),('me',unixepoch()-72),('returning',unixepoch()-1000),('direct',unixepoch()-200),('old',unixepoch()-3600000);
    INSERT INTO excluded_accounts VALUES ('me');
    INSERT INTO excluded_browsers VALUES ('owner',unixepoch());
  `);
  assert.deepEqual({...db.prepare("SELECT * FROM homepage_funnel_30d").get()}, {
    homepage_browsers: 2, browsers_clicked_get_started: 1, signed_up_from_button: 1, all_outside_signups: 3,
  });
  db.close();
});
