import {defineConfig} from 'vite';
import {writeFileSync} from 'node:fs';
export default defineConfig({base:'/admin/',plugins:[{name:'keep-admin-directory',closeBundle(){writeFileSync('../internal/adminweb/static/.gitkeep','');}}],build:{outDir:'../internal/adminweb/static',emptyOutDir:true,rollupOptions:{input:'admin.html'}}});
