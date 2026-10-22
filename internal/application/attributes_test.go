package application

import (
	"reflect"
	"testing"
)

func TestImportAttributeMergePreservesBlanksAndOwnsItsSlices(t *testing.T) {
	old := Attributes{InformationSystem: "ИС", ConnectionMethod: "SSH", Domain: "отсутствует", Location: "ЦОД", ServiceOrganization: "Организация", Supervisor: "Руководитель", Responsible: "Иван", OSVersion: "17", OSFamily: "Linux", PersonalData: "Нет", Administration: []string{"Мониторинг и контроль"}}
	merged := mergeAttributes(old, Attributes{})
	if !reflect.DeepEqual(old, merged) {
		t.Fatal("blank cells cleared existing attributes")
	}
	merged.Administration[0] = "Изменение"
	if old.Administration[0] == "Изменение" {
		t.Fatal("merge aliases existing attributes")
	}
	incoming := Attributes{Responsible: "Пётр", Administration: []string{"Резервное копирование"}}
	merged = mergeAttributes(old, incoming)
	if merged.Responsible != "Пётр" || merged.Supervisor != old.Supervisor || merged.Administration[0] != "Резервное копирование" {
		t.Fatal(merged)
	}
	merged.Administration[0] = "Изменение"
	if incoming.Administration[0] == "Изменение" {
		t.Fatal("merge aliases incoming attributes")
	}
	if got := mergeAttributes(Attributes{}, Attributes{}); got.Administration == nil {
		t.Fatal("empty list must remain a JSON array")
	}
}
