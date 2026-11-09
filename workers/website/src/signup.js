const encoder = new TextEncoder();
const hex = bytes => Array.from(new Uint8Array(bytes), byte => byte.toString(16).padStart(2, "0")).join("");
const invalid = status => new Response(null, {status, headers: {"Cache-Control":"no-store"}});

async function signedBody(request, header, secret) {
  if (!secret) return invalid(503);
  const match = /^t=(\d{13}),\s*v1=([a-f0-9]{64})$/.exec(request.headers.get(header) || "");
  if (!match || Math.abs(Date.now() - Number(match[1])) > 300_000) return invalid(401);
  const reader = request.body?.getReader();
  if (!reader) return invalid(400);
  const decoder = new TextDecoder();
  let body = "", size = 0;
  while (true) {
    const {done, value} = await reader.read();
    if (done) break;
    size += value.length;
    if (size > 128 * 1024) { await reader.cancel(); return invalid(413); }
    body += decoder.decode(value, {stream: true});
  }
  body += decoder.decode();
  const key = await crypto.subtle.importKey("raw", encoder.encode(secret), {name:"HMAC",hash:"SHA-256"}, false, ["verify"]);
  const signature = Uint8Array.from(match[2].match(/../g), pair => parseInt(pair, 16));
  if (!await crypto.subtle.verify("HMAC", key, signature, encoder.encode(match[1] + "." + body))) return invalid(401);
  try { return JSON.parse(body) ?? invalid(400); } catch { return invalid(400); }
}

export async function signupEvent(request, env, path) {
  if (request.method !== "POST") return invalid(405);
  if (path === "/_funnel/workos") {
    const event = await signedBody(request, "WorkOS-Signature", env.FUNNEL_WORKOS_SECRET);
    if (event instanceof Response) return event;
    if (event.event !== "user.created") return invalid(204);
    const user = event.data;
    const created = Date.parse(user?.created_at ?? event.created_at);
    if (user?.object !== "user" || !/^user_[A-Za-z0-9]+$/.test(user.id) || !Number.isFinite(created)) return invalid(400);
    const account = hex(await crypto.subtle.digest("SHA-256", encoder.encode(user.id)));
    await env.FUNNEL.prepare("INSERT OR IGNORE INTO cloud_signups(account,created_at) VALUES (?,?)")
      .bind(account, Math.floor(created/1000)).run();
  } else {
    const receipt = await signedBody(request, "Ding-Signature", env.FUNNEL_RECEIPT_SECRET);
    if (receipt instanceof Response) return receipt;
    if (!/^[a-f0-9]{64}$/.test(receipt.account)) return invalid(400);
    if (path === "/_funnel/report") {
      if (receipt.action !== "report") return invalid(400);
      const allowed = await env.FUNNEL.prepare("SELECT 1 FROM report_readers WHERE account=?").bind(receipt.account).first();
      if (!allowed) return invalid(403);
      const totals = await env.FUNNEL.prepare("SELECT * FROM homepage_funnel_30d").first();
      return Response.json(totals, {headers: {"Cache-Control": "private, no-store"}});
    }
    if (receipt.action !== "verified" || !/^[a-f0-9-]{36}$/.test(receipt.handoff) || !/^[a-f0-9]{64}$/.test(receipt.account)) return invalid(400);
    // The handoff is single-use. Retries are harmless, including webhook-first delivery.
    await env.FUNNEL.batch([
      env.FUNNEL.prepare("UPDATE homepage_clicks SET account=?, verified_at=unixepoch() WHERE handoff=? AND account IS NULL AND clicked_at >= unixepoch()-900").bind(receipt.account, receipt.handoff),
      env.FUNNEL.prepare(`INSERT OR REPLACE INTO excluded_browsers
        SELECT browser,unixepoch() FROM homepage_clicks WHERE handoff=? AND account IN (SELECT account FROM excluded_accounts)`).bind(receipt.handoff),
    ]);
  }
  return invalid(204);
}
