import { useEffect, useState } from "react";
import { api, type Diagnostics } from "./api";

const states: Record<string,string> = {
 connected:"Подключено", disconnected:"Нет подключения", error:"Ошибка проверки",
 valid:"Настройки доступны", missing:"Настройки ещё не сохранены", available:"Доступно при последнем обращении",
 unexamined:"Обращений ещё не было", current:"Все миграции применены, SHA-256 совпадают",
 unavailable:"Нет данных", attention:"Нужна проверка миграций", stopped:"Не запущен",
 connecting:"Подключается", listening:"Слушает уведомления", retrying:"Повторное подключение",
 applied:"Применена", pending:"Не применена", mismatch:"Не совпадает SHA-256", unknown:"Неизвестна этой версии приложения",
};
const label = (state: string) => states[state] ?? "Нет данных";
const date = (value: string) => value ? new Date(value).toLocaleString("ru") : "Нет данных";
export function DiagnosticsPanel({busy,run}:{busy:boolean;run:(fn:()=>Promise<void>)=>Promise<void>}) {
 const [data,setData]=useState<Diagnostics|null>(null);
 const [message,setMessage]=useState("");
 useEffect(()=>{let active=true;void run(async()=>{const d=await api().GetDiagnostics();if(active)setData(d)});return()=>{active=false}},[]);
 const refresh=()=>run(async()=>{setMessage("");setData(await api().GetDiagnostics())});
 const field=(title:string,value:string|number)=><div><dt>{title}</dt><dd>{value || "Нет данных"}</dd></div>;
 return <>
 <div className="page-heading"><div><div className="eyebrow">СОСТОЯНИЕ ПРИЛОЖЕНИЯ</div><h1>Диагностика</h1><p className="muted">Доступна без входа и подключения к каталогу.</p></div>
 <div className="actions"><button disabled={busy} onClick={()=>void refresh()}>↻ Обновить</button><button className="primary" disabled={busy || !data} onClick={()=>void run(async()=>{setMessage("");const saved=await api().SaveDiagnosticsReport();setMessage(saved?"Отчёт сохранён":"Сохранение отменено")})}>Сохранить отчёт JSON</button></div></div>
 <p className="info">Отчёт не содержит паролей, токенов, адресов серверов, имён пользователей, путей и содержимого записей. Проверка только читает состояние и не применяет миграции.</p>
 {message && <p role="status">{message}</p>}
 {data && <>
 <p className="muted">Проверено: {date(data.generatedAt)}</p>
 <div className="diagnostics-grid">
 <section className="settings-card"><h2>Приложение</h2><dl className="diagnostics-fields">{field("Версия CatalogDB",data.appVersion)}{field("Платформа",data.platform)}{field("Go",data.goVersion)}{field("Wails",data.wailsVersion)}{field("Ревизия исходников",data.revision)}{field("Изменения при сборке",!data.revision?"Нет данных":data.modified?"Есть незакоммиченные изменения":"Нет")}{field("Настройки подключения",label(data.configState))}</dl></section>
 <section className="settings-card"><h2>PostgreSQL</h2><dl className="diagnostics-fields">{field("Подключение",label(data.databaseState))}{field("Версия сервера",data.postgresqlVersion)}{field("TLS",data.databaseState==="connected"?(data.tls?"Используется":"Не используется"):"Нет данных")}{field("Режим сервера",data.databaseState==="connected"?(data.readOnly?"Только чтение / реплика":"Чтение и запись"):"Нет данных")}{field("Соединения пула",`${data.poolConnections} / ${data.poolMaximum}`)}</dl></section>
 <section className="settings-card"><h2>Обновления LISTEN/NOTIFY</h2><dl className="diagnostics-fields">{field("Слушатель",label(data.listener.state))}{field("Подключён",date(data.listener.connectedAt))}{field("Последнее событие каталога",date(data.listener.lastEventAt))}</dl><p className="muted">Отсутствие событий не означает сбой: в каталоге могло не быть изменений. При разрыве приложение переподключается автоматически.</p></section>
 <section className="settings-card"><h2>Системное хранилище</h2><dl className="diagnostics-fields">{field("Служба",data.credentialBackend)}{field("Последнее обращение",label(data.credentialState))}{field("Время обращения",date(data.credentialCheckedAt))}</dl><p className="muted">Показывается результат последней операции приложения. Отдельный запрос к хранилищу и разблокировка при просмотре диагностики не выполняются.</p></section>
 </div>
 <section className="settings-card"><h2>Миграции схемы каталога</h2><p>{label(data.migrationState)}</p>{data.migrations.length>0 && <div className="table-scroll"><table><thead><tr><th>Версия</th><th>Состояние</th></tr></thead><tbody>{data.migrations.map(m=><tr key={m.version}><td>{String(m.version).padStart(3,"0")}</td><td>{label(m.state)}</td></tr>)}</tbody></table></div>}</section>
 </>}
 </>;
}
