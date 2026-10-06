import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const read=name=>JSON.parse(fs.readFileSync(path.join(root,'locales',name+'.json'),'utf8'));
const ru=read('ru');
const parameters=text=>({
 slots:[...new Set(text.match(/\{p\d+\}/g)||[])].sort(),
 printf:text.match(/%(?:\[\d+\])?[-+# 0]*(?:\d+|\*)?(?:\.\d+)?[a-zA-Z%]/g)||[],
});
const failures=[];
for(const language of ['ru','uk','en']) {
 const catalog=read(language);
 for(const key of Object.keys(ru))if(!(key in catalog))failures.push(`${language}: missing translation ${key}`);
 for(const [key,value] of Object.entries(catalog)) {
  if(typeof value!=='string'||!value.trim()) {failures.push(`${language}: empty/non-string ${key}`);continue;}
  if(!(key in ru))failures.push(`${language}: unknown key ${key}`);
  else if(JSON.stringify(parameters(value))!==JSON.stringify(parameters(ru[key])))failures.push(`${language}: placeholders differ: ${key}`);
  if(/@@(?:EXPR|TRANS)\d+@@/.test(value))failures.push(`${language}: extraction token: ${key}`);
  if(/<\/?[a-z][^>]*>/i.test(value))failures.push(`${language}: HTML must stay in code: ${key}`);
 }
}
function visit(directory) {
 for(const entry of fs.readdirSync(path.join(root,directory),{withFileTypes:true})) {
  const relative=path.join(directory,entry.name);
  if(entry.isDirectory())visit(relative);
  else if(/\.(ts|go|kt|html|kts)$/.test(entry.name)&&!entry.name.endsWith('_test.go')) {
   const source=fs.readFileSync(path.join(root,relative),'utf8');
   const references=[...source.matchAll(/(?:\bt|\btext|locales\.(?:Text|Format|Errorf))\(["']((?:web|go|android)\.[^"']+)["']/g),...source.matchAll(/data-i18n(?:-[\w-]+)?="([^"]+)"/g)];
   for(const match of references)if(!(match[1] in ru))failures.push(`${relative}: missing ${match[1]}`);
  }
 }
}
for(const directory of ['web/src','internal','cmd','mobile','android/app/src'])visit(directory);
const shell=fs.readFileSync(path.join(root,'web/index.html'),'utf8');
for(const match of shell.matchAll(/data-i18n(?:-[\w-]+)?="([^"]+)"/g))if(!(match[1] in ru))failures.push(`index.html: missing ${match[1]}`);
if(failures.length) {console.error(failures.join('\n'));process.exitCode=1;}
else console.log(`Localization OK: ${Object.keys(ru).length} messages in each of ru/uk/en.`);
