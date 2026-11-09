import { signupEvent } from "./signup.js";

const login = "https://cloud.ding.ing/auth/login";
const origin = "https://ding.ing";
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const noStore = {"Cache-Control": "private, no-store"};
const optOut = "__Host-ding-funnel-off=1; Path=/; Max-Age=31536000; Secure; SameSite=Lax";

function cookie(request, name) {
  return (request.headers.get("Cookie") || "").split(";").map(x => x.trim())
    .find(x => x.startsWith(name + "="))?.slice(name.length + 1);
}

function eligible(request) {
  return request.headers.get("Sec-GPC") !== "1" && !request.cf?.botManagement?.verifiedBot &&
    !/bot\b|crawler|spider|headless|lighthouse|pagespeed|prerender/i.test(request.headers.get("User-Agent") || "") &&
    !/prefetch|prerender/i.test([request.headers.get("Purpose"), request.headers.get("Sec-Purpose")].join(" ")) &&
    cookie(request, "__Host-ding-funnel-off") !== "1";
}

function fromHomepage(request) {
  try {
    const ref = new URL(request.headers.get("Referer"));
    return ref.origin === origin && ref.pathname === "/";
  } catch { return false; }
}

function toLogin(handoff) {
  const target = new URL(login);
  if (handoff) target.searchParams.set("homepage_handoff", handoff);
  return new Response(null, {status: 302, headers: {...noStore, Location: target.href, "Referrer-Policy": "no-referrer"}});
}

export async function homepageFunnel(request, env) {
  const {pathname} = new URL(request.url);
  if (pathname === "/_funnel/workos" || pathname === "/_funnel/verified" || pathname === "/_funnel/report") return signupEvent(request, env, pathname);
  const browser = cookie(request, "__Host-ding-visit");
  if (pathname === "/go/cloud") {
    if (request.method !== "GET" || !uuid.test(browser) || !eligible(request) || !fromHomepage(request) ||
        request.headers.get("Sec-Fetch-Dest") !== "document") return toLogin();
    const handoff = crypto.randomUUID();
    try {
      await env.FUNNEL.batch([
        env.FUNNEL.prepare("INSERT OR IGNORE INTO homepage_landings VALUES (?,date('now'))").bind(browser),
        env.FUNNEL.prepare("INSERT INTO homepage_clicks(handoff,browser,clicked_at) VALUES (?,?,unixepoch())").bind(handoff,browser),
      ]);
      return toLogin(handoff);
    } catch {
      console.error("Homepage click measurement unavailable");
      return toLogin();
    }
  }
  if (pathname === "/_funnel/land") {
    if (request.method === "POST" && request.headers.get("Origin") === origin && fromHomepage(request) && eligible(request) && uuid.test(browser)) {
      await env.FUNNEL.prepare("INSERT OR IGNORE INTO homepage_landings VALUES (?,date('now'))").bind(browser).run();
    }
    return new Response(null, {status: 204, headers: noStore});
  }
  if (pathname === "/analytics/exclude") {
    const headers = new Headers({...noStore, "Content-Type": "text/html; charset=utf-8", "X-Robots-Tag": "noindex", "Content-Security-Policy": "default-src 'none'; form-action 'self'; frame-ancestors 'none'"});
    let excluded = cookie(request,"__Host-ding-funnel-off") === "1";
    if (request.method === "POST" && request.headers.get("Origin") === origin) {
      if (uuid.test(browser)) await env.FUNNEL.prepare("INSERT OR REPLACE INTO excluded_browsers VALUES (?,unixepoch())").bind(browser).run();
      headers.append("Set-Cookie", optOut);
      headers.append("Set-Cookie", "__Host-ding-visit=; Path=/; Max-Age=0; Secure; SameSite=Lax");
      excluded = true;
    }
    return new Response(`<!doctype html><meta charset=utf-8><title>Ding analytics preference</title><h1>Homepage analytics</h1>${excluded ? "<p>This browser is excluded from the Ding homepage funnel.</p>" : '<form method="post"><button>Exclude this browser</button></form>'}`, {headers});
  }
  return null;
}

export async function expireFunnel(db) {
  await db.batch([
    db.prepare("DELETE FROM homepage_landings WHERE day < date('now','-30 days')"),
    db.prepare("DELETE FROM homepage_clicks WHERE clicked_at < unixepoch('now','-31 days')"),
    db.prepare("DELETE FROM excluded_browsers WHERE last_seen < unixepoch('now','-62 days')"),
    db.prepare("DELETE FROM cloud_signups WHERE created_at < unixepoch('now','-31 days')"),
  ]);
}
