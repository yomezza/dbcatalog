package application

import (
	"fmt"
	"github.com/yomezza/test/internal/documents"
	"strings"
)

func (s *Service) InstructionTemplate(kind, id, template string) (documents.Instruction, error) {
	c, err := s.GetObjectContext(kind, id)
	if err != nil {
		return documents.Instruction{}, err
	}
	in := documents.Instruction{Template: template, Title: "Инструкция: " + c.Object.Name, Prerequisites: "Подтвердить согласованное окно работ и наличие необходимых прав.", Verification: "Зафиксировать фактический результат и время выполнения.", Rollback: "Указать проверенный порядок отката до начала работ."}
	switch template {
	case "connection":
		in.Title = "Подключение: " + c.Object.Name
		in.Purpose = "Порядок доступа к " + c.Path
		in.Steps = "1. Проверить доступность сервера.\n2. Открыть согласованный клиент подключения.\n3. Выполнить подключение под своей учётной записью.\n4. Проверить выбранный сервер, экземпляр и БД."
	case "backup":
		in.Title = "Резервное копирование: " + c.Object.Name
		in.Purpose = "Порядок создания и проверки резервной копии."
		in.Steps = "1. Проверить актуальную политику РК.\n2. Указать используемую команду или задание копирования.\n3. Выполнить копирование в согласованном порядке.\n4. Проверить журнал, файл копии и возможность восстановления."
	case "restore":
		in.Title = "Восстановление: " + c.Object.Name
		in.Purpose = "Согласованный порядок восстановления из резервной копии."
		in.Steps = "1. Выбрать и проверить резервную копию.\n2. Согласовать целевой сервер и последствия восстановления.\n3. Указать точные команды для используемой СУБД.\n4. Выполнить восстановление.\n5. Проверить целостность и доступ приложения."
	case "maintenance":
		in.Title = "Регламент: " + c.Object.Name
		in.Purpose = "Порядок обслуживания объекта."
		in.Steps = "1. Уточнить состав работ.\n2. Зафиксировать исходное состояние.\n3. Описать и выполнить согласованные действия.\n4. Проверить результат."
	default:
		return documents.Instruction{}, fmt.Errorf("Неизвестный шаблон инструкции")
	}
	var policy *Policy
	for i := range c.Policies {
		p := &c.Policies[i]
		if p.ObjectID == c.Object.ID {
			policy = p
			break
		}
		if p.Kind == "instance" {
			policy = p
		}
	}
	if policy != nil {
		in.Notes = strings.Join([]string{"Политика РК: " + policy.Name, "Тип: " + policy.BackupType, "Планировщик: " + policy.Scheduler, "Расписание: " + policy.Schedule, "Путь: " + policy.Storage, fmt.Sprintf("Хранение: %d дней", policy.RetentionDays)}, "\n")
	}
	return in, nil
}
