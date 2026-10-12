import {readFile,writeFile,readdir} from 'node:fs/promises';
import path from 'node:path';
import {report} from '../worker/src/report.mjs';
const root=process.argv[2];if(!root)throw Error('Usage: node report.mjs PRIVATE_EVIDENCE_DIRECTORY');
const input={config:JSON.parse(await readFile(path.join(root,'config.json'),'utf8')),samples:[],fixtures:[],events:[]};
for(const type of ['samples','fixtures','events']) {
  for(const name of await readdir(path.join(root,type)).catch(()=>[]))if(name.endsWith('.json'))input[type].push(JSON.parse(await readFile(path.join(root,type,name),'utf8')));
}
for(const type of ['maintenance','drills','safety']){try{input[type]=JSON.parse(await readFile(path.join(root,type+'.json'),'utf8'));}catch(e){if(e.code!=='ENOENT')throw e;}}
const output=report(input);await writeFile(path.join(root,'report.json'),JSON.stringify(output,null,2)+'\n',{mode:0o600});
console.log(JSON.stringify({runId:output.runId,mode:output.mode,elapsedHours:output.elapsedHours,ready:output.ready,verdict:output.verdict,gates:Object.fromEntries(Object.entries(output.gates).map(([k,v])=>[k,v.verdict])),issueCount:output.issues.length},null,2));
