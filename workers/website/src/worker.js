import redirects from '../../../content/redirects.json' with { type: 'json' };
import product from '../../../content/product.json' with { type: 'json' };

import { homepageFunnel, expireFunnel } from './funnel.js';

export default {
  async scheduled(_event, env) {
    await expireFunnel(env.FUNNEL);
  },
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.hostname === 'www.ding.ing') {
      url.hostname = 'ding.ing';
      url.protocol = 'https:';
      return Response.redirect(url.toString(), 301);
    }
    if (url.hostname === 'ding.ing' && env.FUNNEL) {
      const response = await homepageFunnel(request, env);
      if (response) return response;
    }
    const destination = redirects[url.pathname.replace(/\/+$/, '')];
    if (destination) {
      const target = new URL(destination);
      if (env.PREVIEW === 'true') target.host = new URL(product.previewDocsUrl).host;
      target.search = url.search;
      return Response.redirect(target.toString(), 301);
    }
    return env.ASSETS.fetch(request);
  },
};
