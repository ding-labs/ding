const natural = value => Number.isSafeInteger(value) && value >= 0;
const date = value => typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value);
const archive = /^ding_(?:darwin|linux|windows)_(?:amd64|arm64)\.(?:tar\.gz|zip)$/;
const bottle = /^ding-[\w.-]+\.bottle\.tar\.gz$/;

export async function storeDownloadReading(db, data) {
  const reject = () => new Response(null, {status:400});
  const now = Math.floor(Date.now()/1000);
  if (!data || !natural(data.observed) || Math.abs(now-data.observed) > 3600) return reject();
  if (data.source === 'homebrew') {
    if (!date(data.start_date) || !date(data.end_date) || data.start_date >= data.end_date || !natural(data.installs)) return reject();
    await db.prepare(`INSERT INTO brew_download_report VALUES(1,?,?,?,?)
      ON CONFLICT(singleton) DO UPDATE SET observed=excluded.observed,start_date=excluded.start_date,
      end_date=excluded.end_date,installs=excluded.installs WHERE excluded.observed >= brew_download_report.observed
      AND excluded.end_date >= brew_download_report.end_date`)
      .bind(data.observed,data.start_date,data.end_date,data.installs).run();
  } else if (data.source === 'github') {
    if (!Array.isArray(data.assets) || !data.assets.length || !Array.isArray(data.fetches)) return reject();
    const ids = new Set();
    for (const asset of data.assets) {
      if (!asset || !natural(asset.id) || ids.has(asset.id) || !natural(asset.created) || !natural(asset.count) ||
          typeof asset.tag !== 'string' || !(asset.kind === 'archive' && archive.test(asset.name) ||
          asset.kind === 'bottle' && bottle.test(asset.name))) return reject();
      ids.add(asset.id);
    }
    for (const job of data.fetches) {
      if (!job || !natural(job.id) || !natural(job.fetched) || typeof job.tag !== 'string' || !archive.test(job.name)) return reject();
    }
    await db.batch([
      db.prepare('INSERT OR IGNORE INTO github_download_samples VALUES (?,?)').bind(data.observed,JSON.stringify(data.assets)),
      db.prepare(`INSERT OR IGNORE INTO release_job_fetches(job,tag,filename,fetched)
        SELECT value->>'id',value->>'tag',value->>'name',value->>'fetched' FROM json_each(?)`).bind(JSON.stringify(data.fetches)),
      db.prepare("DELETE FROM github_download_samples WHERE observed < unixepoch('now','-36 days')"),
      db.prepare("DELETE FROM release_job_fetches WHERE fetched < unixepoch('now','-36 days')"),
    ]);
  } else return reject();
  return new Response(null, {status:204});
}
