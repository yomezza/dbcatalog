import {test,expect} from "@playwright/test";

test.beforeEach(async({page})=>{
 await page.addInitScript(()=>{
  let calls=0;
  const root=window as unknown as {go:unknown};
  root.go={main:{App:{
   GetConnection:async()=>({connection:{host:"localhost",port:5432,database:"catalogdb",username:"catalogdb",schema:"dbcatalog",sslMode:"verify-full",sslRootCert:""},hasPassword:false}),
   IsSetupMode:async()=>false,
   GetDiagnostics:async()=>({generatedAt:"2026-10-23T07:00:00Z",appVersion:"0.11.0",goVersion:"go1.27.3",wailsVersion:"v2.17.0",platform:"linux/amd64",revision:"demo-revision",modified:false,configState:"valid",credentialBackend:"Secret Service (D-Bus)",credentialState:"unexamined",credentialCheckedAt:"",databaseState:++calls>1?"connected":"disconnected",postgresqlVersion:calls>1?"17.11":"",tls:false,readOnly:false,poolConnections:1,poolMaximum:4,migrationState:calls>1?"attention":"unavailable",migrations:calls>1?[{version:1,state:"mismatch"}]:[],listener:{state:"stopped",connectedAt:"",lastEventAt:""}}),
   SaveDiagnosticsReport:async()=>false,
  }}};
 });
 await page.goto("/");
});

test("diagnostics works before login, refreshes migration results and handles save cancellation",async({page})=>{
 await page.getByRole("button",{name:"Диагностика"}).click();
 await expect(page.getByRole("heading",{name:"Диагностика",exact:true})).toBeVisible();
 await expect(page.getByText("linux/amd64",{exact:true})).toBeVisible();
 await expect(page.getByText("Обращений ещё не было",{exact:true})).toBeVisible();
 const versionRow=page.getByText("Версия CatalogDB",{exact:true}).locator("..");
 const value=versionRow.locator("dd");
 const box=await value.boundingBox();
 expect(box?.width).toBeGreaterThan(100);
 expect(box?.height).toBeLessThan(40);
 await page.getByRole("button",{name:"↻ Обновить",exact:true}).click();
 await expect(page.getByText("17.11",{exact:true})).toBeVisible();
 await expect(page.getByText("Не совпадает SHA-256",{exact:true})).toBeVisible();
 await page.getByRole("button",{name:"Сохранить отчёт JSON"}).click();
 await expect(page.getByText("Сохранение отменено",{exact:true})).toBeVisible();
});
