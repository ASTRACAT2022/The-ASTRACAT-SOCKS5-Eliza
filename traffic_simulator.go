package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"
)

// Stats представляет структуру данных статистики
type Stats struct {
	Connections []Connection `json:"connections"`
}

// Connection представляет одно соединение
type Connection struct {
	User     string `json:"user"`
	SrcIP    string `json:"src_ip"`
	DstIP    string `json:"dst_ip"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	API      string `json:"api"`
}

// Глобальные переменные для генерации случайных данных
var (
	// Список возможных пользователей
	usersList = []string{"user1", "user2", "user3", "admin", "guest", "test"}

	// Список возможных API
	apisList = []string{"facebook", "google", "twitter", "instagram", "tiktok", "youtube", "netflix", "amazon", "github", "stackoverflow"}

	// Список возможных IP-адресов назначения
	dstIPs = []string{
		"8.8.8.8", "1.1.1.1", "208.67.222.222", "208.67.220.220",
		"216.58.214.174", "31.13.72.36", "104.244.42.193", "151.101.1.140",
		"13.107.42.16", "140.82.121.4", "185.199.108.153", "192.30.255.112",
		"172.217.20.174", "52.216.239.53", "54.231.0.37", "99.84.239.80",
		"23.23.212.126", "34.102.136.180", "35.190.247.0", "35.186.224.25",
		"142.250.185.78", "142.250.185.174", "142.250.185.110", "142.250.185.46",
	}

	// Список возможных локальных IP-адресов
	srcIPs = []string{
		"192.168.1.10", "192.168.1.11", "192.168.1.12", "192.168.1.13",
		"192.168.1.14", "192.168.1.15", "192.168.1.16", "192.168.1.17",
	}
)

func main() {
	// Параметры командной строки
	statsFile := flag.String("stats-file", "./stats.json", "Путь к файлу статистики")
	interval := flag.Int("interval", 5, "Интервал обновления в секундах")
	maxConnections := flag.Int("max-connections", 20, "Максимальное количество соединений")
	flag.Parse()

	// Инициализация генератора случайных чисел
	rand.Seed(time.Now().UnixNano())

	fmt.Printf("Запуск симулятора трафика\n")
	fmt.Printf("Файл статистики: %s\n", *statsFile)
	fmt.Printf("Интервал обновления: %d секунд\n", *interval)
	fmt.Printf("Максимальное количество соединений: %d\n", *maxConnections)

	// Основной цикл симуляции
	for {
		// Генерация случайного количества соединений
		numConnections := rand.Intn(*maxConnections) + 5 // Минимум 5 соединений
		connections := generateConnections(numConnections)

		// Создание статистики
		stats := Stats{
			Connections: connections,
		}

		// Запись в файл
		err := writeStatsToFile(stats, *statsFile)
		if err != nil {
			log.Fatalf("Ошибка записи статистики: %v", err)
		}

		fmt.Printf("Сгенерировано %d соединений, статистика записана в %s\n", numConnections, *statsFile)

		// Пауза перед следующим обновлением
		time.Sleep(time.Duration(*interval) * time.Second)
	}
}

// Генерация случайных соединений
func generateConnections(count int) []Connection {
	connections := make([]Connection, count)

	for i := 0; i < count; i++ {
		// Генерация случайных значений трафика (от 1 КБ до 100 МБ)
		uploadBytes := rand.Int63n(100*1024*1024) + 1024
		downloadBytes := rand.Int63n(100*1024*1024) + 1024

		connections[i] = Connection{
			User:     usersList[rand.Intn(len(usersList))],
			SrcIP:    srcIPs[rand.Intn(len(srcIPs))],
			DstIP:    dstIPs[rand.Intn(len(dstIPs))],
			Upload:   uploadBytes,
			Download: downloadBytes,
			API:      apisList[rand.Intn(len(apisList))],
		}
	}

	return connections
}

// Запись статистики в файл
func writeStatsToFile(stats Stats, filename string) error {
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("ошибка записи в файл: %w", err)
	}

	return nil
}