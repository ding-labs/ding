import tellTail from '../../../content/tell-tail.json' with { type: 'json' };

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === '/tell-tail' || url.pathname.startsWith('/tell-tail/')) {
      const target = new URL(env.PREVIEW === 'true' ? tellTail.previewWebsiteUrl : tellTail.websiteUrl);
      const suffix = url.pathname.slice('/tell-tail/'.length).replace(/(^|\/)index\.html$/, '$1');
      target.pathname = '/docs/' + suffix;
      target.search = url.search;
      return Response.redirect(target, 301);
    }
    if (env.PREVIEW === 'true' && url.pathname === '/robots.txt') {
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
