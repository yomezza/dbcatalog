const tones: Record<string, string> = {
  Подключено: "success",
  "Не подключено": "neutral",
  Промышленный: "production",
  Тестовый: "testing",
  Эксплуатируется: "success",
  "Выведена из эксплуатации": "neutral",
  Заморожена: "info",
  Устаревшая: "warning",
  Справочник: "purple",
  "К удалению": "danger",
  Актуализировать: "warning",
  Архив: "neutral",
  Разрабатывается: "info",
  Отсоединена: "neutral",
  PostgreSQL: "info",
  "Oracle Database": "danger",
  "MySQL/MariaDB": "production",
  Firebird: "warning",
  "Microsoft SQL Server": "purple",
  RDP: "info",
  SSH: "production",
  отсутствует: "neutral",
  "oms.admsurgut.ru": "success",
};
export function ValueBadge({
  value,
  category = "",
}: {
  value: string;
  category?: string;
}) {
  let tone = tones[value] ?? "neutral";
  if (category === "personalData") {
    if (value.toLocaleLowerCase("ru") === "да") tone = "warning";
    else if (value.toLocaleLowerCase("ru") === "нет") tone = "success";
  }
  return (
    <span className={"value-badge " + tone}>
      <span className="badge-dot" aria-hidden="true" />
      {value}
    </span>
  );
}
