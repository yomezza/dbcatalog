//go:build !windows && (!linux || (!desktop && !bindings))

package main

import "fmt"

func main() {
	fmt.Println("CatalogDB: соберите настольное приложение командой wails build (Linux: -tags webkit2_41). Без desktop-тега доступны backend-тесты и cmd/catalogdb-check.")
}
