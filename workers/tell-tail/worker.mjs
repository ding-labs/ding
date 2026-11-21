export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.hostname === "www.tell-tail.dev") {
      url.host = "tell-tail.dev";
      url.protocol = "https:";
      return Response.redirect(url, 301);
    }
    if (env.PREVIEW !== "true") return env.ASSETS.fetch(request);
    if (url.pathname === "/robots.txt") {
      return new Response("User-agent: *\nDisallow: /\n", { headers: { "Content-Type": "text/plain" } });
    }
    const asset = await env.ASSETS.fetch(request);
    const response = new Response(asset.body, asset);
    response.headers.set("X-Robots-Tag", "noindex, follow");
    return response;
  },
};
