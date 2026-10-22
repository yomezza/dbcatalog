import {test,expect} from '@playwright/test';

// Browser contract tests simulate only the Wails bridge. PostgreSQL and Windows
// integration are exercised separately; this is not a replacement for Windows QA.
test.beforeEach(async({page})=>{
 await page.addInitScript(()=>{
  const g=window as unknown as {go:unknown;__connection:unknown};
  g.go={main:{App:{
   GetConnection:async()=>({connection:{host:'localhost',port:5432,database:'catalogdb',username:'catalogdb',schema:'dbcatalog',sslMode:'verify-full',sslRootCert:''},hasPassword:false}),
   IsSetupMode:async()=>false,
   Connect:async(connection:unknown)=>{g.__connection=connection;return {connected:true,schema:(connection as {schema:string}).schema,needsAdmin:true,user:null}}
  }}};
 });
 await page.goto('/');
});
test('default schema can be changed and reaches the Go bridge',async({page})=>{
 const schema=page.getByLabel('Схема БД');await expect(schema).toHaveValue('dbcatalog');await schema.fill('team_catalog');
 await page.getByRole('button',{name:'Проверить, обновить схему и подключиться'}).click();
 await expect(page.getByRole('heading',{name:'Первый администратор'})).toBeVisible();
 expect(await page.evaluate(()=> (window as unknown as {__connection:{schema:string}}).__connection.schema)).toBe('team_catalog');
 await expect(page.getByText('Схема: team_catalog')).toBeVisible();
});
test('invalid schema is rejected before connecting',async({page})=>{
 await page.getByLabel('Схема БД').fill('x;drop schema public');
 await page.getByRole('button',{name:'Проверить, обновить схему и подключиться'}).click();
 expect(await page.getByLabel('Схема БД').evaluate((input:HTMLInputElement)=>input.validity.patternMismatch)).toBe(true);
 expect(await page.evaluate(()=> (window as unknown as {__connection?:unknown}).__connection)).toBeUndefined();
});
