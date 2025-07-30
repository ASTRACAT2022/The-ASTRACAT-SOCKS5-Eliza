package main

import (
	"context" // Добавьте импорт context
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time" // Добавьте импорт time

	"github.com/go-redis/redis/v8" // Импорт go-redis
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

// Глобальный клиент Redis
var redisClient *redis.Client
// Контекст для Redis операций
var ctx = context.Background()

func main() {
	// Инициализация Redis клиента
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Адрес вашего Redis сервера
		Password: "",           // Пароль Redis, если есть
		DB: 0,                  // Номер базы данных
	})

	// Проверяем соединение с Redis
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Не удалось подключиться к Redis: %v", err)
	}
	log.Println("Успешно подключено к Redis!")

	listener, err := net.Listen("tcp", "0.0.0.0:7777")
	if err != nil {
		log.Fatalf("Ошибка при запуске SOCKS5 сервера: %v", err)
	}
	defer listener.Close()
	log.Println("SOCKS5 сервер запущен на 0.0.0.0:7777 с аутентификацией логин/пароль")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка при приёме соединения: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
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

// socks5Handshake теперь возвращает имя пользователя при успешной аутентификации
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

	// Передаем результат аутентификации
	username, err := authenticateUserPass(conn)
	if err != nil {
		return "", err
	}
	return username, nil
}

// authenticateUserPass теперь возвращает имя пользователя
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
	username := string(usernameBuf) // Сохраняем имя пользователя

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

	if username == "astranet" && string(password) == "astranet" {
		log.Printf("Аутентификация успешна для пользователя: %s", username)
		_, err = conn.Write([]byte{0x01, replySuccess})
		if err != nil {
			return "", fmt.Errorf("ошибка отправки ответа об успешной аутентификации: %w", err)
		}
		return username, nil // Возвращаем имя пользователя
	} else {
		log.Printf("Аутентификация не удалась для пользователя: %s", username)
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", fmt.Errorf("неверные имя пользователя или пароль")
	}
}

// handleSocks5Request теперь принимает имя пользователя
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
	// Передаем имя пользователя в proxyData
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

// proxyData теперь собирает статистику и отправляет её в Redis
func proxyData(clientConn, targetConn net.Conn, username string) error {
	done := make(chan error, 2)

	// Создаем обертки для подсчета трафика
	clientWriter := &customWriter{Writer: clientConn}
	targetWriter := &customWriter{Writer: targetConn}

	// Копируем данные от клиента к целевому серверу и считаем трафик
	go func() {
		_, err := io.Copy(targetWriter, clientConn) // clientConn (Reader) -> targetWriter (Writer)
		done <- err
	}()

	// Копируем данные от целевого сервера к клиенту и считаем трафик
	go func() {
		_, err := io.Copy(clientWriter, targetConn) // targetConn (Reader) -> clientWriter (Writer)
		done <- err
	}()

	err1 := <-done
	err2 := <-done

	// После завершения соединения, отправляем статистику в Redis
	// Ключи Redis: `user:astranet:download_bytes`, `user:astranet:upload_bytes`
	// Используем HINCRBY для атомарного увеличения счетчиков
	if targetWriter.bytesWritten > 0 { // Байты, отправленные от клиента через прокси на целевой сервер (UPLOAD)
		redisClient.HIncrBy(ctx, "user_traffic:"+username, "upload_bytes", targetWriter.bytesWritten)
		log.Printf("Пользователь %s: загружено %d байт (UPLOAD)", username, targetWriter.bytesWritten)
	}
	if clientWriter.bytesWritten > 0 { // Байты, полученные от целевого сервера и отправленные клиенту (DOWNLOAD)
		redisClient.HIncrBy(ctx, "user_traffic:"+username, "download_bytes", clientWriter.bytesWritten)
		log.Printf("Пользователь %s: скачано %d байт (DOWNLOAD)", username, clientWriter.bytesWritten)
	}

	if err1 != nil && err1 != io.EOF {
		return fmt.Errorf("ошибка копирования клиент -> цель: %w", err1)
	}
	if err2 != nil && err2 != io.EOF {
		return fmt.Errorf("ошибка копирования цель -> клиент: %w", err2)
	}
	return nil
}
