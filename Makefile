.PHONY: run dev build clean tidy lint lint-fix test ci migrate

run: # Запускает приложение через go run.
	go run main.go

dev: # Запускает с Air — автоперезагрузка при изменении файлов.
	air

build: # Собирает бинарный файл link-shortener из main.go.
	go build -o link-shortener main.go

clean: # Удаляет собранный бинарный файл.
	rm -f link-shortener

tidy: # Приводит в порядок go.mod и go.sum.
	go mod tidy

lint: # Запускает линтер для проверки кода
	go tool golangci-lint run

lint-fix: # Автоматически исправляет ошибки, которые может исправить линтер
	go tool golangci-lint run --fix

test: # Запускает тест server.go
	go test ./... -race

ci: # Запускает все тесты
	lint test

migrate: # Применяет миграции к базе из переменной DATABASE_URL.
	go tool goose -dir ./db/migrations postgres "$(DATABASE_URL)" up

cover: # Тесты с покрытием (файл для SonarCloud)
	go test -race -coverprofile=coverage.out -covermode=atomic ./...