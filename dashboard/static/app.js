document.addEventListener('DOMContentLoaded', () => {
    const API_URL = '/api/stats';
    let trafficChart;
    let trafficMap;
    let mapMarkersLayer;

    const summaryCardsContainer = document.getElementById('summary-cards');
    const userStatsTableBody = document.querySelector('#user-stats-table tbody');
    const chartCanvas = document.getElementById('traffic-chart').getContext('2d');

    // --- Инициализация карты ---
    function initMap() {
        trafficMap = L.map('traffic-map').setView([20, 0], 2); // Центрируем карту
        L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
        }).addTo(trafficMap);
        mapMarkersLayer = L.layerGroup().addTo(trafficMap);
    }

    // Функция для форматирования байтов
    function formatBytes(bytes, decimals = 2) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const dm = decimals < 0 ? 0 : decimals;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
    }

    // Функция для обновления карточек
    function updateSummaryCards(stats) {
        summaryCardsContainer.innerHTML = `
            <div class="card">
                <h3>Активные соединения</h3>
                <div class="value">${stats.activeConnections || 0}</div>
            </div>
            <div class="card">
                <h3>Всего загружено (Upload)</h3>
                <div class="value">${formatBytes(stats.totalUploadBytes || 0)}</div>
            </div>
            <div class="card">
                <h3>Всего скачано (Download)</h3>
                <div class="value">${formatBytes(stats.totalDownloadBytes || 0)}</div>
            </div>
        `;
    }

    // Функция для обновления таблицы пользователей
    function updateUserStatsTable(userStats) {
        userStatsTableBody.innerHTML = ''; // Очищаем таблицу
        if (!userStats) {
            userStatsTableBody.innerHTML = '<tr><td colspan="3">Нет данных о пользователях.</td></tr>';
            return;
        }

        const sortedUsers = Object.keys(userStats).sort();

        for (const username of sortedUsers) {
            const stats = userStats[username];
            const row = document.createElement('tr');
            row.innerHTML = `
                <td>${username}</td>
                <td>${formatBytes(stats.uploadBytes)}</td>
                <td>${formatBytes(stats.downloadBytes)}</td>
            `;
            userStatsTableBody.appendChild(row);
        }
    }

    // Функция для создания/обновления графика
    function updateChart(userStats) {
        if (!userStats) return;

        const labels = Object.keys(userStats).sort();
        const uploadData = labels.map(u => userStats[u].uploadBytes);
        const downloadData = labels.map(u => userStats[u].downloadBytes);

        if (trafficChart) {
            // Обновляем данные существующего графика
            trafficChart.data.labels = labels;
            trafficChart.data.datasets[0].data = uploadData;
            trafficChart.data.datasets[1].data = downloadData;
            trafficChart.update();
        } else {
            // Создаем новый график
            trafficChart = new Chart(chartCanvas, {
                type: 'bar',
                data: {
                    labels: labels,
                    datasets: [
                        {
                            label: 'Загружено (Upload)',
                            data: uploadData,
                            backgroundColor: 'rgba(54, 162, 235, 0.6)',
                            borderColor: 'rgba(54, 162, 235, 1)',
                            borderWidth: 1
                        },
                        {
                            label: 'Скачано (Download)',
                            data: downloadData,
                            backgroundColor: 'rgba(255, 99, 132, 0.6)',
                            borderColor: 'rgba(255, 99, 132, 1)',
                            borderWidth: 1
                        }
                    ]
                },
                options: {
                    responsive: true,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Распределение трафика по пользователям'
                        },
                        tooltip: {
                            callbacks: {
                                label: function(context) {
                                    let label = context.dataset.label || '';
                                    if (label) {
                                        label += ': ';
                                    }
                                    if (context.parsed.y !== null) {
                                        label += formatBytes(context.parsed.y);
                                    }
                                    return label;
                                }
                            }
                        }
                    },
                    scales: {
                        y: {
                            beginAtZero: true,
                            ticks: {
                                callback: function(value) {
                                    return formatBytes(value);
                                }
                            }
                        }
                    }
                }
            });
        }
    }

    // Функция для получения цвета в зависимости от объема трафика
    function getTrafficColor(traffic, minTraffic, maxTraffic) {
        if (traffic === 0) return "#78c850"; // Зеленый для нулевого трафика

        // Нормализуем значение от 0 до 1
        const ratio = (Math.log(traffic + 1) - Math.log(minTraffic + 1)) / (Math.log(maxTraffic + 1) - Math.log(minTraffic + 1));

        // Простой градиент от зеленого к желтому и к красному
        if (ratio < 0.5) {
            // от зеленого (0, 255, 0) к желтому (255, 255, 0)
            const green = 255;
            const red = Math.round(255 * (ratio * 2));
            return `rgb(${red}, ${green}, 0)`;
        } else {
            // от желтого (255, 255, 0) к красному (255, 0, 0)
            const red = 255;
            const green = Math.round(255 * (1 - (ratio - 0.5) * 2));
            return `rgb(${red}, ${green}, 0)`;
        }
    }


    // Функция для обновления карты
    function updateMap(countryStats) {
        mapMarkersLayer.clearLayers(); // Очищаем старые маркеры

        if (!countryStats || Object.keys(countryStats).length === 0) return;

        // Находим минимальный и максимальный трафик для масштабирования
        let minTraffic = Infinity;
        let maxTraffic = -Infinity;
        for (const code in countryStats) {
            const traffic = countryStats[code].uploadBytes + countryStats[code].downloadBytes;
            if (traffic < minTraffic) minTraffic = traffic;
            if (traffic > maxTraffic) maxTraffic = traffic;
        }
        // Если только одна точка данных, установим мин = 0 для градиента
        if (minTraffic === maxTraffic) {
            minTraffic = 0;
        }


        for (const countryCode in countryStats) {
            const stats = countryStats[countryCode];
            const coords = countryCoordinates[countryCode];

            if (coords) {
                const totalTraffic = stats.uploadBytes + stats.downloadBytes;
                if (totalTraffic === 0) continue; // Не будем показывать страны без трафика

                // Рассчитываем радиус в зависимости от трафика (логарифмическая шкала)
                const radius = Math.log(totalTraffic + 1) * 1.5 + 3;

                // Получаем цвет на основе трафика
                const color = getTrafficColor(totalTraffic, minTraffic, maxTraffic);

                const marker = L.circleMarker([coords.lat, coords.lon], {
                    radius: radius,
                    fillColor: color,
                    color: color, // Используем тот же цвет для границы
                    weight: 2,
                    fillOpacity: 0.8
                });

                // Добавляем всплывающее окно
                marker.bindPopup(`
                    <b>Страна:</b> ${countryCode}<br>
                    <b>Соединений:</b> ${stats.connections}<br>
                    <b>Трафик:</b> ${formatBytes(totalTraffic)}<br>
                    <small>(Загружено: ${formatBytes(stats.uploadBytes)}, Скачано: ${formatBytes(stats.downloadBytes)})</small>
                `);

                mapMarkersLayer.addLayer(marker);
            }
        }
    }

    // Основная функция для получения и обновления данных
    async function fetchData() {
        try {
            const response = await fetch(API_URL);
            if (!response.ok) {
                throw new Error(`Ошибка сети: ${response.statusText}`);
            }
            const stats = await response.json();

            updateSummaryCards(stats);
            updateUserStatsTable(stats.userStats);
            updateChart(stats.userStats);
            updateMap(stats.countryStats); // Обновляем карту

        } catch (error) {
            console.error('Не удалось загрузить статистику:', error);
            summaryCardsContainer.innerHTML = `<p style="color: red; text-align: center; width: 100%;">Не удалось загрузить данные. Проверьте, запущен ли сервер и доступен ли файл статистики.</p>`;
        }
    }

    // Инициализация
    initMap();
    fetchData();
    setInterval(fetchData, 5000);
});