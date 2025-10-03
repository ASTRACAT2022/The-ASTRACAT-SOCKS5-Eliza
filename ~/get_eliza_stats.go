package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"
)

const (
	statsFilePath = "/var/lib/astra_socks_eliza/stats.json" // Путь к файлу статистики
)

// GlobalStats (структура должна быть идентична той, что в main.go)
type GlobalStats struct {
	TotalUploadBytes   int64                 `json:"totalUploadBytes"`
	TotalDownloadBytes int64                 `json:"totalDownloadBytes"`
	ActiveConnections  int32                 `json:"activeConnections"`
	UserStats          map[string]UserTraffic `json:"userStats"`
	LastUpdateTime     time.Time             `json:"lastUpdateTime"`
}

// UserTraffic (структура должна быть идентична той, что в main.go)
type UserTraffic struct {
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
}

func main() {
	stats, err := loadStatsFromFile()
	if err != nil {
		fmt.Printf("Ошибка при чтении статистики: %v\n", err)
		fmt.Println("Убедитесь, что сервис 'astra-socks-eliza' запущен и генерирует файл статистики.")
		os.Exit(1)
	}

	fmt.Printf("--- Статистика The-ASTRACAT-SOCKS-Eliza ---\n")
	fmt.Printf("Последнее обновление: %s\n", stats.LastUpdateTime.Format("2006-01-02 15:04:05 MSK")) // Россия - время MSK
	fmt.Printf("Активные соединения: %d\n", stats.ActiveConnections)
	fmt.Printf("Общий трафик: Загружено: %s, Скачано: %s\n",
		formatBytes(stats.TotalUploadBytes),
		formatBytes(stats.TotalDownloadBytes),
	)
	fmt.Println("---------------------------------------")
	fmt.Println("Статистика по пользователям:")
	if len(stats.UserStats) == 0 {
		fmt.Println("  Нет данных по пользователям.")
	} else {
		for username, userStats := range stats.UserStats {
			fmt.Printf("  '%s': Загружено: %s, Скачано: %s\n",
				username,
				formatBytes(userStats.UploadBytes),
				formatBytes(userStats.DownloadBytes),
			)
		}
	}
	fmt.Println("---------------------------------------")
}

// loadStatsFromFile загружает статистику из JSON-файла
func loadStatsFromFile() (*GlobalStats, error) {
	data, err := ioutil.ReadFile(statsFilePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла статистики %s: %w", statsFilePath, err)
	}

	var stats GlobalStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("ошибка декодирования JSON из файла статистики %s: %w", statsFilePath, err)
	}
	return &stats, nil
}

// formatBytes форматирует байты в более читаемый вид (KB, MB, GB)
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
