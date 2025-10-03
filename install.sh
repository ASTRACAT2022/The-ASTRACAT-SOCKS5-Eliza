#!/bin/bash

# === Параметры ===
PROJECT_DIR="/opt/astra-socks-eliza"
USER="root"
PORT_PROXY="7777"
PORT_DASHBOARD="8080"
GEO_DB_PATH="/usr/share/GeoIP/GeoLite2-Country.mmdb"
GEO_DB_URL="https://geolite.maxmind.com/download/geoip/database/GeoLite2-Country.mmdb"

# === Проверка прав ===
if [[ $EUID -ne 0 ]]; then
   echo "Ошибка: Этот скрипт должен быть запущен от root."
   exit 1
fi

# === Шаг 1: Подготовка ===
echo "[+] Подготовка: Установка зависимостей и клонирование репозитория..."

# Установка Go (если не установлен)
if ! command -v go &> /dev/null; then
    echo "[-] Go не найден. Установка Go (версия 1.18+)"
    # Установка Go (пример для Ubuntu/Debian)
    wget https://golang.org/dl/go1.21.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.21.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo "export PATH=\$PATH:/usr/local/go/bin" >> /etc/profile.d/go.sh
    echo "Go установлен."
else
    echo "[+] Go уже установлен."
fi

# Клонирование репозитория
if [ ! -d "$PROJECT_DIR" ]; then
    echo "[+] Клонирование репозитория..."
    git clone https://github.com/ASTRACAT2022/The-ASTRACAT-SOKS-Eliza.git "$PROJECT_DIR"
    cd "$PROJECT_DIR"
else
    echo "[+] Репозиторий уже существует. Обновление..."
    cd "$PROJECT_DIR"
    git pull
fi

# === Шаг 2: Настройка геолокации (ОПЦИОНАЛЬНО) ===
echo "[+] Настройка геолокации (GeoLite2-Country.mmdb)..."
if [ ! -f "$GEO_DB_PATH" ]; then
    echo "[-] База данных GeoLite2 не найдена. Попытка загрузки..."
    mkdir -p /usr/share/GeoIP
    wget -O "$GEO_DB_PATH" "$GEO_DB_URL" || echo "[!] Не удалось загрузить GeoLite2-Country.mmdb"
    echo "[+] База данных установлена в $GEO_DB_PATH"
else
    echo "[+] База данных уже установлена."
fi

# === Шаг 3: Сборка ===
echo "[+] Сборка проекта..."

cd "$PROJECT_DIR"

# Загрузка зависимостей
go mod tidy

# Сборка прокси-сервера
echo "[+] Сборка прокси-сервера..."
go build -o astra_socks_eliza main.go
if [ $? -ne 0 ]; then
    echo "[-] Ошибка при сборке прокси-сервера."
    exit 1
fi

# Сборка панели мониторинга
echo "[+] Сборка панели мониторинга..."
go build -o eliza_dashboard dashboard/dashboard.go
if [ $? -ne 0 ]; then
    echo "[-] Ошибка при сборке панели мониторинга."
    exit 1
fi

# === Шаг 4: Настройка systemd ===
echo "[+] Настройка systemd..."

# Создание директории для конфигурации
mkdir -p /etc/astra-socks-eliza

# Файл прокси-сервера
cat > /etc/systemd/system/astra-socks-eliza.service << EOF
[Unit]
Description=The-ASTRACAT-SOCKS-Eliza Proxy Server
After=network.target

[Service]
User=$USER
Group=$USER
WorkingDirectory=$PROJECT_DIR
ExecStart=$PROJECT_DIR/astra_socks_eliza
StandardOutput=null
StandardError=journal
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Файл панели мониторинга
cat > /etc/systemd/system/eliza-dashboard.service << EOF
[Unit]
Description=Dashboard for The-ASTRACAT-SOCKS-Eliza
After=network.target

[Service]
User=$USER
Group=$USER
WorkingDirectory=$PROJECT_DIR
ExecStart=$PROJECT_DIR/eliza_dashboard -port $PORT_DASHBOARD
Restart=always

[Install]
WantedBy=multi-user.target
EOF

echo "[+] Сервисы systemd созданы."

# === Шаг 5: Запуск сервисов ===
echo "[+] Перезагрузка systemd и запуск сервисов..."

systemctl daemon-reload
systemctl enable --now astra-socks-eliza.service
systemctl enable --now eliza-dashboard.service

echo "[+] Сервисы запущены."

# === Шаг 6: Настройка брандмауэра ===
echo "[+] Настройка брандмауэра (UFW)..."
if command -v ufw &> /dev/null; then
    ufw allow $PORT_PROXY/tcp
    ufw allow $PORT_DASHBOARD/tcp
    ufw enable
    echo "[+] Порты $PORT_PROXY и $PORT_DASHBOARD открыты."
else
    echo "[!] UFW не найден. Убедитесь, что брандмауэр настроен вручную."
fi

# === Шаг 7: Проверка пользователей ===
echo "[+] Настройка пользователей..."

if [ ! -f "/etc/astra_socks_eliza/users.json" ]; then
    echo "[+] Создание файла пользователей..."
    cat > /etc/astra_socks_eliza/users.json << EOF
{
  "users": [
    {
      "username": "astranet",
      "password": "astranet",
      "limit": 1000000000,
      "expire": 0
    }
  ]
}
EOF
    echo "[+] Пользователь astranet:astranet создан."
fi

# === Завершение ===
echo ""
echo "=================================================="
echo "✅ Установка завершена!"
echo ""
echo "Информация:"
echo "  Прокси-сервер: http://$IP:$PORT_PROXY"
echo "  Панель мониторинга: http://$IP:$PORT_DASHBOARD"
echo "  Пользователи: /etc/astra_socks_eliza/users.json"
echo ""
echo "Для управления пользователями:"
echo "  sudo nano /etc/astra_socks_eliza/users.json"
echo "  sudo systemctl restart astra-socks-eliza"
echo "=================================================="
