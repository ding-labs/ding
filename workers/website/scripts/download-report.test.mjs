import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {DatabaseSync} from 'node:sqlite';
import test from 'node:test';

test('recent releases backfill, older assets start at baseline, and verified jobs are excluded once', () => {
  const db = new DatabaseSync(':memory:');
  db.exec(readFileSync(new URL('../migrations/0003_local_downloads.sql', import.meta.url), 'utf8'));
  const report = () => db.prepare('SELECT * FROM github_download_report').get();
  assert.equal(report(), undefined);
  const day = 86400, start = Date.UTC(2027, 0, 1) / 1000;
  const archive = {id:1,tag:'v1',name:'ding_linux_arm64.tar.gz',kind:'archive',created:start-40*day,count:100};
  const bottle = {id:2,tag:'v1',name:'ding-1.sequoia.bottle.tar.gz',kind:'bottle',created:start-day,count:5};
  const sample = (t, assets) => db.prepare('INSERT INTO github_download_samples VALUES (?,?)').run(start+t,JSON.stringify(assets));
  sample(0,[archive,bottle]);
  assert.equal(report().archives,0);
  assert.equal(report().bottles,5);
  db.prepare('INSERT INTO release_job_fetches VALUES (1,?,?,?)').run(archive.tag,archive.name,start+4);
  sample(day,[{...archive,count:109},bottle]);
  assert.equal(report().archives,8);
  assert.equal(report().excluded,1);
  sample(2*day,[{...archive,id:3,created:start+day+1,count:2}]);
  assert.equal(report().archives,10);
  assert.equal(report().bottles,5);
  sample(31*day,[{...archive,id:3,created:start+day+1,count:11}]);
  assert.equal(report().archives,11);
  assert.equal(report().excluded,0);
  assert.equal(report().bottles,0);
  db.close();
});

import {signupEvent} from '../src/signup.js';
import {storeDownloadReading} from '../src/downloads.js';

test('the download writer has a separate credential and preserves the latest valid reading', async () => {
  const path = '/_funnel/downloads';
  const unauthorized = await signupEvent(new Request('https://ding.ing'+path,{method:'POST'}),{DOWNLOAD_REPORT_SECRET:'writer'},path);
  assert.equal(unauthorized.status,401);
  const raw = new DatabaseSync(':memory:');
  raw.exec(readFileSync(new URL('../migrations/0003_local_downloads.sql',import.meta.url),'utf8'));
  const db = {prepare(sql) {return {run:() => raw.prepare(sql).run(), bind(...args) {return {run:() => raw.prepare(sql).run(...args)};}};},
    batch: async stmts => stmts.map(s=>s.run())};
  const observed = Math.floor(Date.now()/1000);
  const brew = {source:'homebrew',observed,start_date:'2026-10-10',end_date:'2026-11-09',installs:2};
  assert.equal((await storeDownloadReading(db,brew)).status,204);
  await storeDownloadReading(db,{...brew,observed:observed-1,installs:0});
  assert.equal(raw.prepare('SELECT installs FROM brew_download_report').get().installs,2);
  const snap = {source:'github', observed, assets:[{id:1,tag:'v1',name:'ding_linux_arm64.tar.gz',kind:'archive',created:observed-20,count:7}],
    fetches:[{id:77,tag:'v1',name:'ding_linux_arm64.tar.gz',fetched:observed-3}]};
  for (let i=0;i<2;i++) assert.equal((await storeDownloadReading(db,snap)).status,204);
  assert.equal(raw.prepare('SELECT archives FROM github_download_report').get().archives,6);
  assert.equal((await storeDownloadReading(db,{...snap,assets:[]})).status,400);
  raw.close();
});
