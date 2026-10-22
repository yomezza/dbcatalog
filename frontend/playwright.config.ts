import {defineConfig} from '@playwright/test';
export default defineConfig({
 testDir:'./tests',
 use:{baseURL:'http://127.0.0.1:4173',launchOptions:process.env.CATALOGDB_CHROMIUM?{executablePath:process.env.CATALOGDB_CHROMIUM,args:['--no-sandbox']}:{}},
 webServer:{command:'npm run build && npm exec vite -- preview --host 127.0.0.1 --port 4173 --strictPort',port:4173,reuseExistingServer:false},
 reporter:'list'
});
