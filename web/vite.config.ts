import {defineConfig} from 'vite';
import {readFileSync, existsSync, writeFileSync} from 'node:fs';
const path = new URL('../config/local/build.json', import.meta.url);
const network = existsSync(path) ? JSON.parse(readFileSync(path, 'utf8')) : {};
export default defineConfig({
  define: {__BUILD_NETWORK__: JSON.stringify(network), __APP_VERSION__: JSON.stringify(process.env.PREFERANS_VERSION || 'dev')},
  plugins: [{name:'keep-web-directory',closeBundle(){writeFileSync('dist/.gitkeep','');}}],
});
