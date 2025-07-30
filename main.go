package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

// TimeSeriesDataPoint представляет одну точку данных для графика
type TimeSeriesDataPoint struct {
	Timestamp         int64 `json:"timestamp"` // Unix timestamp
	UploadBytes       int64 `json:"uploadBytes"`
	DownloadBytes     int64 `json:"downloadBytes"`
	ActiveConnections int32 `json:"activeConnections"`
}

// Глобальные хранилища в памяти
var (
	users      = make(map[string]User)      // key: username, value: User
	usersMutex sync.RWMutex                 // Мьютекс для доступа к users

	trafficStats = make(map[string]UserTraffic) // key: username, value: UserTraffic
	trafficMutex sync.RWMutex                 // Мьютекс для доступа к trafficStats

	activeConnectionsCounter int32
	activeConnectionsMutex   sync.Mutex

	timeSeriesStats []TimeSeriesDataPoint
	timeSeriesMutex sync.RWMutex
)

// Инициализация пользователей
func init() {
	users["astranet"] = User{Username: "astranet", Password: "astranet", Enabled: true}
	log.Println("Тестовый пользователь 'astranet:astranet' добавлен.")
	timeSeriesStats = make([]TimeSeriesDataPoint, 0, 100)
}

// --- SOCKS5 Прокси-сервер ---

func main() {
	go startSocks5Server()
	go aggregateStatsPeriodically(5 * time.Second)
	startWebServer()
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
	log.Printf("Новое соединение от: %s", conn.RemoteAddr())

	username, err := socks5Handshake(conn)
	if err != nil {
		log.Printf("Ошибка SOCKS5 рукопожатия для %s: %v", conn.RemoteAddr(), err)
		return
	}

	if err := handleSocks5Request(conn, username); err != nil {
		log.Printf("Ошибка SOCKS5 запроса для %s: %v", conn.RemoteAddr(), err)
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
		log.Printf("Аутентификация не удалась для пользователя: %s", username)
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", fmt.Errorf("неверные имя пользователя или пароль, или пользователь неактивен")
	}

	log.Printf("Аутентификация успешна для пользователя: %s", username)
	_, err = conn.Write([]byte{0x01, replySuccess})
	if err != nil {
		return "", fmt.Errorf("ошибка отправки ответа об успешной аутентификации: %w", err)
	}
	return username, nil
}

func handleSocks5Request(conn net.Conn, username string) error {
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
	log.Printf("Клиент %s (%s) запрашивает соединение с %s", conn.RemoteAddr(), username, target)

	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("Ошибка Dial к %s: %v", target, err)
		_, _ = conn.Write([]byte{socks5Version, 0x05, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return fmt.Errorf("не удалось подключиться к целевому хосту: %w", err)
	}
	defer targetConn.Close()

	_, err = conn.Write([]byte{socks5Version, replySuccess, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		return fmt.Errorf("ошибка отправки ответа об успехе: %w", err)
	}

	log.Printf("Установлено прокси-соединение: %s (%s) <-> %s", conn.RemoteAddr(), username, target)
	return proxyData(conn, targetConn, username)
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

// proxyData теперь собирает статистику в память
func proxyData(clientConn, targetConn net.Conn, username string) error {
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
	trafficMutex.Lock() // Используем Lock для записи
	stats := trafficStats[username]
	stats.UploadBytes += targetWriter.bytesWritten   // UPLOAD для пользователя
	stats.DownloadBytes += clientWriter.bytesWritten // DOWNLOAD для пользователя
	trafficStats[username] = stats
	trafficMutex.Unlock()

	log.Printf("Пользователь %s: загружено %d байт (UPLOAD), скачано %d байт (DOWNLOAD) за сессию.", username, targetWriter.bytesWritten, clientWriter.bytesWritten)

	if err1 != nil && err1 != io.EOF {
		return fmt.Errorf("ошибка копирования клиент -> цель: %w", err1)
	}
	if err2 != nil && err2 != io.EOF {
		return fmt.Errorf("ошибка копирования цель -> клиент: %w", err2)
	}
	return nil
}

// aggregateStatsPeriodically собирает общую статистику и добавляет ее в временной ряд
func aggregateStatsPeriodically(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		var totalUpload int64
		var totalDownload int64

		trafficMutex.RLock() // Читаем текущую статистику
		for _, stats := range trafficStats {
			totalUpload += stats.UploadBytes
			totalDownload += stats.DownloadBytes
		}
		trafficMutex.RUnlock()

		activeConnectionsMutex.Lock() // Читаем текущее количество активных соединений
		currentActiveConnections := activeConnectionsCounter
		activeConnectionsMutex.Unlock()

		dataPoint := TimeSeriesDataPoint{
			Timestamp:         time.Now().Unix(),
			UploadBytes:       totalUpload,
			DownloadBytes:     totalDownload,
			ActiveConnections: currentActiveConnections,
		}

		timeSeriesMutex.Lock()
		timeSeriesStats = append(timeSeriesStats, dataPoint)
		if len(timeSeriesStats) > 100 { // Например, храним последние 100 точек (100 * 5 секунд = 500 секунд = ~8 минут)
			timeSeriesStats = timeSeriesStats[1:]
		}
		timeSeriesMutex.Unlock()
		log.Printf("Агрегирована статистика: U:%d, D:%d, ActiveConn:%d", totalUpload, totalDownload, currentActiveConnections)
	}
}

// --- Веб-панель (Бэкенд и Фронтенд) ---

func startWebServer() {
	r := gin.Default()

	r.SetHTMLTemplate(parseTemplate())
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	// API-эндпоинты
	r.GET("/api/users", getUsers)
	r.POST("/api/users", createUser)
	r.PUT("/api/users/:username", updateUser)
	r.DELETE("/api/users/:username", deleteUser)
	r.GET("/api/stats", getTrafficStats)
	r.GET("/api/total_stats", getTotalTrafficStats)
	r.GET("/api/time_series_stats", getTimeSeriesStats)

	log.Println("Веб-панель The-ASTRACAT-SOCKS-Eliza запущена на http://localhost:3434")
	if err := r.Run(":3434"); err != nil { // Изменен порт на 3434
		log.Fatalf("Ошибка при запуске веб-сервера: %v", err)
	}
}

// Функции API
func getUsers(c *gin.Context) {
	usersMutex.RLock()
	defer usersMutex.RUnlock()

	userList := make([]User, 0, len(users))
	for _, user := range users {
		userList = append(userList, user)
	}
	c.JSON(http.StatusOK, userList)
}

type UserCreationRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Enabled  bool   `json:"enabled"`
}

func createUser(c *gin.Context) {
	var req UserCreationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	usersMutex.Lock()
	defer usersMutex.Unlock()

	if _, ok := users[req.Username]; ok {
		c.JSON(http.StatusConflict, gin.H{"error": "Пользователь с таким именем уже существует"})
		return
	}

	users[req.Username] = User{Username: req.Username, Password: req.Password, Enabled: req.Enabled}
	c.JSON(http.StatusCreated, gin.H{"message": "Пользователь успешно создан", "user": users[req.Username]})
}

func updateUser(c *gin.Context) {
	username := c.Param("username")
	var req UserCreationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	usersMutex.Lock()
	defer usersMutex.Unlock()

	user, ok := users[username]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Пользователь не найден"})
		return
	}

	user.Password = req.Password
	user.Enabled = req.Enabled
	users[username] = user

	c.JSON(http.StatusOK, gin.H{"message": "Пользователь успешно обновлен", "user": users[username]})
}

func deleteUser(c *gin.Context) {
	username := c.Param("username")

	usersMutex.Lock()
	defer usersMutex.Unlock()

	if _, ok := users[username]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Пользователь не найден"})
		return
	}

	delete(users, username)
	delete(trafficStats, username) // Очищаем статистику для удаленного пользователя

	c.JSON(http.StatusOK, gin.H{"message": "Пользователь успешно удален"})
}

func getTrafficStats(c *gin.Context) {
	trafficMutex.RLock()
	defer trafficMutex.RUnlock()

	statsList := make([]map[string]interface{}, 0, len(trafficStats))
	for username, stats := range trafficStats {
		statsList = append(statsList, map[string]interface{}{
			"username":      username,
			"uploadBytes":   stats.UploadBytes,
			"downloadBytes": stats.DownloadBytes,
		})
	}
	c.JSON(http.StatusOK, statsList)
}

func getTotalTrafficStats(c *gin.Context) {
	trafficMutex.RLock()
	defer trafficMutex.RUnlock()

	var totalUpload int64
	var totalDownload int64

	for _, stats := range trafficStats {
		totalUpload += stats.UploadBytes
		totalDownload += stats.DownloadBytes
	}

	activeConnectionsMutex.Lock()
	currentActiveConnections := activeConnectionsCounter
	activeConnectionsMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"totalUploadBytes":   totalUpload,
		"totalDownloadBytes": totalDownload,
		"activeConnections":  currentActiveConnections,
	})
}

func getTimeSeriesStats(c *gin.Context) {
	timeSeriesMutex.RLock()
	defer timeSeriesMutex.RUnlock()
	c.JSON(http.StatusOK, timeSeriesStats)
}

// parseTemplate парсит HTML-шаблон для фронтенда
func parseTemplate() *template.Template {
	htmlContent := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>The-ASTRACAT-SOCKS-Eliza Admin Panel</title> <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        :root {
            --background: #09090b;
            --foreground: #fafafa;
            --card: #18181b;
            --card-foreground: #fafafa;
            --popover: #09090b;
            --popover-foreground: #fafafa;
            --primary: #a855f7; /* Purple */
            --primary-foreground: #fafafa;
            --secondary: #27272a;
            --secondary-foreground: #fafafa;
            --muted: #27272a;
            --muted-foreground: #a1a1aa;
            --accent: #27272a;
            --accent-foreground: #fafafa;
            --destructive: #ef4444;
            --destructive-foreground: #fafafa;
            --border: #27272a;
            --input: #27272a;
            --ring: #a855f7;
            --radius: 0.5rem;
        }

        body {
            font-family: Arial, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: var(--background);
            color: var(--foreground);
            line-height: 1.6;
        }

        .container {
            max-width: 1200px;
            margin: auto;
            display: grid;
            grid-template-columns: 1fr;
            gap: 20px;
        }

        @media (min-width: 768px) {
            .container {
                grid-template-columns: 2fr 1fr;
            }
            .full-width-card {
                 grid-column: 1 / -1;
            }
        }

        .card {
            background-color: var(--card);
            border: 1px solid var(--border);
            border-radius: var(--radius);
            padding: 20px;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
        }

        h1, h2, h3 {
            color: var(--primary);
            margin-top: 0;
            margin-bottom: 15px;
        }

        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
            color: var(--foreground);
        }

        th, td {
            border: 1px solid var(--border);
            padding: 10px;
            text-align: left;
        }

        th {
            background-color: var(--secondary);
            font-weight: bold;
        }

        .form-group {
            margin-bottom: 15px;
        }

        .form-group label {
            display: block;
            margin-bottom: 8px;
            font-weight: bold;
            color: var(--muted-foreground);
        }

        .form-group input[type="text"],
        .form-group input[type="password"] {
            width: calc(100% - 22px);
            padding: 10px;
            border: 1px solid var(--border);
            border-radius: var(--radius);
            background-color: var(--input);
            color: var(--foreground);
        }

        .form-group input[type="checkbox"] {
            margin-right: 8px;
            transform: scale(1.2);
        }

        .btn {
            background-color: var(--primary);
            color: var(--primary-foreground);
            padding: 10px 18px;
            border: none;
            border-radius: var(--radius);
            cursor: pointer;
            font-size: 1rem;
            margin-right: 10px;
            transition: background-color 0.2s ease;
        }

        .btn:hover {
            background-color: color-mix(in srgb, var(--primary) 90%, black);
        }

        .btn-secondary {
            background-color: var(--secondary);
            color: var(--secondary-foreground);
        }
        .btn-secondary:hover {
            background-color: color-mix(in srgb, var(--secondary) 90%, black);
        }

        .btn-danger {
            background-color: var(--destructive);
            color: var(--destructive-foreground);
        }
        .btn-danger:hover {
            background-color: color-mix(in srgb, var(--destructive) 90%, black);
        }

        .message {
            margin-top: 15px;
            padding: 10px;
            border-radius: var(--radius);
            font-weight: bold;
        }

        .message.success {
            background-color: #1e40af;
            color: #dbeafe;
            border: 1px solid #93c5fd;
        }
        .message.error {
            background-color: #7f1d1d;
            color: #fee2e2;
            border: 1px solid #fca5a5;
        }

        canvas {
            background-color: var(--popover);
            border-radius: var(--radius);
            padding: 10px;
        }

        .grid-3 {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .metric-card {
            background-color: var(--card);
            padding: 15px;
            border-radius: var(--radius);
            border: 1px solid var(--border);
            text-align: center;
        }
        .metric-card .value {
            font-size: 2em;
            font-weight: bold;
            color: var(--primary);
            margin-top: 5px;
        }
        .metric-card .label {
            color: var(--muted-foreground);
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="card full-width-card">
            <h1>The-ASTRACAT-SOCKS-Eliza Admin Panel</h1> <div class="message" id="message" style="display:none;"></div>

            <div class="grid-3">
                <div class="metric-card">
                    <div class="label">Total Upload</div>
                    <div class="value" id="totalUpload">0 KB</div>
                </div>
                <div class="metric-card">
                    <div class="label">Total Download</div>
                    <div class="value" id="totalDownload">0 KB</div>
                </div>
                <div class="metric-card">
                    <div class="label">Active Connections</div>
                    <div class="value" id="activeConnections">0</div>
                </div>
            </div>

            <h2>Traffic & Connections Over Time</h2>
            <div class="card" style="margin-bottom: 20px;">
                <canvas id="trafficChart"></canvas>
            </div>
        </div>

        <div class="card">
            <h2>Manage Users</h2>
            <form id="userForm">
                <div class="form-group">
                    <label for="username">Username:</label>
                    <input type="text" id="username" name="username" required>
                </div>
                <div class="form-group">
                    <label for="password">Password:</label>
                    <input type="password" id="password" name="password" required>
                </div>
                <div class="form-group">
                    <input type="checkbox" id="enabled" name="enabled" checked>
                    <label for="enabled">Enabled</label>
                </div>
                <button type="submit" class="btn">Add User</button>
                <button type="button" class="btn btn-secondary" id="updateUserBtn" style="display:none;">Update User</button>
                <button type="button" class="btn-danger" id="cancelEditBtn" style="display:none;">Cancel</button>
            </form>
        </div>

        <div class="card">
            <h3>User List</h3>
            <table id="userTable">
                <thead>
                    <tr>
                        <th>Username</th>
                        <th>Password (raw)</th>
                        <th>Enabled</th>
                        <th>Upload (KB)</th>
                        <th>Download (KB)</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    </tbody>
            </table>
        </div>
    </div>

    <script>
        const API_BASE_URL = '/api';
        const messageDiv = document.getElementById('message');
        const userForm = document.getElementById('userForm');
        const usernameInput = document.getElementById('username');
        const passwordInput = document.getElementById('password');
        const enabledInput = document.getElementById('enabled');
        const addUserBtn = userForm.querySelector('button[type="submit"]');
        const updateUserBtn = document.getElementById('updateUserBtn');
        const cancelEditBtn = document.getElementById('cancelEditBtn');
        const userTableBody = document.querySelector('#userTable tbody');
        
        const totalUploadSpan = document.getElementById('totalUpload');
        const totalDownloadSpan = document.getElementById('totalDownload');
        const activeConnectionsSpan = document.getElementById('activeConnections');

        let editingUsername = null;
        let trafficChart;

        document.addEventListener('DOMContentLoaded', () => {
            const ctx = document.getElementById('trafficChart').getContext('2d');
            trafficChart = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: [],
                    datasets: [
                        {
                            label: 'Upload (KB)',
                            data: [],
                            borderColor: 'rgb(168, 85, 247)',
                            backgroundColor: 'rgba(168, 85, 247, 0.2)',
                            fill: true,
                            tension: 0.1
                        },
                        {
                            label: 'Download (KB)',
                            data: [],
                            borderColor: 'rgb(34, 197, 94)',
                            backgroundColor: 'rgba(34, 197, 94, 0.2)',
                            fill: true,
                            tension: 0.1
                        },
                        {
                            label: 'Active Connections',
                            data: [],
                            borderColor: 'rgb(59, 130, 246)',
                            backgroundColor: 'rgba(59, 130, 246, 0.2)',
                            fill: false,
                            tension: 0.1,
                            yAxisID: 'y1'
                        }
                    ]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    scales: {
                        x: {
                            type: 'time',
                            time: {
                                unit: 'second',
                                displayFormats: {
                                    second: 'HH:mm:ss'
                                }
                            },
                            title: {
                                display: true,
                                text: 'Time',
                                color: 'var(--muted-foreground)'
                            },
                            ticks: {
                                color: 'var(--muted-foreground)'
                            },
                            grid: {
                                color: 'var(--border)'
                            }
                        },
                        y: {
                            type: 'linear',
                            display: true,
                            position: 'left',
                            title: {
                                display: true,
                                text: 'Traffic (KB)',
                                color: 'var(--muted-foreground)'
                            },
                            ticks: {
                                color: 'var(--muted-foreground)'
                            },
                            grid: {
                                color: 'var(--border)'
                            }
                        },
                        y1: {
                            type: 'linear',
                            display: true,
                            position: 'right',
                            title: {
                                display: true,
                                text: 'Connections',
                                color: 'var(--muted-foreground)'
                            },
                            grid: {
                                drawOnChartArea: false,
                                color: 'var(--border)'
                            },
                            ticks: {
                                color: 'var(--muted-foreground)',
                                callback: function(value, index, values) {
                                    if (Number.isInteger(value)) {
                                        return value;
                                    }
                                    return null;
                                }
                            }
                        }
                    },
                    plugins: {
                        legend: {
                            labels: {
                                color: 'var(--muted-foreground)'
                            }
                        }
                    }
                }
            });
        });


        function showMessage(msg, type) {
            messageDiv.textContent = msg;
            messageDiv.className = `message ${type}`;
            messageDiv.style.display = 'block';
            setTimeout(() => {
                messageDiv.style.display = 'none';
            }, 3000);
        }

        async function fetchAllData() {
            try {
                const [usersRes, statsRes, totalStatsRes, timeSeriesRes] = await Promise.all([
                    fetch(`${API_BASE_URL}/users`),
                    fetch(`${API_BASE_URL}/stats`),
                    fetch(`${API_BASE_URL}/total_stats`),
                    fetch(`${API_BASE_URL}/time_series_stats`)
                ]);

                if (!usersRes.ok) throw new Error(`HTTP error! status: ${usersRes.status}`);
                if (!statsRes.ok) throw new Error(`HTTP error! status: ${statsRes.status}`);
                if (!totalStatsRes.ok) throw new Error(`HTTP error! status: ${totalStatsRes.status}`);
                if (!timeSeriesRes.ok) throw new Error(`HTTP error! status: ${timeSeriesRes.status}`);

                const users = await usersRes.json();
                const stats = await statsRes.json();
                const totalStats = await totalStatsRes.json();
                const timeSeriesData = await timeSeriesRes.json();

                renderUserTable(users, stats);
                renderTotalStats(totalStats);
                updateChart(timeSeriesData);
            } catch (error) {
                console.error('Error fetching data:', error);
                showMessage(`Error loading data: ${error.message}`, 'error');
            }
        }

        function renderUserTable(users, stats) {
            userTableBody.innerHTML = '';
            const statsMap = new Map(stats.map(s => [s.username, s]));

            if (users.length === 0) {
                userTableBody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No users found.</td></tr>';
                return;
            }

            users.forEach(user => {
                const row = userTableBody.insertRow();
                const userStats = statsMap.get(user.username) || { uploadBytes: 0, downloadBytes: 0 };

                row.insertCell().textContent = user.username;
                row.insertCell().textContent = user.password;
                row.insertCell().textContent = user.enabled ? 'Yes' : 'No';
                row.insertCell().textContent = (userStats.uploadBytes / 1024).toFixed(2);
                row.insertCell().textContent = (userStats.downloadBytes / 1024).toFixed(2);

                const actionsCell = row.insertCell();
                const editBtn = document.createElement('button');
                editBtn.textContent = 'Edit';
                editBtn.className = 'btn btn-secondary';
                editBtn.onclick = () => editUser(user);
                actionsCell.appendChild(editBtn);

                const deleteBtn = document.createElement('button');
                deleteBtn.textContent = 'Delete';
                deleteBtn.className = 'btn btn-danger';
                deleteBtn.onclick = () => deleteUser(user.username);
                actionsCell.appendChild(deleteBtn);
            });
        }

        function renderTotalStats(totalStats) {
            totalUploadSpan.textContent = (totalStats.totalUploadBytes / 1024).toFixed(2) + ' KB';
            totalDownloadSpan.textContent = (totalStats.totalDownloadBytes / 1024).toFixed(2) + ' KB';
            activeConnectionsSpan.textContent = totalStats.activeConnections;
        }

        function updateChart(timeSeriesData) {
            const labels = timeSeriesData.map(d => new Date(d.timestamp * 1000));
            const uploadData = timeSeriesData.map(d => (d.uploadBytes / 1024).toFixed(2));
            const downloadData = timeSeriesData.map(d => (d.downloadBytes / 1024).toFixed(2));
            const activeConnData = timeSeriesData.map(d => d.activeConnections);

            trafficChart.data.labels = labels;
            trafficChart.data.datasets[0].data = uploadData;
            trafficChart.data.datasets[1].data = downloadData;
            trafficChart.data.datasets[2].data = activeConnData;
            trafficChart.update();
        }

        function editUser(user) {
            editingUsername = user.username;
            usernameInput.value = user.username;
            passwordInput.value = user.password;
            enabledInput.checked = user.enabled;

            usernameInput.disabled = true;
            addUserBtn.style.display = 'none';
            updateUserBtn.style.display = 'inline-block';
            cancelEditBtn.style.display = 'inline-block';
        }

        function cancelEdit() {
            editingUsername = null;
            userForm.reset();
            usernameInput.disabled = false;
            addUserBtn.style.display = 'inline-block';
            updateUserBtn.style.display = 'none';
            cancelEditBtn.style.display = 'none';
        }

        async function submitUserForm(event) {
            event.preventDefault();

            const userData = {
                username: usernameInput.value,
                password: passwordInput.value,
                enabled: enabledInput.checked
            };

            let url = `${API_BASE_URL}/users`;
            let method = 'POST';

            if (editingUsername) {
                url = `${API_BASE_URL}/users/${editingUsername}`;
                method = 'PUT';
            }

            try {
                const response = await fetch(url, {
                    method: method,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(userData)
                });

                const result = await response.json();

                if (response.ok) {
                    showMessage(result.message, 'success');
                    userForm.reset();
                    cancelEdit();
                    fetchAllData();
                } else {
                    showMessage(`Error: ${result.error || response.statusText}`, 'error');
                }
            } catch (error) {
                console.error('Error submitting form:', error);
                showMessage(`Network error: ${error.message}`, 'error');
            }
        }

        async function deleteUser(username) {
            if (!confirm(`Are you sure you want to delete user ${username}? This will also clear their traffic stats.`)) {
                return;
            }

            try {
                const response = await fetch(`${API_BASE_URL}/users/${username}`, {
                    method: 'DELETE'
                });

                const result = await response.json();

                if (response.ok) {
                    showMessage(result.message, 'success');
                    fetchAllData();
                } else {
                    showMessage(`Error: ${result.error || response.statusText}`, 'error');
                }
            } catch (error) {
                console.error('Error deleting user:', error);
                showMessage(`Network error: ${error.message}`, 'error');
            }
        }

        userForm.addEventListener('submit', submitUserForm);
        updateUserBtn.addEventListener('click', submitUserForm);
        cancelEditBtn.addEventListener('click', cancelEdit);

        fetchAllData();
        setInterval(fetchAllData, 5000);
    </script>
</body>
</html>
`
	tmpl := template.New("index.html")
	tmpl, err := tmpl.Parse(htmlContent)
	if err != nil {
		log.Fatalf("Ошибка при парсинге HTML шаблона: %v", err)
	}
	return tmpl
}
