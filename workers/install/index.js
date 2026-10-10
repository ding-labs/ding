export default {
  async fetch() {
    const scriptUrl =
      "https://raw.githubusercontent.com/ding-labs/ding/main/scripts/install.sh";
    const response = await fetch(scriptUrl);
    if (!response.ok) {
      return new Response("Ding installer is temporarily unavailable. Try the official GitHub releases.\n", {
        status: 503,
        headers: { "content-type": "text/plain; charset=utf-8", "cache-control": "no-store" },
      });
    }
    return new Response(response.body, {
      headers: {
        "content-type": "text/plain; charset=utf-8",
        "cache-control": "no-cache",
      },
    });
  },
};
