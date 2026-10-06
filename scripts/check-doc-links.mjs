import {readFileSync, existsSync, readdirSync} from 'node:fs';
import {resolve, dirname} from 'node:path';
const files = [
  ...readdirSync('.').filter(f=>f.endsWith('.md')),
  ...readdirSync('docs').filter(f=>f.endsWith('.md')).map(f=>'docs/'+f),
  'config/README.md', 'web/public/cards/README.md',
];
let bad=0;
for(const file of files) {
  const text=readFileSync(file,'utf8');
  for(const m of text.matchAll(/\]\(([^)]+)\)/g)) {
    let link=m[1].trim().split('#')[0];
    if(!link || /^[a-z]+:/i.test(link))continue;
    link=decodeURIComponent(link.replace(/^<|>$/g,''));
    if(!existsSync(resolve(dirname(file),link))) {
      console.error(file+': missing '+link);bad++;
    }
  }
}
if(bad)process.exit(1);
console.log('Documentation links OK ('+files.length+' files)');
