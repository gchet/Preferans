import fs from 'node:fs';
const settings=JSON.parse(fs.readFileSync(new URL('../config/interface.json',import.meta.url),'utf8'));
for(const [key,value] of Object.entries(settings)) {
  if(key==='_comments') {
    if(!value || typeof value!=='object' || Array.isArray(value) || Object.values(value).some(text=>typeof text!=='string'))throw Error('Invalid interface comments');
    continue;
  }
  if(key==='roomServiceURL') {
    if(value!=='' && !['http:','https:'].includes(new URL(value).protocol))throw Error('Invalid room service URL');
    continue;
  }
  if(typeof value!=='number' || !Number.isFinite(value) || value<=0 || value>2147483647 || (key.endsWith('Ms') && !Number.isInteger(value)))throw Error(`Invalid interface parameter: ${key}`);
}
if(settings.cardMinHeightPx>settings.cardMaxHeightPx || settings.cardStairMinPx>settings.cardStairMaxPx)throw Error('Minimum exceeds maximum');
console.log('Interface parameters OK.');
