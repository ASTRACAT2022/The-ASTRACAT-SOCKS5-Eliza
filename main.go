package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

const (
	socks5Version        = 0x05
	noAuthRequired       = 0x00
	usernamePasswordAuth = 0x02
	connectCommand       = 0x01
	ipv4Address          = 0x01
	domainNameAddress    = 0x03
	ipv6Address          = 0x04
	replySuccess         = 0x00

	statsFilePath    = "/var/lib/astra_socks_eliza/stats.json" // Путь к файлу статистики
	usersFilePath    = "/etc/astra_socks_eliza/users.json"     // Путь к файлу пользователей
	geoIPDBPath      = "/usr/share/GeoIP/GeoLite2-Country.mmdb" // Путь к GeoIP базе данных
)

// --- Структуры данных для пользователей и статистики (в памяти) ---

// User представляет пользователя SOCKS5
type User struct {
	Username string `json:"username"`
	Password string `json:"password"` // В реальной жизни хешировать!
	Enabled  bool   `json:"enabled"`
}

// UserTraffic представляет статистику трафика для пользователя
type UserTraffic struct {
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
}

// CountryStats представляет статистику по стране
type CountryStats struct {
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
	Connections   int64 `json:"connections"`
}

// GlobalStats представляет общую статистику
type GlobalStats struct {
	TotalUploadBytes   int64                    `json:"totalUploadBytes"`
	TotalDownloadBytes int64                    `json:"totalDownloadBytes"`
	ActiveConnections  int32                    `json:"activeConnections"`
	UserStats          map[string]UserTraffic   `json:"userStats"`          // Статистика по каждому пользователю
	CountryStats       map[string]*CountryStats `json:"countryStats"`       // Статистика по странам (ключ - код страны)
	LastUpdateTime     time.Time                `json:"lastUpdateTime"`
}

// Глобальные хранилища в памяти
var (
	users      = make(map[string]User)      // key: username, value: User
	usersMutex sync.RWMutex                 // Мьютекс для доступа к users

	trafficStats = make(map[string]UserTraffic) // key: username, value: UserTraffic
	countryStats = make(map[string]*CountryStats) // key: country code, value: stats
	trafficMutex sync.RWMutex                   // Мьютекс для доступа к trafficStats и countryStats

	activeConnectionsCounter int32
	activeConnectionsMutex   sync.Mutex

	geoDB *geoip2.Reader // Указатель на ридер GeoIP базы
)

// init вызывается один раз при запуске программы
func init() {
	// Попытка загрузить пользователей из файла
	if err := loadUsersFromFile(); err != nil {
		log.Printf("Внимание: Не удалось загрузить пользователей из файла %s: %v. Добавляем тестового пользователя.", usersFilePath, err)
		// Добавляем тестового пользователя по умолчанию, если файл не найден или пуст
		users["astranet"] = User{Username: "astranet", Password: "astranet", Enabled: true}
	} else {
		log.Printf("Пользователи загружены из %s.", usersFilePath)
	}

	// Попытка загрузить GeoIP базу данных
	var err error
	geoDB, err = geoip2.Open(geoIPDBPath)
	if err != nil {
		log.Printf("Внимание: Не удалось загрузить GeoIP базу данных из %s: %v. Сбор геолокационной статистики будет отключен.", geoIPDBPath, err)
	} else {
		log.Printf("GeoIP база данных успешно загружена из %s.", geoIPDBPath)
	}

	log.Println("Порт SOCKS5: 7777")
	log.Println("Статистика сохраняется в файл: " + statsFilePath)
}

// --- SOCKS5 Прокси-сервер ---

func main() {
	// Создаем необходимые директории для файлов статистики и пользователей, если их нет
	err := os.MkdirAll(filepath.Dir(statsFilePath), 0755)
	if err != nil {
		log.Fatalf("Критическая ошибка: Не удалось создать директорию для файла статистики (%s): %v", filepath.Dir(statsFilePath), err)
	}
	err = os.MkdirAll(filepath.Dir(usersFilePath), 0755)
	if err != nil {
		log.Fatalf("Критическая ошибка: Не удалось создать директорию для файла пользователей (%s): %v", filepath.Dir(usersFilePath), err)
	}

	go startSocks5Server()
	go saveStatsPeriodically(5 * time.Second) // Сохраняем статистику каждые 5 секунд

	// Основная горутина просто ждет, чтобы программа не завершилась
	select {}
}

func startSocks5Server() {
	listener, err := net.Listen("tcp", "0.0.0.0:7777") // SOCKS5 прокси остаётся на порту 7777
	if err != nil {
		log.Fatalf("Ошибка при запуске SOCKS5 сервера The-ASTRACAT-SOCKS-Eliza: %v", err)
	}
	defer listener.Close()
	log.Println("SOCKS5 сервер The-ASTRACAT-SOCKS-Eliza запущен на 0.0.0.0:7777 с аутентификацией логин/пароль.")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка при приёме соединения: %v", err)
			continue
		}
		activeConnectionsMutex.Lock()
		activeConnectionsCounter++
		activeConnectionsMutex.Unlock()

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		activeConnectionsMutex.Lock()
		activeConnectionsCounter--
		activeConnectionsMutex.Unlock()
	}()

	clientIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		log.Printf("Не удалось получить IP клиента %s: %v", conn.RemoteAddr(), err)
		clientIP = "unknown" // На всякий случай
	}

	// Обновляем счетчик подключений для страны
	countryCode := getCountryCode(clientIP)
	if countryCode != "XX" {
		trafficMutex.Lock()
		if stats, ok := countryStats[countryCode]; ok {
			stats.Connections++
		} else {
			countryStats[countryCode] = &CountryStats{Connections: 1}
		}
		trafficMutex.Unlock()
	}

	username, err := socks5Handshake(conn)
	if err != nil {
		log.Printf("Ошибка SOCKS5 рукопожатия для %s: %v", conn.RemoteAddr(), err)
		return
	}

	if err := handleSocks5Request(conn, username, clientIP); err != nil {
		log.Printf("Ошибка SOCKS5 запроса для %s (пользователь %s): %v", conn.RemoteAddr(), username, err)
		return
	}
}

func socks5Handshake(conn net.Conn) (string, error) {
	buf := make([]byte, 2)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения приветствия SOCKS5: %w", err)
	}

	if buf[0] != socks5Version {
		return "", fmt.Errorf("неподдерживаемая версия SOCKS: %d", buf[0])
	}

	numMethods := int(buf[1])
	methods := make([]byte, numMethods)
	_, err = io.ReadFull(conn, methods)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения методов аутентификации: %w", err)
	}

	foundUserPassAuth := false
	for _, method := range methods {
		if method == usernamePasswordAuth {
			foundUserPassAuth = true
			break
		}
	}

	if !foundUserPassAuth {
		_, _ = conn.Write([]byte{socks5Version, 0xFF})
		return "", fmt.Errorf("нет поддерживаемых методов аутентификации (требуется 0x02)")
	}

	_, err = conn.Write([]byte{socks5Version, usernamePasswordAuth})
	if err != nil {
		return "", fmt.Errorf("ошибка отправки подтверждения метода аутентификации: %w", err)
	}

	username, err := authenticateUserPass(conn)
	if err != nil {
		return "", err
	}
	return username, nil
}

func authenticateUserPass(conn net.Conn) (string, error) {
	buf := make([]byte, 2)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения заголовка аутентификации: %w", err)
	}

	if buf[0] != 0x01 {
		return "", fmt.Errorf("неподдерживаемая версия протокола аутентификации: %d", buf[0])
	}

	usernameLen := int(buf[1])
	usernameBuf := make([]byte, usernameLen)
	_, err = io.ReadFull(conn, usernameBuf)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения имени пользователя: %w", err)
	}
	username := string(usernameBuf)

	_, err = io.ReadFull(conn, buf[0:1])
	if err != nil {
		return "", fmt.Errorf("ошибка чтения длины пароля: %w", err)
	}
	passwordLen := int(buf[0])
	password := make([]byte, passwordLen)
	_, err = io.ReadFull(conn, password)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения пароля: %w", err)
	}

	usersMutex.RLock()
	user, ok := users[username]
	usersMutex.RUnlock()

	if !ok || !user.Enabled || user.Password != string(password) {
		log.Printf("Аутентификация не удалась для пользователя: %s (с %s)", username, conn.RemoteAddr())
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", fmt.Errorf("неверные имя пользователя или пароль, или пользователь неактивен")
	}

	log.Printf("Аутентификация успешна для пользователя: %s (с %s)", username, conn.RemoteAddr())
	_, err = conn.Write([]byte{0x01, replySuccess})
	if err != nil {
		return "", fmt.Errorf("ошибка отправки ответа об успешной аутентификации: %w", err)
	}
	return username, nil
}

func handleSocks5Request(conn net.Conn, username string, clientIP string) error {
	buf := make([]byte, 4)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return fmt.Errorf("ошибка чтения заголовка запроса SOCKS5: %w", err)
	}

	if buf[0] != socks5Version {
		return fmt.Errorf("неподдерживаемая версия SOCKS в запросе: %d", buf[0])
	}

	if buf[1] != connectCommand {
		_, _ = conn.Write([]byte{socks5Version, 0x07, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return fmt.Errorf("неподдерживаемая команда: %d", buf[1])
	}

	var destAddr string
	var destPort int
	switch buf[3] { // ATYP
	case ipv4Address:
		ipv4 := make([]byte, 4)
		_, err = io.ReadFull(conn, ipv4)
		if err != nil {
			return fmt.Errorf("ошибка чтения IPv4 адреса: %w", err)
		}
		destAddr = net.IPv4(ipv4[0], ipv4[1], ipv4[2], ipv4[3]).String()
	case domainNameAddress:
		lenBuf := make([]byte, 1)
		_, err = io.ReadFull(conn, lenBuf)
		if err != nil {
			return fmt.Errorf("ошибка чтения длины доменного имени: %w", err)
		}
		domainLen := int(lenBuf[0])
		domain := make([]byte, domainLen)
		_, err = io.ReadFull(conn, domain)
		if err != nil {
			return fmt.Errorf("ошибка чтения доменного имени: %w", err)
		}
		destAddr = string(domain)
	case ipv6Address:
		ipv6 := make([]byte, 16)
		_, err = io.ReadFull(conn, ipv6)
		if err != nil {
			return fmt.Errorf("ошибка чтения IPv6 адреса: %w", err)
		}
		destAddr = net.IP(ipv6).String()
	default:
		_, _ = conn.Write([]byte{socks5Version, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return fmt.Errorf("неподдерживаемый тип адреса: %d", buf[3])
	}

	portBuf := make([]byte, 2)
	_, err = io.ReadFull(conn, portBuf)
	if err != nil {
		return fmt.Errorf("ошибка чтения порта: %w", err)
	}
	destPort = int(portBuf[0])<<8 | int(portBuf[1])

	target := net.JoinHostPort(destAddr, strconv.Itoa(destPort))

	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("Ошибка Dial к %s (запрошено %s от %s): %v", target, username, conn.RemoteAddr(), err)
		_, _ = conn.Write([]byte{socks5Version, 0x05, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return fmt.Errorf("не удалось подключиться к целевому хосту: %w", err)
	}
	defer targetConn.Close()

	_, err = conn.Write([]byte{socks5Version, replySuccess, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		return fmt.Errorf("ошибка отправки ответа об успехе: %w", err)
	}

	countryCode := getCountryCode(clientIP)
	return proxyData(conn, targetConn, username, countryCode)
}

// customWriter обертывает net.Conn и считает переданные байты
type customWriter struct {
	io.Writer
	bytesWritten int64
}

func (cw *customWriter) Write(p []byte) (n int, err error) {
	n, err = cw.Writer.Write(p)
	cw.bytesWritten += int64(n)
	return
}

// proxyData теперь собирает статистику в память для пользователей и стран
func proxyData(clientConn, targetConn net.Conn, username, countryCode string) error {
	done := make(chan error, 2)

	clientWriter := &customWriter{Writer: clientConn}
	targetWriter := &customWriter{Writer: targetConn}

	go func() {
		_, err := io.Copy(targetWriter, clientConn) // clientConn (Reader) -> targetWriter (Writer)
		done <- err
	}()

	go func() {
		_, err := io.Copy(clientWriter, targetConn) // targetConn (Reader) -> clientWriter (Writer)
		done <- err
	}()

	err1 := <-done
	err2 := <-done

	// Обновляем статистику в памяти
	trafficMutex.Lock()
	defer trafficMutex.Unlock()

	// Обновляем статистику пользователя
	userStats := trafficStats[username]
	userStats.UploadBytes += targetWriter.bytesWritten
	userStats.DownloadBytes += clientWriter.bytesWritten
	trafficStats[username] = userStats

	// Обновляем статистику страны
	if countryCode != "XX" {
		cStats, ok := countryStats[countryCode]
		if !ok {
			cStats = &CountryStats{}
			countryStats[countryCode] = cStats
		}
		cStats.UploadBytes += targetWriter.bytesWritten
		cStats.DownloadBytes += clientWriter.bytesWritten
		// Note: Connections are counted once per handleConnection, not here
	}

	if err1 != nil && err1 != io.EOF {
		return fmt.Errorf("ошибка копирования клиент -> цель: %w", err1)
	}
	if err2 != nil && err2 != io.EOF {
		return fmt.Errorf("ошибка копирования цель -> клиент: %w", err2)
	}
	return nil
}

// saveStatsPeriodically собирает общую статистику и сохраняет её в файл
func saveStatsPeriodically(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		trafficMutex.RLock()
		activeConnectionsMutex.Lock()

		var totalUpload int64
		var totalDownload int64

		currentUserStats := make(map[string]UserTraffic)
		for username, stats := range trafficStats {
			totalUpload += stats.UploadBytes
			totalDownload += stats.DownloadBytes
			currentUserStats[username] = stats
		}

		// Копируем статистику по странам
		currentCountryStats := make(map[string]*CountryStats)
		for code, stats := range countryStats {
			// Создаем копию, чтобы избежать гонки данных при параллельной записи
			sCopy := *stats
			currentCountryStats[code] = &sCopy
		}

		currentActiveConnections := activeConnectionsCounter

		activeConnectionsMutex.Unlock()
		trafficMutex.RUnlock()

		globalStats := GlobalStats{
			TotalUploadBytes:   totalUpload,
			TotalDownloadBytes: totalDownload,
			ActiveConnections:  currentActiveConnections,
			UserStats:          currentUserStats,
			CountryStats:       currentCountryStats, // Добавляем статистику по странам
			LastUpdateTime:     time.Now(),
		}

		// Сохраняем статистику в файл
		file, err := os.OpenFile(statsFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			log.Printf("Ошибка при открытии/создании файла статистики %s: %v", statsFilePath, err)
			continue
		}
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ") // Для красивого форматирования JSON
		if err := encoder.Encode(globalStats); err != nil {
			log.Printf("Ошибка при записи статистики в файл %s: %v", statsFilePath, err)
		}
		file.Close()
	}
}

// getCountryCode определяет код страны по IP-адресу.
// Возвращает "XX" в случае ошибки или если база данных недоступна.
func getCountryCode(ipStr string) string {
	if geoDB == nil {
		return "XX" // База данных не загружена
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "XX" // Невалидный IP
	}

	record, err := geoDB.Country(ip)
	if err != nil {
		// log.Printf("Не удалось определить страну для IP %s: %v", ipStr, err)
		return "XX"
	}

	return record.Country.IsoCode
}


// loadUsersFromFile загружает пользователей из JSON-файла
func loadUsersFromFile() error {
	file, err := os.Open(usersFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Если файл не существует, создаем его с тестовыми данными
			log.Printf("Инфо: Файл пользователей %s не найден. Создаю новый файл с пользователем 'astranet:astranet'.", usersFilePath)
			defaultUsers := map[string]User{
				"astranet": {Username: "astranet", Password: "astranet", Enabled: true},
			}
			data, err := json.MarshalIndent(defaultUsers, "", "  ")
			if err != nil {
				return fmt.Errorf("ошибка кодирования JSON для пользователей по умолчанию: %w", err)
			}
			// Убедимся, что директория существует перед записью файла
			if err := os.MkdirAll(filepath.Dir(usersFilePath), 0755); err != nil {
				return fmt.Errorf("не удалось создать директорию для файла пользователей %s: %w", filepath.Dir(usersFilePath), err)
			}
			if err := os.WriteFile(usersFilePath, data, 0644); err != nil {
				return fmt.Errorf("ошибка записи файла пользователей по умолчанию: %w", err)
			}
			usersMutex.Lock()
			users = defaultUsers
			usersMutex.Unlock()
			return nil
		}
		return fmt.Errorf("ошибка открытия файла пользователей %s: %w", usersFilePath, err)
	}
	defer file.Close()

	var tempUsers map[string]User
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&tempUsers); err != nil {
		return fmt.Errorf("ошибка декодирования JSON из файла пользователей %s: %w", usersFilePath, err)
	}

	usersMutex.Lock()
	users = tempUsers
	usersMutex.Unlock()
	return nil
}
