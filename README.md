Превосходно\! Это замечательно, что всё заработало как надо.

Теперь давайте создадим файл `README.md` для вашего проекта. Этот файл является визитной карточкой проекта и содержит основную информацию о нём.

-----

Создайте файл `README.md` в корневой директории вашего проекта:

```bash
cd /root/The-ASTRACAT-SOKS-Eliza
nano README.md
```

Затем скопируйте и вставьте следующее содержимое:

````markdown
# The-ASTRACAT-SOCKS-Eliza

The-ASTRACAT-SOCKS-Eliza — это легковесный SOCKS5 прокси-сервер с аутентификацией по логину/паролю, написанный на Go. Он предназначен для простой установки и использования на серверах Linux с минимальным объемом логов и доступом к статистике по запросу.

## Особенности

* **SOCKS5 Проксирование:** Поддерживает базовое SOCKS5 проксирование.
* **Аутентификация:** Требует аутентификации по логину и паролю.
* **Управление пользователями через файл:** Пользователи хранятся в простом JSON-файле, который можно легко редактировать.
* **Минимальное логирование:** Основной сервис логирует только критические события и успешные аутентификации в `systemd journal`, чтобы не засорять логи.
* **Статистика по запросу:** Общая статистика трафика и активных соединений, а также трафик по каждому пользователю сохраняется в JSON-файл и может быть просмотрена с помощью отдельной утилиты.
* **Легковесный:** Написан на Go, что обеспечивает высокую производительность и низкое потребление ресурсов.

## Установка и запуск

Следуйте этим шагам для установки и запуска The-ASTRACAT-SOCKS-Eliza на вашей системе Linux. Предполагается, что у вас уже установлен Go.

### 1. Клонирование репозитория (или создание файлов вручную)
git clone https://github.com/ASTRACAT2022/The-ASTRACAT-SOKS-Eliza.git
Если у вас нет репозитория, создайте директорию проекта и файлы `main.go` и `get_eliza_stats.go` вручную.

```bash
# Создайте директорию проекта, если она еще не существует
mkdir -p /root/The-ASTRACAT-SOKS-Eliza
cd /root/The-ASTRACAT-SOKS-Eliza

# Создайте файл main.go и вставьте в него код (см. выше в истории чата или из вашего рабочего файла)
nano main.go

# Создайте файл get_eliza_stats.go (где угодно, потом переместим)
nano ~/get_eliza_stats.go
# Вставьте код для утилиты (см. выше в истории чата)
````

### 2\. Сборка основного прокси-сервера

Перейдите в директорию проекта и скомпилируйте `main.go`:

```bash
cd /root/The-ASTRACAT-SOKS-Eliza
go build -o astra_socks_eliza main.go
```

### 3\. Сборка утилиты для просмотра статистики

Перейдите в вашу домашнюю директорию, скомпилируйте утилиту и переместите её в системный PATH:

```bash
cd ~
go build -o get_eliza_stats get_eliza_stats.go
sudo mv get_eliza_stats /usr/local/bin/
```

### 4\. Настройка сервиса `systemd`

Создайте файл юнита `systemd` для автоматического запуска прокси-сервера:

```bash
sudo nano /etc/systemd/system/astra-socks-eliza.service
```

Вставьте следующее содержимое:

```ini
[Unit]
Description=The-ASTRACAT-SOCKS-Eliza Proxy Server
After=network.target

[Service]
User=root
Group=root
WorkingDirectory=/root/The-ASTRACAT-SOKS-Eliza
ExecStart=/root/The-ASTRACAT-SOKS-Eliza/astra_socks_eliza
StandardOutput=null      # Перенаправляем стандартный вывод в /dev/null
StandardError=journal    # Ошибки будут попадать в journald
Restart=always
RestartSec=5s
JournalMaxRetentionSec=10m # Хранить логи этого сервиса в journald не более 10 минут

[Install]
WantedBy=multi-user.target
```

Сохраните файл (`Ctrl+O`, `Enter`, `Ctrl+X`).

### 5\. Запуск сервиса

Перезагрузите `systemd`, включите автозапуск и запустите сервис:

```bash
sudo systemctl daemon-reload
sudo systemctl enable astra-socks-eliza
sudo systemctl start astra-socks-eliza
```

### 6\. Проверка статуса сервиса

Убедитесь, что прокси-сервер запущен:

```bash
sudo systemctl status astra-socks-eliza
```

Вывод должен показать `Active: active (running)`.

### 7\. Настройка правил брандмауэра (UFW)

Если вы используете UFW, разрешите входящие соединения на порт `7777` (или любой другой порт, который вы настроили):

```bash
sudo ufw allow 7777/tcp
sudo ufw enable # Включите UFW, если он еще не активен. Будьте осторожны!
```

## Использование

### Проксирование

Прокси-сервер SOCKS5 будет слушать на порту `7777`. Вы можете настроить свой клиент (браузер, приложение) для использования прокси по адресу `ВАШ_IP_СЕРВЕРА:7777` с аутентификацией по логину/паролю.

### Управление пользователями

Пользователи хранятся в JSON-файле по пути `/etc/astra_socks_eliza/users.json`. При первом запуске сервиса, если этот файл не существует, он будет создан с тестовым пользователем `astranet:astranet`.

Чтобы добавить, удалить или изменить пользователей:

1.  Отредактируйте файл `users.json`:
    ```bash
    sudo nano /etc/astra_socks_eliza/users.json
    ```
    Пример содержимого:
    ```json
    {
      "astranet": {
        "username": "astranet",
        "password": "astranet",
        "enabled": true
      },
      "newuser": {
        "username": "newuser",
        "password": "strong_password",
        "enabled": true
      },
      "disabled_user": {
        "username": "disabled_user",
        "password": "password123",
        "enabled": false
      }
    }
    ```
    **Важно:** Убедитесь, что JSON-формат корректен.
2.  После сохранения изменений, перезапустите сервис, чтобы он загрузил обновленный список пользователей:
    ```bash
    sudo systemctl restart astra-socks-eliza
    ```

### Просмотр статистики

Для просмотра текущей статистики трафика и активных соединений используйте утилиту `get_eliza_stats`:

```bash
get_eliza_stats
```

Статистика обновляется каждые 5 секунд и сохраняется в `/var/lib/astra_socks_eliza/stats.json`.

### Просмотр логов

Основные события и ошибки можно просмотреть с помощью `journalctl`:

```bash
sudo journalctl -u astra-socks-eliza -f
```

Логи хранятся не более 10 минут.

## Разработка

Для разработчиков, желающих внести изменения:

1.  Отредактируйте `main.go`.
2.  Пересоберите: `go build -o astra_socks_eliza main.go`.
3.  Перезапустите сервис: `sudo systemctl restart astra-socks-eliza`.

-----

```

---

