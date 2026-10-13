const installGuide = "https://docs.ding.ing/install/";

export default {
  async fetch(request) {
    const acceptsHtml = request.headers.get("accept")?.includes("text/html");
    if ((request.method === "GET" || request.method === "HEAD") && acceptsHtml) {
      return Response.redirect(installGuide, 301);
    }

    return new Response(`The start.ding.ing shell installer has been retired.\nInstall Ding using ${installGuide}\n`, {
      status: 410,
      headers: {
        "content-type": "text/plain; charset=utf-8",
        "cache-control": "no-store",
      },
    });
  },
};
