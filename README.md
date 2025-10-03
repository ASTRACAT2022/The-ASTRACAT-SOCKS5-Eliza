# The-ASTRACAT-SOCKS-Eliza

The-ASTRACAT-SOCKS-Eliza — это легковесный SOCKS5 прокси-сервер на Go с аутентификацией по логину/паролю и встроенной **веб-панелью для мониторинга трафика в реальном времени**.

## Особенности

- **SOCKS5 Проксирование:** Поддержка стандартного протокола SOCKS5.
- **Аутентификация:** Защита доступа с помощью логина и пароля.
- **Управление пользователями:** Пользователи легко управляются через редактирование JSON-файла.
- **Веб-панель мониторинга ("Трафик-Радар"):**
    - **Сводная статистика:** Активные соединения, общий трафик (upload/download).
    - **Статистика по пользователям:** Графики и таблицы с трафиком для каждого пользователя.
    - **Карта трафика:** Интерактивная карта мира, показывающая, из каких стран идут подключения, с визуализацией объема трафика.
    - **Настраиваемый порт:** Панель мониторинга может быть запущена на любом порту.
- **Геолокация:** Автоматическое определение страны клиента по IP-адресу (требуется база данных GeoLite2).
- **Минимальное логирование:** Логируются только критические события и успешные аутентификации для экономии дискового пространства.
- **Высокая производительность:** Низкое потребление ресурсов благодаря Go.

## Установка

### 1. Подготовка

- Убедитесь, что у вас установлен **Go (версия 1.18 или новее)**.
- Клонируйте репозиторий:
  ```bash
  git clone https://github.com/ASTRACAT2022/The-ASTRACAT-SOKS-Eliza.git
  cd The-ASTRACAT-SOKS-Eliza
  ```

### 2. (Опционально) Настройка геолокации

Для работы карты трафика необходима база данных **GeoLite2 Country**.

1.  Скачайте бесплатную базу данных `GeoLite2-Country.mmdb` с официального сайта [MaxMind](https://www.maxmind.com/en/geolite2/signup).
2.  Создайте директорию и поместите в нее файл базы данных:
    ```bash
    sudo mkdir -p /usr/share/GeoIP
    sudo mv /путь/к/вашему/GeoLite2-Country.mmdb /usr/share/GeoIP/GeoLite2-Country.mmdb
    ```
> **Примечание:** Если база данных не будет найдена, прокси-сервер и панель мониторинга все равно будут работать, но без сбора и отображения геолокационной статистики.

### 3. Сборка

Проект состоит из двух частей: самого прокси-сервера и сервера для панели мониторинга.

1.  **Загрузка зависимостей:**
    ```bash
    go mod tidy
    ```
2.  **Сборка прокси-сервера:**
    ```bash
    go build -o astra_socks_eliza main.go
    ```
3.  **Сборка сервера панели мониторинга:**
    ```bash
    go build -o eliza_dashboard dashboard/dashboard.go
    ```

### 4. Настройка сервисов `systemd`

Рекомендуется настроить автозапуск обоих сервисов с помощью `systemd`.

#### а) Сервис для прокси (`astra-socks-eliza.service`)

Создайте файл `/etc/systemd/system/astra-socks-eliza.service`:
```bash
sudo nano /etc/systemd/system/astra-socks-eliza.service
```
Вставьте следующее содержимое (замените `/path/to/project` на ваш реальный путь):
```ini
[Unit]
Description=The-ASTRACAT-SOCKS-Eliza Proxy Server
After=network.target

[Service]
User=root
Group=root
WorkingDirectory=/path/to/project
ExecStart=/path/to/project/astra_socks_eliza
StandardOutput=null
StandardError=journal
Restart=always

[Install]
WantedBy=multi-user.target
```

#### б) Сервис для панели мониторинга (`eliza-dashboard.service`)

Создайте файл `/etc/systemd/system/eliza-dashboard.service`:
```bash
sudo nano /etc/systemd/system/eliza-dashboard.service
```
Вставьте следующее содержимое (замените `/path/to/project` и порт, если нужно):
```ini
[Unit]
Description=Dashboard for The-ASTRACAT-SOCKS-Eliza
After=network.target

[Service]
User=root
Group=root
WorkingDirectory=/path/to/project
# Запуск панели на порту 8080. Вы можете изменить порт.
ExecStart=/path/to/project/eliza_dashboard -port 8080
Restart=always

[Install]
WantedBy=multi-user.target
```

### 5. Запуск сервисов

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now astra-socks-eliza.service
sudo systemctl enable --now eliza-dashboard.service
```

### 6. Настройка брандмауэра

Не забудьте открыть порты в вашем брандмауэре (например, UFW):
```bash
sudo ufw allow 7777/tcp  # Порт для SOCKS5 прокси
sudo ufw allow 8080/tcp # Порт для панели мониторинга
sudo ufw enable
```

## Использование

### Прокси-сервер

- **Адрес:** `ВАШ_IP_СЕРВЕРА`
- **Порт:** `7777`
- **Аутентификация:** Логин/пароль

### Управление пользователями

Пользователи хранятся в файле `/etc/astra_socks_eliza/users.json`. При первом запуске он создается автоматически с пользователем `astranet:astranet`.

Для добавления или изменения пользователей отредактируйте этот файл и перезапустите сервис прокси:
```bash
sudo nano /etc/astra_socks_eliza/users.json
sudo systemctl restart astra-socks-eliza
```

### Панель мониторинга

Откройте в браузере `http://ВАШ_IP_СЕРВЕРА:8080` (или другой порт, который вы указали). Панель обновляется автоматически каждые 5 секунд.

#### Параметры запуска панели мониторинга

- `-port`: Порт для веб-сервера (по умолчанию: `8080`).
- `-stats-file`: Путь к файлу статистики (по умолчанию: `/var/lib/astra_socks_eliza/stats.json`).

Пример запуска вручную на порту 9000:
```bash
./eliza_dashboard -port 9000
```