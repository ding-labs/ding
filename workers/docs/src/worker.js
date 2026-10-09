export default {
  async fetch(request, env) {
    if (env.PREVIEW === 'true' && new URL(request.url).pathname === '/robots.txt') {
      return new Response('User-agent: *\nDisallow: /\n', {
        headers: {'Content-Type':'text/plain; charset=utf-8','X-Robots-Tag':'noindex, follow'},
      });
    }
    const response = await env.ASSETS.fetch(request);
    if (env.PREVIEW !== 'true') return response;
    const result = new Response(response.body, response);
    result.headers.set('X-Robots-Tag', 'noindex, follow');
    return result;
  },
};
