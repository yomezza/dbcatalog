package spreadsheet

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/shakinm/xlsReader/xls"
	"github.com/xuri/excelize/v2"
)

var Headers = []string{"SID", "Экземпляр", "Состояние", "Тип эксплуатации", "Имя сервера", "Тип СУБД", "Версия СУБД", "Подключение к серверу", "Доменное членство", "IPv4-адрес", "Месторасположение", "Информационная система", "Обслуживающая организация", "Сопровождение (руководитель)", "Ответственное лицо", "Операционная система", "Версия системы", "Семейство системы", "Назначение", "Работа АБД", "Персональные данные", "Тип РК", "Путь РК", "Служба планировщика заданий", "Статус СРК", "Срок хранения РК", "Пароль от архива", "Прошлый пароль от архива"}

const MaxFileSize = 20 * 1024 * 1024
const MaxRows = 100000

type Row struct {
	Number     int
	Values     [26]string
	HasSecrets bool
}
type Workbook struct {
	Sheet       string
	Columns     int
	Rows        []Row
	IgnoredRows int
	SecretRows  int
	BackupRows  int
}

func normalized(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func parseSheet(name string, count int, cell func(int, int) string) (Workbook, bool, error) {
	var out Workbook
	out.Sheet = name
	if count > MaxRows {
		return out, false, errors.New("В листе слишком много строк (максимум 100000)")
	}
	headerRow := -1
	for r := 0; r < count && r < 10; r++ {
		match := true
		for c := 0; c < 21; c++ {
			if normalized(cell(r, c)) != normalized(Headers[c]) {
				match = false
				break
			}
		}
		if match {
			headerRow = r
			break
		}
	}
	if headerRow < 0 {
		return out, false, nil
	}
	out.Columns = 21
	hasExtra := false
	for c := 21; c < 28; c++ {
		if strings.TrimSpace(cell(headerRow, c)) != "" {
			hasExtra = true
		}
	}
	if hasExtra {
		for c := 21; c < 28; c++ {
			if normalized(cell(headerRow, c)) != normalized(Headers[c]) {
				return out, true, fmt.Errorf("Заголовок колонки %d не соответствует шаблону", c+1)
			}
		}
		out.Columns = 28
	}
	out.Rows = []Row{}
	for r := headerRow + 1; r < count; r++ {
		row := Row{Number: r + 1}
		meaningful := false
		for c := 0; c < 26; c++ {
			if c >= out.Columns {
				break
			}
			v := strings.TrimSpace(cell(r, c))
			if len(v) > 20000 {
				return out, true, fmt.Errorf("Слишком длинное поле в строке %d", r+1)
			}
			row.Values[c] = v
			if v != "" && !(c == 1 && strings.EqualFold(v, "default")) && !(c == 10 && v == `МКУ "УИТС г. Сургута"`) {
				meaningful = true
			}
		}
		// The supplied template fills 'default' and the default location down to row 64844. Those are not objects.
		if !meaningful {
			out.IgnoredRows++
			continue
		}
		if out.Columns == 28 {
			row.HasSecrets = strings.TrimSpace(cell(r, 26)) != "" || strings.TrimSpace(cell(r, 27)) != ""
			if row.HasSecrets {
				out.SecretRows++
			}
			for c := 21; c < 26; c++ {
				if row.Values[c] != "" {
					out.BackupRows++
					break
				}
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out, true, nil
}

// Read keeps archive password values out of the returned model entirely.
func Read(data []byte) (out Workbook, err error) {
	defer func() {
		if recover() != nil {
			out = Workbook{}
			err = errors.New("Повреждённый или неподдерживаемый файл Excel")
		}
	}()
	if len(data) == 0 || len(data) > MaxFileSize {
		return out, errors.New("Размер Excel должен быть от 1 байта до 20 МБ")
	}
	if bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0}) {
		wb, e := xls.OpenReader(bytes.NewReader(data))
		if e != nil {
			return out, errors.New("Не удалось прочитать XLS")
		}
		if wb.GetNumberSheets() > 100 {
			return out, errors.New("В книге слишком много листов")
		}
		for i := 0; i < wb.GetNumberSheets(); i++ {
			sheet, e := wb.GetSheet(i)
			if e != nil {
				continue
			}
			result, found, e := parseSheet(sheet.GetName(), sheet.GetNumberRows(), func(r, c int) string {
				row, _ := sheet.GetRow(r)
				v, e := row.GetCol(c)
				if e != nil || v == nil {
					return ""
				}
				return v.GetString()
			})
			if e != nil {
				return out, e
			}
			if found {
				return result, nil
			}
		}
	} else if bytes.HasPrefix(data, []byte{'P', 'K'}) {
		wb, e := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 128 * 1024 * 1024, UnzipXMLSizeLimit: 32 * 1024 * 1024})
		if e != nil {
			return out, errors.New("Не удалось прочитать XLSX")
		}
		defer wb.Close()
		sheets := wb.GetSheetList()
		if len(sheets) > 100 {
			return out, errors.New("В книге слишком много листов")
		}
		for _, name := range sheets {
			rows, e := wb.GetRows(name)
			if e != nil {
				return out, errors.New("Не удалось прочитать лист XLSX")
			}
			result, found, e := parseSheet(name, len(rows), func(r, c int) string {
				if r >= len(rows) || c >= len(rows[r]) {
					return ""
				}
				return rows[r][c]
			})
			if e != nil {
				return out, e
			}
			if found {
				return result, nil
			}
		}
	} else {
		return out, errors.New("Поддерживаются файлы XLS и XLSX")
	}
	return out, errors.New("Не найден лист с заголовками утверждённого шаблона (21 или 28 колонок)")
}
func Write(rows [][21]string) ([]byte, error) { return WriteWithSummary(rows, nil) }
func WriteWithSummary(rows [][21]string, summary [][2]string) ([]byte, error) {
	wb := excelize.NewFile()
	defer wb.Close()
	sheet := "Каталог"
	if err := wb.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	write := func(r int, values []string) error {
		for c, v := range values {
			addr, _ := excelize.CoordinatesToCellName(c+1, r)
			if err := wb.SetCellStr(sheet, addr, v); err != nil {
				return err
			}
		}
		return nil
	}
	if err := write(1, Headers[:21]); err != nil {
		return nil, err
	}
	for r, values := range rows {
		if err := write(r+2, values[:]); err != nil {
			return nil, err
		}
	}
	style, err := wb.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"18566A"}, Pattern: 1}, Alignment: &excelize.Alignment{WrapText: true, Vertical: "center"}})
	if err != nil {
		return nil, err
	}
	if err = wb.SetCellStyle(sheet, "A1", "U1", style); err != nil {
		return nil, err
	}
	if err = wb.SetRowHeight(sheet, 1, 48); err != nil {
		return nil, err
	}
	if err = wb.SetColWidth(sheet, "A", "U", 23); err != nil {
		return nil, err
	}
	if err = wb.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		if err = wb.AutoFilter(sheet, fmt.Sprintf("A1:U%d", len(rows)+1), nil); err != nil {
			return nil, err
		}
	}
	if len(summary) > 0 {
		if _, err = wb.NewSheet("Статистика"); err != nil {
			return nil, err
		}
		for r, values := range summary {
			for c, v := range values {
				addr, _ := excelize.CoordinatesToCellName(c+1, r+1)
				if err = wb.SetCellStr("Статистика", addr, v); err != nil {
					return nil, err
				}
			}
		}
		if err = wb.SetColWidth("Статистика", "A", "A", 48); err != nil {
			return nil, err
		}
		if err = wb.SetColWidth("Статистика", "B", "B", 32); err != nil {
			return nil, err
		}
		if err = wb.SetCellStyle("Статистика", "A1", "B1", style); err != nil {
			return nil, err
		}
	}
	buffer, err := wb.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
