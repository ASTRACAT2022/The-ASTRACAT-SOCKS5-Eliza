package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultStatsPath = "./stats.json"
	defaultPort      = "8080"
)

// IPGeoResponse представляет данные геолокации IP-адреса
type IPGeoResponse struct {
	IP          string  `json:"ip"`
	CountryCode string  `json:"country_code2"`
	CountryName string  `json:"country_name"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// IPGeoCache кэш для хранения данных геолокации
type IPGeoCache struct {
	cache map[string]IPGeoResponse
	mu    sync.RWMutex
}

// NewIPGeoCache создает новый кэш для геолокации
func NewIPGeoCache() *IPGeoCache {
	return &IPGeoCache{
		cache: make(map[string]IPGeoResponse),
	}
}

// Get получает данные из кэша
func (c *IPGeoCache) Get(ip string) (IPGeoResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, ok := c.cache[ip]
	return data, ok
}

// Set сохраняет данные в кэш
func (c *IPGeoCache) Set(ip string, data IPGeoResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[ip] = data
}

// Глобальный кэш для геолокации
var geoCache = NewIPGeoCache()

// Локальная база данных для геолокации
var geoDatabase = map[string]IPGeoResponse{
	"8.8.8.8": {
		IP:          "8.8.8.8",
		CountryCode: "US",
		CountryName: "United States",
		City:        "Mountain View",
		Latitude:    37.386,
		Longitude:   -122.0838,
	},
	"1.1.1.1": {
		IP:          "1.1.1.1",
		CountryCode: "AU",
		CountryName: "Australia",
		City:        "Sydney",
		Latitude:    -33.8688,
		Longitude:   151.2093,
	},
	"77.88.55.88": {
		IP:          "77.88.55.88",
		CountryCode: "RU",
		CountryName: "Russia",
		City:        "Moscow",
		Latitude:    55.7558,
		Longitude:   37.6173,
	},
	"95.217.163.246": {
		IP:          "95.217.163.246",
		CountryCode: "FI",
		CountryName: "Finland",
		City:        "Helsinki",
		Latitude:    60.1699,
		Longitude:   24.9384,
	},
	"185.143.223.100": {
		IP:          "185.143.223.100",
		CountryCode: "NL",
		CountryName: "Netherlands",
		City:        "Amsterdam",
		Latitude:    52.3676,
		Longitude:   4.9041,
	},
}

func main() {
	// Определение флагов командной строки
	port := flag.String("port", defaultPort, "Порт для запуска веб-сервера")
	statsPath := flag.String("stats-file", defaultStatsPath, "Путь к файлу статистики JSON")
	flag.Parse()

	// Настройка обработчиков
	http.HandleFunc("/api/stats", statsHandler(*statsPath))
	http.HandleFunc("/api/ipgeo", ipGeoHandler)
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

// getIPGeolocation получает информацию о геолокации IP-адреса из локальной базы данных
func getIPGeolocation(ip string) (IPGeoResponse, error) {
	// Проверяем кэш
	if data, ok := geoCache.Get(ip); ok {
		return data, nil
	}

	// Проверяем локальную базу данных
	if data, ok := geoDatabase[ip]; ok {
		geoCache.Set(ip, data)
		return data, nil
	}

	// Если IP не найден в базе, генерируем случайные координаты
	// Это упрощенный подход для демонстрации
	octets := strings.Split(ip, ".")
	var countryCode, countryName, city string
	
	// Определяем страну по первому октету (очень упрощенно)
	firstOctet := 0
	if len(octets) > 0 {
		fmt.Sscanf(octets[0], "%d", &firstOctet)
	}
	
	switch {
	case firstOctet < 50:
		countryCode = "US"
		countryName = "United States"
		city = "New York"
	case firstOctet < 100:
		countryCode = "EU"
		countryName = "Europe"
		city = "Berlin"
	case firstOctet < 150:
		countryCode = "AS"
		countryName = "Asia"
		city = "Tokyo"
	case firstOctet < 200:
		countryCode = "AF"
		countryName = "Africa"
		city = "Cairo"
	default:
		countryCode = "OC"
		countryName = "Oceania"
		city = "Sydney"
	}
	
	// Генерируем случайные координаты
	lat := (rand.Float64() * 180) - 90
	lon := (rand.Float64() * 360) - 180
	
	data := IPGeoResponse{
		IP:          ip,
		CountryCode: countryCode,
		CountryName: countryName,
		City:        city,
		Latitude:    lat,
		Longitude:   lon,
	}
	
	// Сохраняем в кэш
	geoCache.Set(ip, data)
	return data, nil
}

// ipGeoHandler обрабатывает запросы к API геолокации
func ipGeoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	ip := r.URL.Query().Get("ip")
	if ip == "" {
		http.Error(w, `{"error": "IP-адрес не указан"}`, http.StatusBadRequest)
		return
	}

	geoData, err := getIPGeolocation(ip)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Ошибка получения геолокации: %v"}`, err), http.StatusInternalServerError)
		log.Printf("Ошибка получения геолокации для IP %s: %v", ip, err)
		return
	}

	jsonData, err := json.Marshal(geoData)
	if err != nil {
		http.Error(w, `{"error": "Ошибка сериализации данных"}`, http.StatusInternalServerError)
		return
	}

	w.Write(jsonData)
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