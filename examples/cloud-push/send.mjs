// node send.mjs HTTPS_URL STABLE_KEY EXISTING_JSON_FILE
// Keep the same key and exact bytes when restarting after an uncertain response.
import { readFile } from "node:fs/promises";
import { setTimeout as sleep } from "node:timers/promises";
const [url,key,path]=process.argv.slice(2);
if(new URL(url).protocol!=="https:") throw Error("Use an HTTPS ingest URL");
const body=await readFile(path);
if(body.length>1048576) throw Error("Body exceeds 1 MiB");
if(!process.env.DING_PRODUCER_TOKEN) throw Error("DING_PRODUCER_TOKEN is required");
for(let attempt=0;attempt<5;attempt++){
 let delay=2**attempt, response;
 try{
  response=await fetch(url,{method:"POST",redirect:"error",signal:AbortSignal.timeout(15000),headers:{Authorization:`Bearer ${process.env.DING_PRODUCER_TOKEN}`,"Content-Type":"application/json","Idempotency-Key":key},body});
 }catch{}
 if(response?.status===202){console.log(JSON.stringify(await response.json()));break;}
 if(response&&!([429,503].includes(response.status))) throw Error(`HTTP ${response.status}; request ${response.headers.get("X-Ding-Request-ID")}`);
 const retry=Number(response?.headers.get("Retry-After"));
 if(Number.isFinite(retry))delay=Math.max(delay,Math.min(retry,30));
 await response?.body?.cancel();
 if(attempt===4)throw Error("Acceptance unconfirmed: retain this exact file and key");
 await sleep((delay+Math.random())*1000);
}
