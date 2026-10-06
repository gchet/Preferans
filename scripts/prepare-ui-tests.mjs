import {mkdirSync, writeFileSync} from 'node:fs';
const network = {
  roomServiceURL: 'http://signaling.example.org:8080/v2/rooms',
  turnServer: 'turn:signaling.example.org:3478?transport=udp',
  turnUser: 'preferans',
  legacySignalingURL: 'http://signaling.example.org:8080/v1/singleton',
};
mkdirSync('.build', {recursive:true});
mkdirSync('config/local', {recursive:true});
writeFileSync('.build/ui-test-config.json', JSON.stringify({network},null,2));
writeFileSync('config/local/build.json', JSON.stringify(network,null,2));
