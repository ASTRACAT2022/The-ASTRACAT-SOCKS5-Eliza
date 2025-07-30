package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
)

const (
	socks5Version        = 0x05 // Версия SOCKS5
	noAuthRequired       = 0x00 // Метод аутентификации: без аутентификации
	usernamePasswordAuth = 0x02 // Метод аутентификации: логин/пароль
	connectCommand       = 0x01 // Команда CONNECT
	ipv4Address          = 0x01 // Тип адреса: IPv4
	domainNameAddress    = 0x03 // Тип адреса: Доменное имя
	ipv6Address          = 0x04 // Тип адреса: IPv6
	replySuccess         = 0x00 // Ответ: Успешно
)

// Main function to start the SOCKS5 server
func main() {
	// 1. Listen for incoming connections on port 7777
	listener, err := net.Listen("tcp", "0.0.0.0:7777")
	if err != nil {
		log.Fatalf("Ошибка при запуске SOCKS5 сервера: %v", err)
	}
	defer listener.Close()
	log.Println("SOCKS5 сервер запущен на 0.0.0.0:7777 с аутентификацией логин/пароль")

	for {
		// Accept a new connection
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка при приёме соединения: %v", err)
			continue
		}
		// Handle the connection in a new goroutine
		go handleConnection(conn)
	}
}

// handleConnection handles a single client connection
func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Новое соединение от: %s", conn.RemoteAddr())

	// Step 1: SOCKS5 Handshake
	if err := socks5Handshake(conn); err != nil {
		log.Printf("Ошибка SOCKS5 рукопожатия для %s: %v", conn.RemoteAddr(), err)
		return
	}

	// Step 2: Handle SOCKS5 Request (CONNECT)
	if err := handleSocks5Request(conn); err != nil {
		log.Printf("Ошибка SOCKS5 запроса для %s: %v", conn.RemoteAddr(), err)
		return
	}
}

// socks5Handshake performs the SOCKS5 handshake and authentication
func socks5Handshake(conn net.Conn) error {
	// Read SOCKS version and number of methods
	buf := make([]byte, 2)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return fmt.Errorf("ошибка чтения приветствия SOCKS5: %w", err)
	}

	if buf[0] != socks5Version {
		return fmt.Errorf("неподдерживаемая версия SOCKS: %d", buf[0])
	}

	// Read supported authentication methods
	numMethods := int(buf[1])
	methods := make([]byte, numMethods)
	_, err = io.ReadFull(conn, methods)
	if err != nil {
		return fmt.Errorf("ошибка чтения методов аутентификации: %w", err)
	}

	// Check if Username/Password authentication (0x02) is supported
	foundUserPassAuth := false
	for _, method := range methods {
		if method == usernamePasswordAuth {
			foundUserPassAuth = true
			break
		}
	}

	if !foundUserPassAuth {
		// If 0x02 is not supported, reply with "No acceptable methods"
		_, _ = conn.Write([]byte{socks5Version, 0xFF})
		return fmt.Errorf("нет поддерживаемых методов аутентификации (требуется 0x02)")
	}

	// Reply to client: SOCKS5, Username/Password authentication (0x02)
	_, err = conn.Write([]byte{socks5Version, usernamePasswordAuth})
	if err != nil {
		return fmt.Errorf("ошибка отправки подтверждения метода аутентификации: %w", err)
	}

	// Perform Username/Password authentication
	return authenticateUserPass(conn)
}

// authenticateUserPass performs Username/Password authentication (RFC 1929)
func authenticateUserPass(conn net.Conn) error {
	// Read version byte and username length byte
	buf := make([]byte, 2)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return fmt.Errorf("ошибка чтения заголовка аутентификации: %w", err)
	}

	// Check authentication protocol version (must be 0x01)
	if buf[0] != 0x01 {
		return fmt.Errorf("неподдерживаемая версия протокола аутентификации: %d", buf[0])
	}

	// Read username
	usernameLen := int(buf[1])
	username := make([]byte, usernameLen)
	_, err = io.ReadFull(conn, username)
	if err != nil {
		return fmt.Errorf("ошибка чтения имени пользователя: %w", err)
	}

	// Read password length byte and password
	_, err = io.ReadFull(conn, buf[0:1]) // Read 1 byte for password length
	if err != nil {
		return fmt.Errorf("ошибка чтения длины пароля: %w", err)
	}
	passwordLen := int(buf[0])
	password := make([]byte, passwordLen)
	_, err = io.ReadFull(conn, password)
	if err != nil {
		return fmt.Errorf("ошибка чтения пароля: %w", err)
	}

	// --- Hardcoded authentication check (replace with DB lookup later) ---
	if string(username) == "astranet" && string(password) == "astranet" {
		// Authentication successful
		log.Printf("Аутентификация успешна для пользователя: %s", username)
		_, err = conn.Write([]byte{0x01, replySuccess}) // Succeeded (0x00)
		if err != nil {
			return fmt.Errorf("ошибка отправки ответа об успешной аутентификации: %w", err)
		}
		return nil
	} else {
		// Authentication failed
		log.Printf("Аутентификация не удалась для пользователя: %s", username)
		_, err = conn.Write([]byte{0x01, 0x01}) // General failure (0x01)
		if err != nil {
			return fmt.Errorf("ошибка отправки ответа о неудачной аутентификации: %w", err)
		}
		return fmt.Errorf("неверные имя пользователя или пароль")
	}
}

// handleSocks5Request handles the client's request (CONNECT command)
func handleSocks5Request(conn net.Conn) error {
	// Read the first 4 bytes of the request: [VER | CMD | RSV | ATYP]
	buf := make([]byte, 4)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return fmt.Errorf("ошибка чтения заголовка запроса SOCKS5: %w", err)
	}

	if buf[0] != socks5Version {
		return fmt.Errorf("неподдерживаемая версия SOCKS в запросе: %d", buf[0])
	}

	if buf[1] != connectCommand {
		// Reply with "Command not supported" (0x07)
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
		// Reply with "Address type not supported" (0x08)
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
	log.Printf("Клиент %s запрашивает соединение с %s", conn.RemoteAddr(), target)

	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("Ошибка Dial к %s: %v", target, err)
		// Reply with "Connection refused/host unreachable" (0x05)
		_, _ = conn.Write([]byte{socks5Version, 0x05, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return fmt.Errorf("не удалось подключиться к целевому хосту: %w", err)
	}
	defer targetConn.Close()

	// Reply to client with success
	_, err = conn.Write([]byte{socks5Version, replySuccess, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		return fmt.Errorf("ошибка отправки ответа об успехе: %w", err)
	}

	log.Printf("Установлено прокси-соединение: %s <-> %s", conn.RemoteAddr(), target)
	return proxyData(conn, targetConn)
}

// proxyData redirects data between two connections
func proxyData(clientConn, targetConn net.Conn) error {
	done := make(chan error, 2)

	go func() {
		_, err := io.Copy(targetConn, clientConn)
		done <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, targetConn)
		done <- err
	}()

	err1 := <-done
	err2 := <-done

	if err1 != nil && err1 != io.EOF {
		return fmt.Errorf("ошибка копирования клиент -> цель: %w", err1)
	}
	if err2 != nil && err2 != io.EOF {
		return fmt.Errorf("ошибка копирования цель -> клиент: %w", err2)
	}
	return nil
}
