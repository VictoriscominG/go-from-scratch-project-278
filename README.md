# Сокращатель ссылок (Go)

[![hexlet-check](https://github.com/VictoriscominG/go-from-scratch-project-278/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/VictoriscominG/go-from-scratch-project-278/actions) [![CI](https://github.com/VictoriscominG/go-from-scratch-project-278/actions/workflows/ci.yml/badge.svg)](https://github.com/VictoriscominG/go-from-scratch-project-278/actions/workflows/ci.yml) [![Coverage](https://sonarcloud.io/api/project_badges/measure?project=VictoriscominG_go-from-scratch-project-278&metric=coverage)](https://sonarcloud.io/summary/new_code?id=VictoriscominG_go-from-scratch-project-278)


## 🚀 Запущенное приложение

Приложение развернуто на Render:  
👉 [go-from-scratch-project-278-0b05.onrender.com](https://go-from-scratch-project-278-0b05.onrender.com)

## Описание

Сервис сокращения ссылок: превращает длинный URL в короткое имя и перенаправляет по нему, собирая статистику переходов.


Спроектируйте приложение для удобных ссылок

Учебный проект Хекслета: https://ru.hexlet.io/programs/go-from-scratch


## Стек

- Go + Gin — HTTP-сервер и маршрутизация
- PostgreSQL + pgx, SQL-запросы генерируются sqlc
- Миграции — goose
- Фронтенд — готовый пакет @hexlet/project-url-shortener-frontend, раздаётся Caddy
- Sentry — мониторинг ошибок
- Docker + Render — сборка и деплой
- CI — GitHub Actions: тесты с -race, golangci-lint, сборка


## API

| Метод | Путь | Назначение |
|---|---|---|
| GET | /ping | health-check, возвращает `pong` |
| POST | /api/links | создание ссылки |
| GET | /api/links | список ссылок с пагинацией |
| GET | /api/links/:id | ссылка по id |
| PUT | /api/links/:id | обновление ссылки |
| DELETE | /api/links/:id | удаление ссылки |
| GET | /r/:short_name | редирект на исходный URL |
| GET | /api/link_visits | список переходов с пагинацией |


## Использование

```bash
# Создать ссылку
curl -X POST https://go-from-scratch-project-278-0b05.onrender.com/api/links \
  -H "Content-Type: application/json" \
  -d '{"original_url": "https://example.com/long-url", "short_name": "exmpl"}'

# Список с пагинацией
curl -i "https://go-from-scratch-project-278-0b05.onrender.com/api/links?range=[0,10]"
# → Content-Range: links 0-10/42

# Редирект по короткому имени
curl -i https://go-from-scratch-project-278-0b05.onrender.com/r/exmpl

```
## Переменные окружения

| Переменная | Обязательна | Назначение |
|---|---|---|
| `DATABASE_URL` | да | строка подключения к PostgreSQL |
| `PORT` | нет | порт приложения, по умолчанию 8080 |
| `BASE_URL` | нет | базовый адрес для short_url; на Render — публичный HTTPS-адрес |
| `SENTRY_DSN` | нет | DSN Sentry; без него мониторинг отключён |
| `SENTRY_DEBUG` | нет | `on` — включает тестовый маршрут /debug/sentry |


## Установка

```bash
git clone https://github.com/VictoriscominG/go-from-scratch-project-278.git
cd go-from-scratch-project-278
go mod download

# переменные окружения (например, через .env — godotenv подхватит его)
export DATABASE_URL=postgres://postgres:password@localhost:5432/appdb?sslmode=disable

# миграции
goose -dir ./db/migrations postgres "$DATABASE_URL" up

# запуск
go run main.go

```

---

<details>
<summary>Автоматические тесты Хекслета</summary>

Тесты запускаются на каждый коммит. За запуск отвечает файл `.github/workflows/hexlet-check.yml` — не удаляйте и не переименовывайте ни его, ни репозиторий.

</details>

## О Хекслете

[Хекслет](https://ru.hexlet.io/) — школа программирования: авторские программы обучения с практикой, поддержкой наставников и реальными проектами, которые остаются в резюме. Этот репозиторий — один из таких проектов.
