import {readFileSync,readdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {gzipSync} from 'node:zlib';
const dir=resolve(import.meta.dirname,'../../../internal/webui/dist/assets');
for(const name of readdirSync(dir).filter(n=>n.startsWith('index-')&&n.endsWith('.js'))){const size=gzipSync(readFileSync(resolve(dir,name))).byteLength;console.log(`Initial JavaScript: ${(size/1024).toFixed(1)} KiB gzip (budget: 250 KiB)`);if(size>250*1024)process.exitCode=1;}
