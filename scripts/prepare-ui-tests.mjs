import {mkdirSync, writeFileSync} from 'node:fs';
const network = {updateAddressHint: 'http://updates.example.org:8787'};
mkdirSync('.build', {recursive:true});
mkdirSync('config/local', {recursive:true});
writeFileSync('.build/ui-test-config.json', JSON.stringify({network},null,2));
writeFileSync('config/local/build.json', JSON.stringify(network,null,2));
