// Глобальные переменные
let trafficMap;
let userTrafficChart;
let heatmapLayer;

// Инициализация карты
function initMap() {
    trafficMap = L.map('traffic-map').setView([30, 10], 2);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
    }).addTo(trafficMap);

    heatmapLayer = L.heatLayer([], {
        radius: 30,
        blur: 20,
        maxZoom: 10,
        gradient: { 0.4: 'blue', 0.65: 'lime', 0.8: 'yellow', 1.0: 'red' }
    }).addTo(trafficMap);
}

// Форматирование байтов в читаемый формат
function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Обновление карточек со сводной статистикой
function updateSummaryCards(data) {
    document.getElementById('active-connections').textContent = data.activeConnections || 0;
    document.getElementById('total-upload').textContent = formatBytes(data.totalUploadBytes || 0);
    document.getElementById('total-download').textContent = formatBytes(data.totalDownloadBytes || 0);
    document.getElementById('last-update-time').textContent = new Date(data.lastUpdateTime).toLocaleString();
}

// Обновление карты на основе статистики по странам
function updateCountryMap(countryStats) {
    // Очистка существующих маркеров и тепловой карты
    trafficMap.eachLayer(layer => {
        if (layer instanceof L.Marker) {
            trafficMap.removeLayer(layer);
        }
    });
    const heatmapData = [];

    if (!countryStats) return;

    for (const countryCode in countryStats) {
        const stats = countryStats[countryCode];
        const coords = countryCoordinates[countryCode];

        if (coords) {
            const totalTraffic = stats.uploadBytes + stats.downloadBytes;
            const popupContent = `
                <b>Страна:</b> ${countryCode}<br>
                <b>Соединений:</b> ${stats.connections}<br>
                <b>Загружено:</b> ${formatBytes(stats.uploadBytes)}<br>
                <b>Скачано:</b> ${formatBytes(stats.downloadBytes)}
            `;

            L.marker([coords.lat, coords.lon]).addTo(trafficMap)
                .bindPopup(popupContent);
            
            // Интенсивность для тепловой карты (логарифмическая шкала)
            const intensity = Math.log(totalTraffic + 1) / Math.log(1024*1024*100); // Нормализуем
            heatmapData.push([coords.lat, coords.lon, intensity]);
        }
    }

    heatmapLayer.setLatLngs(heatmapData);
}


// Обновление таблицы и графика статистики по пользователям
function updateUserStats(userStats) {
    const tableBody = document.querySelector('#user-stats-table tbody');
    tableBody.innerHTML = '';
    
    if (!userStats) return;

    const sortedUsers = Object.keys(userStats).sort((a, b) => {
        const totalA = userStats[a].uploadBytes + userStats[a].downloadBytes;
        const totalB = userStats[b].uploadBytes + userStats[b].downloadBytes;
        return totalB - totalA;
    });

    const chartLabels = [];
    const uploadData = [];
    const downloadData = [];

    for (const user of sortedUsers) {
        const stats = userStats[user];
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${user}</td>
            <td>${formatBytes(stats.uploadBytes)}</td>
            <td>${formatBytes(stats.downloadBytes)}</td>
        `;
        tableBody.appendChild(row);

        chartLabels.push(user);
        uploadData.push(stats.uploadBytes);
        downloadData.push(stats.downloadBytes);
    }
    
    updateUserTrafficChart(chartLabels, uploadData, downloadData);
}

// Обновление графика трафика по пользователям
function updateUserTrafficChart(labels, uploadData, downloadData) {
    const ctx = document.getElementById('user-traffic-chart').getContext('2d');
    
    if (userTrafficChart) {
        userTrafficChart.destroy();
    }
    
    userTrafficChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [
                {
                    label: 'Upload',
                    data: uploadData,
                    backgroundColor: 'rgba(54, 162, 235, 0.7)',
                    borderColor: 'rgba(54, 162, 235, 1)',
                    borderWidth: 1
                },
                {
                    label: 'Download',
                    data: downloadData,
                    backgroundColor: 'rgba(255, 99, 132, 0.7)',
                    borderColor: 'rgba(255, 99, 132, 1)',
                    borderWidth: 1
                }
            ]
        },
        options: {
            responsive: true,
            scales: {
                y: {
                    beginAtZero: true,
                    ticks: {
                        callback: function(value) {
                            return formatBytes(value);
                        }
                    }
                }
            },
            plugins: {
                title: {
                    display: true,
                    text: 'Трафик по пользователям'
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
            }
        }
    });
}

// Получение данных с сервера
async function fetchData() {
    try {
        const response = await fetch('/api/stats');
        if (!response.ok) {
            // Если файл не найден, отображаем пустое состояние
            if (response.status === 404) {
                 console.warn('Файл статистики не найден. Ожидание данных от прокси-сервера...');
                 document.getElementById('error-message').textContent = 'Ожидание данных от прокси-сервера... Убедитесь, что основной сервер запущен и генерирует статистику.';
                 document.getElementById('error-message').style.display = 'block';
            } else {
                throw new Error(`Ошибка API: ${response.status}`);
            }
            // Повторный запрос через 5 секунд
            setTimeout(fetchData, 5000);
            return;
        }
        
        const data = await response.json();
        document.getElementById('error-message').style.display = 'none';

        // Обновление интерфейса
        updateSummaryCards(data);
        updateUserStats(data.userStats);
        updateCountryMap(data.countryStats);
        
    } catch (error) {
        console.error('Ошибка при получении или обработке данных:', error);
        document.getElementById('error-message').textContent = 'Не удалось загрузить или обработать статистику. Проверьте консоль для деталей.';
        document.getElementById('error-message').style.display = 'block';
    } finally {
         // Повторный запрос через 5 секунд
        setTimeout(fetchData, 5000);
    }
}

// Инициализация при загрузке страницы
document.addEventListener('DOMContentLoaded', () => {
    initMap();
    fetchData();
});