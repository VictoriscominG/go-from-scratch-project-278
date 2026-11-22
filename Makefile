.PHONY: run dev build clean tidy

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
