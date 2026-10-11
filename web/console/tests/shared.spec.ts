import { test, expect } from "./fixtures";
import { spawn, execFileSync } from "node:child_process";
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from "node:fs";
import { createServer as httpsServer } from "node:https";
import { request as httpRequest, createServer } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";

test.use({ ignoreHTTPSErrors: true });
test("shared owner invites a viewer and revokes their access", async ({ binary, page, browser }, testInfo) => {
  const dir = mkdtempSync(join(tmpdir(), "ding-shared-test-"));
  const key = join(dir, "tls.key"), cert = join(dir, "tls.crt");
  execFileSync("openssl", ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", cert, "-subj", "/CN=localhost", "-days", "1"], { stdio: "ignore" });
  const reserve = createServer(); await new Promise<void>(r => reserve.listen(0,"127.0.0.1",r));
  const port = (reserve.address() as {port:number}).port; await new Promise<void>(r => reserve.close(()=>r()));
  const proxy = httpsServer({key:readFileSync(key),cert:readFileSync(cert)}, (req,res) => {
    const forwarded=httpRequest({host:"127.0.0.1",port,path:req.url,method:req.method,headers:req.headers},upstream=>{res.writeHead(upstream.statusCode||502,upstream.headers);upstream.pipe(res);});
    forwarded.on("error",()=>{res.writeHead(503);res.end();});req.pipe(forwarded);
  });
  await new Promise<void>(r=>proxy.listen(0,"127.0.0.1",r));
  const url=`https://127.0.0.1:${(proxy.address() as {port:number}).port}`;
  const config=join(dir,"config.json"),password=join(dir,"password");
  writeFileSync(config,JSON.stringify({hosting:"self-hosted",name:"Team Ding",publicURL:url,listen:`127.0.0.1:${port}`,dataDir:join(dir,"state"),keyFile:join(dir,"vault.key")}),{mode:0o600});
  writeFileSync(password,"owner correct password",{mode:0o600});
  execFileSync(binary,["server","init","--config",config,"--username","owner","--name","Owner","--password-file",password]);
  const child=spawn(binary,["server","run","--config",config],{stdio:"pipe"});
  let logs="";child.stderr.on("data",b=>{logs+=b.toString();});
  const reader=await browser.newContext({ignoreHTTPSErrors:true});
  try {
    await expect.poll(async()=>{try{return (await page.request.get(url+"/healthz")).status();}catch{return 0;}},{timeout:15000,message:logs}).toBe(200);
    await page.goto(url+"/ui/");
    await expect(page.getByRole("heading",{name:"Sign in to Team Ding."})).toBeVisible();
    await page.getByLabel("Username",{exact:true}).fill("owner");await page.getByLabel("Password",{exact:true}).fill("owner correct password");await page.getByRole("button",{name:"Sign in",exact:true}).click();
    await page.getByRole("link",{name:"System",exact:true}).click();await page.getByRole("button",{name:"Members",exact:true}).click();
    await page.getByLabel("New username").fill("reader");await page.getByLabel("Display name").fill("Reader");await page.getByRole("button",{name:"Create invitation"}).click();
    const link=page.getByLabel("Account activation link");await expect(link).toBeVisible();
    await page.setViewportSize({width:375,height:900});
    await page.screenshot({path:testInfo.outputPath("members.png"),fullPage:true});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    await page.setViewportSize({width:1440,height:1000});
    const visitor=await reader.newPage();await visitor.goto(await link.inputValue());
    await visitor.getByLabel("New password",{exact:true}).fill("reader correct password");await visitor.getByRole("button",{name:"Set password",exact:true}).click();
    await visitor.getByLabel("Username",{exact:true}).fill("reader");await visitor.getByLabel("Password",{exact:true}).fill("reader correct password");await visitor.getByRole("button",{name:"Sign in",exact:true}).click();
    await expect(visitor.getByRole("link",{name:"System",exact:true})).toBeVisible();await expect(visitor.getByRole("link",{name:"Workbench",exact:true})).toHaveCount(0);
    const forbidden=await visitor.evaluate(async()=>{const s=await(await fetch("/v1/browser/session")).json();return(await fetch("/v1/apply",{method:"POST",headers:{"Content-Type":"application/json","X-Ding-CSRF":s.data.csrf},body:"{}"})).status;});expect(forbidden).toBe(403);
    await page.getByRole("listitem").filter({hasText:"Reader"}).getByRole("button",{name:"Disable",exact:true}).click();
    expect((await visitor.request.get(url+"/v1/watches")).status()).toBe(401);
    await visitor.reload();await expect(visitor.getByRole("heading",{name:"Sign in to Team Ding."})).toBeVisible();
  } finally {
    await reader.close();child.kill("SIGTERM");await new Promise<void>(r=>{if(child.exitCode!==null)r();else child.once("exit",()=>r());});
    await new Promise<void>(r=>proxy.close(()=>r()));rmSync(dir,{recursive:true,force:true});
  }
});
