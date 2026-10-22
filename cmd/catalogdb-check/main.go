// catalogdb-check validates a connection and applies the same automatic migrations
// as the desktop client. Password input is read from stdin, never command arguments.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yomezza/test/internal/config"
	"github.com/yomezza/test/internal/database"
)

func main() {
	path := flag.String("config", "", "Путь к connection.json (без пароля)")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "Укажите --config; пароль PostgreSQL передаётся одной строкой через stdin")
		os.Exit(2)
	}
	c, err := config.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Некорректная конфигурация")
		os.Exit(1)
	}
	password, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && password == "" {
		fmt.Fprintln(os.Stderr, "Не получен пароль через stdin")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, c, strings.TrimSuffix(strings.TrimSuffix(password, "\n"), "\r"))
	if err == nil {
		defer pool.Close()
		err = database.Migrate(ctx, pool, c.Schema)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, database.PublicError(err))
		os.Exit(1)
	}
	fmt.Printf("Соединение проверено, схема %s обновлена.\n", c.Schema)
}
