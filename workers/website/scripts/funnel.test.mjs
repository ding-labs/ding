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

import { homepageFunnel } from "../src/funnel.js";

test("a real homepage button click gets a handoff while crawlers, prefetched links and excluded browsers keep the direct login", async () => {
  const browser = crypto.randomUUID();
  const headers = {Cookie: `__Host-ding-visit=${browser}`, Referer: "https://ding.ing/", "Sec-Fetch-Dest": "document", "User-Agent": "Firefox"};
  let batches = 0;
  const FUNNEL = {prepare: () => ({bind: () => ({})}), batch: async () => { batches++; }};
  const click = extra => homepageFunnel(new Request("https://ding.ing/go/cloud", {headers: {...headers,...extra}}), {FUNNEL});
  assert.match((await click({})).headers.get("Location"), /\/auth\/login\?homepage_handoff=[a-f0-9-]{36}$/);
  for (const extra of [{"Sec-Purpose":"prefetch"}, {"User-Agent":"ExampleCrawler"}, {Cookie: headers.Cookie+"; __Host-ding-funnel-off=1"}, {Referer:"https://elsewhere.example/"}, {"Sec-GPC":"1"}]) {
    assert.equal((await click(extra)).headers.get("Location"), "https://cloud.ding.ing/auth/login");
  }
  assert.equal(batches, 1);
});

import { createHash, createHmac } from "node:crypto";
import { signupEvent } from "../src/signup.js";
test("only signed WorkOS creations count and a signed callback links the browser independently of event order", async () => {
  const raw = new DatabaseSync(":memory:");
  raw.exec(migration);
  const db = {
    prepare(sql) { return {bind(...args) { return {run: () => raw.prepare(sql).run(...args)}; }}; },
    batch(stmts) { return stmts.map(s => s.run()); },
  };
  const env = {FUNNEL:db, FUNNEL_WORKOS_SECRET:"workos", FUNNEL_RECEIPT_SECRET:"cloud"};
  const browser = crypto.randomUUID(), handoff = crypto.randomUUID(), user = "user_Example";
  const account = createHash("sha256").update(user).digest("hex");
  raw.prepare("INSERT INTO homepage_landings VALUES (?,date('now'))").run(browser);
  raw.prepare("INSERT INTO homepage_clicks(handoff,browser,clicked_at) VALUES (?,?,unixepoch()-5)").run(handoff,browser);
  async function post(path, header, secret, payload) {
    const time = Date.now(), body = JSON.stringify(payload);
    const sig = createHmac("sha256",secret).update(time+"."+body).digest("hex");
    const req = new Request("https://ding.ing"+path,{method:"POST",body,headers:{[header]:`t=${time}, v1=${sig}`}});
    return (await signupEvent(req,env,path)).status;
  }
  const event = {event:"user.created",data:{object:"user",id:user,created_at:new Date().toISOString()}};
  assert.equal(await post("/_funnel/workos","WorkOS-Signature","wrong",event),401);
  assert.equal(await post("/_funnel/verified","Ding-Signature","cloud",{action:"verified",account,handoff}),204);
  for(let i=0;i<2;i++) assert.equal(await post("/_funnel/workos","WorkOS-Signature","workos",event),204);
  assert.equal(raw.prepare("SELECT signed_up_from_button FROM homepage_funnel_30d").get().signed_up_from_button,1);
  raw.prepare("INSERT INTO excluded_accounts VALUES (?)").run(account);
  await post("/_funnel/verified","Ding-Signature","cloud",{action:"verified",account,handoff});
  assert.deepEqual(Object.values(raw.prepare("SELECT * FROM homepage_funnel_30d").get()),[0,0,0,0]);
  raw.close();
});
