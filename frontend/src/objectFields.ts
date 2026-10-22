import type { Attributes, CatalogObject } from "./api";
export const fieldLabels: Record<string, string> = {
  name: "Название",
  address: "IPv4 / адрес",
  os: "Операционная система",
  environment: "Тип эксплуатации",
  status: "Состояние",
  dbms: "Тип СУБД",
  dbmsVersion: "Версия СУБД",
  port: "Порт",
  purpose: "Назначение",
  "attributes.informationSystem": "Информационная система",
  "attributes.connectionMethod": "Подключение к серверу",
  "attributes.domain": "Доменное членство",
  "attributes.location": "Месторасположение",
  "attributes.serviceOrganization": "Обслуживающая организация",
  "attributes.supervisor": "Сопровождение (руководитель)",
  "attributes.responsible": "Ответственное лицо",
  "attributes.osVersion": "Версия системы",
  "attributes.osFamily": "Семейство системы",
  "attributes.administration": "Работа АБД",
  "attributes.personalData": "Персональные данные",
  description: "Описание",
  parentId: "Родительский объект",
  version: "Версия записи",
};
export function objectValue(
  o: CatalogObject | null | undefined,
  key: string,
): string {
  if (!o) return "";
  const v = key.startsWith("attributes.")
    ? o.attributes?.[key.slice(11) as keyof Attributes]
    : o[key as keyof CatalogObject];
  return Array.isArray(v) ? v.join("; ") : v == null ? "" : String(v);
}
