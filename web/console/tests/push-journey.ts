import { expect, type Page, type TestInfo } from "@playwright/test";
export async function cloudPushJourney(page:Page,viewer:Page,url:string,testInfo:TestInfo) {
 const manifest=`apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: ci}
spec:
  source: {type: push, fields: {repo: repo, failed: failed}}
  groupBy: [repo]
  condition: {field: failed, operator: eq, value: true}
  policy: {trigger: transition}
`;
 const applied=await page.evaluate(async manifest=>{
  const session=await(await fetch("/v1/browser/session")).json();
  const headers={"Content-Type":"application/json","X-Ding-CSRF":session.data.csrf};
  const preview=await(await fetch("/v1/apply",{method:"POST",headers,body:JSON.stringify({manifest,dryRun:true})})).json();
  const response=await fetch("/v1/apply",{method:"POST",headers:{"Content-Type":"application/json","X-Ding-CSRF":session.data.csrf},body:JSON.stringify({manifest,review:preview.data?.review})});
  return {status:response.status,body:await response.text()};
 },manifest);
 expect(applied.status,applied.body).toBe(200);
 await page.getByRole("button",{name:"Services",exact:true}).click();
 await page.getByRole("button",{name:"Add credential",exact:true}).click();
 await page.getByLabel("Credential name",{exact:true}).fill("Actions producer");
 await page.getByRole("combobox",{name:"Permission",exact:true}).selectOption("ingest",{timeout:10000});
 await page.getByLabel("Allowed push watch IDs",{exact:true}).fill("ci");
 await page.getByRole("button",{name:"Create credential",exact:true}).click();
 const secret=page.getByLabel("New service credential");await expect(secret).toBeVisible();const first=await secret.inputValue();expect(first).toMatch(/^ding_service_/);
 await page.getByLabel("I’ve saved this key").check();await page.getByRole("button",{name:"Done",exact:true}).click();await expect(secret).toHaveCount(0);
 expect((await viewer.request.get(url+"/v1/services")).status()).toBe(403);
 const body='{"repo":"owner/api","failed":true}';
 const send=(token:string,key="run-1",data=body)=>page.request.post(url+"/v1/ingest/ci",{headers:{Authorization:`Bearer ${token}`,"Idempotency-Key":key,"Content-Type":"application/json"},data});
 const accepted=await send(first);expect(accepted.status()).toBe(202);const receipt=(await accepted.json()).data;
 await page.getByRole("button",{name:"Rotate key",exact:true}).click();await expect(secret).toBeVisible();const rotated=await secret.inputValue();expect(rotated).not.toBe(first);
 const replay=await send(rotated);expect(replay.status()).toBe(202);expect((await replay.json()).data).toMatchObject({acceptanceId:receipt.acceptanceId,duplicate:true});
 await page.getByLabel("I’ve saved this key").check();await page.getByRole("button",{name:"Done",exact:true}).click();
 await page.reload();await expect(secret).toHaveCount(0);
 await page.goto(url+"/ui/watches/ci");
 await page.getByText("Preview JSON projection",{exact:true}).click();
 await page.getByLabel("Source JSON",{exact:true}).fill(body);
 const previewResponse=page.waitForResponse(r=>r.url().endsWith("/v1/tools/project")&&r.request().method()==="POST");
 await page.getByRole("button",{name:"Preview JSON",exact:true}).click();
 const preview=await previewResponse;expect(preview.status()).toBe(200);expect((await preview.json()).data.persisted).toBe(false);
 const usage=await(await page.request.get(url+"/v1/cloud/usage")).json();expect(usage.data.ingest.requests).toBe(1);
 await expect(page.getByText(/Last durable acceptance:/)).toBeVisible();
 await page.screenshot({path:testInfo.outputPath("cloud-push.png"),fullPage:true});
 // Errors have safe diagnostic IDs and do not count as accepted observations.
 expect((await send(rotated,"run-1",'{"repo":"owner/api","failed":false}')).status()).toBe(409);
 const status=await(await page.request.get(url+"/v1/ingest-status/ci")).json();expect(status.data.recent.some((r:{code:string})=>r.code==="idempotency_conflict")).toBe(true);
 await page.getByRole("link",{name:"System",exact:true}).click();await page.getByRole("button",{name:"Services",exact:true}).click();
 // A rotation can commit while its one-time secret response is lost.
 await page.route("**/v1/services/*/rotate",async route=>{await route.fetch();await route.abort("failed");},{times:1});
 await page.getByRole("button",{name:"Rotate key",exact:true}).click();
 await expect(page.getByRole("alert")).toBeVisible();
 await expect(secret).toHaveCount(0);
 await page.reload();
 await expect(page.getByRole("button",{name:"Revoke producer",exact:true})).toBeVisible();
 await expect(secret).toHaveCount(0);
 await page.getByRole("button",{name:"Revoke producer",exact:true}).click();
 await expect(page.getByRole("button",{name:"Revoke producer",exact:true})).toHaveCount(0);
 expect((await send(rotated)).status()).toBe(401);
 await page.getByRole("button",{name:"Members",exact:true}).click();
}
