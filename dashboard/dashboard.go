package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	defaultStatsPath = "/var/lib/astra_socks_eliza/stats.json"
	defaultPort      = "8080"
)

func main() {
	// Определение флагов командной строки
	port := flag.String("port", defaultPort, "Порт для запуска веб-сервера")
	statsPath := flag.String("stats-file", defaultStatsPath, "Путь к файлу статистики JSON")
	flag.Parse()

	// Настройка обработчиков
	http.HandleFunc("/api/stats", statsHandler(*statsPath))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("dashboard", "templates", "index.html"))
	})

	// Обработчик для статических файлов (CSS, JS)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join("dashboard", "static")))))


	log.Printf("Запуск сервера панели мониторинга на порту: %s", *port)
	log.Printf("Чтение статистики из файла: %s", *statsPath)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}

// statsHandler создает замыкание для обработки запросов к /api/stats
func statsHandler(statsPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Установка заголовка для CORS, чтобы разрешить запросы с любого источника
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")

		data, err := os.ReadFile(statsPath)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, `{"error": "Файл статистики не найден"}`, http.StatusNotFound)
				return
			}
			http.Error(w, `{"error": "Ошибка чтения файла статистики"}`, http.StatusInternalServerError)
			log.Printf("Ошибка чтения файла %s: %v", statsPath, err)
			return
		}

		// Проверка, что JSON валиден перед отправкой
		var js json.RawMessage
		if err := json.Unmarshal(data, &js); err != nil {
			http.Error(w, `{"error": "Ошибка парсинга JSON в файле статистики"}`, http.StatusInternalServerError)
			log.Printf("Ошибка парсинга JSON из %s: %v", statsPath, err)
			return
		}

		w.Write(data)
	}
}