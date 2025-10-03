// Глобальные переменные
let trafficMap;
let trafficChart;
let apiTrafficChart;
let heatmapLayer;
const IP_GEO_API_URL = '/api/ipgeo';
const ipGeoCache = {};

// Инициализация карты
function initMap() {
    trafficMap = L.map('traffic-map').setView([30, 10], 2);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
    }).addTo(trafficMap);
    
    // Инициализация тепловой карты
    heatmapLayer = L.heatLayer([], {
        radius: 25,
        blur: 15,
        maxZoom: 10,
        gradient: {
            0.4: 'blue',
            0.6: 'cyan',
            0.7: 'lime',
            0.8: 'yellow',
            1.0: 'red'
        }
    }).addTo(trafficMap);
}

// Получение геолокации IP-адреса с кэшированием
async function getIPGeolocation(ip) {
    // Если IP уже в кэше, возвращаем сохраненные данные
    if (ipGeoCache[ip]) {
        return ipGeoCache[ip];
    }

    try {
        // Генерируем случайные координаты для локальных IP-адресов
        if (ip.startsWith('192.168.') || ip.startsWith('10.') || ip === '127.0.0.1') {
            const randomLat = (Math.random() * 180) - 90;
            const randomLng = (Math.random() * 360) - 180;
            const mockData = {
                latitude: randomLat,
                longitude: randomLng,
                country_name: 'Local Network',
                city: 'Local Area'
            };
            ipGeoCache[ip] = mockData;
            return mockData;
        }

        // Запрос к API для получения геолокации
        const response = await fetch(`${IP_GEO_API_URL}?ip=${ip}`);
        if (!response.ok) {
            throw new Error(`Ошибка API: ${response.status}`);
        }
        const data = await response.json();
        
        // Сохраняем в кэш
        ipGeoCache[ip] = data;
        return data;
    } catch (error) {
        console.error(`Ошибка при получении геолокации для ${ip}:`, error);
        
        // В случае ошибки генерируем случайные координаты
        const randomLat = (Math.random() * 180) - 90;
        const randomLng = (Math.random() * 360) - 180;
        const fallbackData = {
            latitude: randomLat,
            longitude: randomLng,
            country_name: 'Unknown',
            city: 'Unknown'
        };
        ipGeoCache[ip] = fallbackData;
        return fallbackData;
    }
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
    const summaryCards = document.getElementById('summary-cards');
    summaryCards.innerHTML = '';
    
    // Общее количество соединений
    const totalConnections = data.connections.length;
    const connectionsCard = document.createElement('div');
    connectionsCard.className = 'card';
    connectionsCard.innerHTML = `
        <h3>Активных соединений</h3>
        <p>${totalConnections}</p>
    `;
    summaryCards.appendChild(connectionsCard);
    
    // Общий объем загруженного трафика
    const totalUpload = data.connections.reduce((sum, conn) => sum + conn.upload, 0);
    const uploadCard = document.createElement('div');
    uploadCard.className = 'card';
    uploadCard.innerHTML = `
        <h3>Загружено (Upload)</h3>
        <p>${formatBytes(totalUpload)}</p>
    `;
    summaryCards.appendChild(uploadCard);
    
    // Общий объем скачанного трафика
    const totalDownload = data.connections.reduce((sum, conn) => sum + conn.download, 0);
    const downloadCard = document.createElement('div');
    downloadCard.className = 'card';
    downloadCard.innerHTML = `
        <h3>Скачано (Download)</h3>
        <p>${formatBytes(totalDownload)}</p>
    `;
    summaryCards.appendChild(downloadCard);
    
    // Уникальные IP-адреса
    const uniqueIPs = new Set(data.connections.map(conn => conn.dst_ip)).size;
    const uniqueIPsCard = document.createElement('div');
    uniqueIPsCard.className = 'card';
    uniqueIPsCard.innerHTML = `
        <h3>Уникальных IP-адресов</h3>
        <p>${uniqueIPs}</p>
    `;
    summaryCards.appendChild(uniqueIPsCard);
    
    // Количество API
    const uniqueAPIs = new Set(data.connections.map(conn => conn.api || 'unknown')).size;
    const uniqueAPIsCard = document.createElement('div');
    uniqueAPIsCard.className = 'card';
    uniqueAPIsCard.innerHTML = `
        <h3>Используемых API</h3>
        <p>${uniqueAPIs}</p>
    `;
    summaryCards.appendChild(uniqueAPIsCard);
}

// Обновление маркеров на карте и тепловой карты
async function updateMapMarkers(data) {
    // Очистка существующих маркеров
    trafficMap.eachLayer(layer => {
        if (layer instanceof L.Marker) {
            trafficMap.removeLayer(layer);
        }
    });
    
    // Получение уникальных IP-адресов
    const uniqueIPs = [...new Set(data.connections.map(conn => conn.dst_ip))];
    
    // Статистика по странам
    const countryStats = {};
    
    // Статистика по API
    const apiStats = {};
    
    // Статистика по IP-адресам
    const ipStats = {};
    
    // Данные для тепловой карты
    const heatmapData = [];
    
    // Для каждого уникального IP получаем геолокацию и добавляем маркер
    for (const ip of uniqueIPs) {
        // Пропускаем undefined или пустые IP
        if (!ip) continue;
        
        try {
            const geoData = await getIPGeolocation(ip);
            
            // Добавляем статистику по стране
            const country = geoData.country_name || 'Unknown';
            if (!countryStats[country]) {
                countryStats[country] = {
                    connections: 0,
                    upload: 0,
                    download: 0
                };
            }
            
            // Суммируем данные по соединениям для этого IP
            const ipConnections = data.connections.filter(conn => conn.dst_ip === ip);
            countryStats[country].connections += ipConnections.length;
            
            // Инициализируем статистику для этого IP
            ipStats[ip] = {
                upload: 0,
                download: 0,
                total: 0,
                connections: ipConnections.length,
                country: country,
                city: geoData.city || 'Unknown',
                latitude: geoData.latitude,
                longitude: geoData.longitude
            };
            
            for (const conn of ipConnections) {
                // Обновляем статистику по стране
                countryStats[country].upload += conn.upload;
                countryStats[country].download += conn.download;
                
                // Обновляем статистику по IP
                ipStats[ip].upload += conn.upload;
                ipStats[ip].download += conn.download;
                ipStats[ip].total += (conn.upload + conn.download);
                
                // Обновляем статистику по API
                const api = conn.api || 'unknown';
                if (!apiStats[api]) {
                    apiStats[api] = {
                        connections: 0,
                        upload: 0,
                        download: 0,
                        total: 0
                    };
                }
                apiStats[api].connections += 1;
                apiStats[api].upload += conn.upload;
                apiStats[api].download += conn.download;
                apiStats[api].total += (conn.upload + conn.download);
            }
            
            // Добавляем маркер на карту
            const marker = L.marker([geoData.latitude, geoData.longitude]).addTo(trafficMap);
            marker.bindPopup(`
                <b>IP:</b> ${ip}<br>
                <b>Страна:</b> ${geoData.country_name || 'Неизвестно'}<br>
                <b>Город:</b> ${geoData.city || 'Неизвестно'}<br>
                <b>Соединений:</b> ${ipConnections.length}<br>
                <b>Загружено:</b> ${formatBytes(ipStats[ip].upload)}<br>
                <b>Скачано:</b> ${formatBytes(ipStats[ip].download)}<br>
                <b>Всего трафика:</b> ${formatBytes(ipStats[ip].total)}
            `);
            
            // Добавляем точку для тепловой карты
            // Интенсивность зависит от объема трафика
            const trafficWeight = Math.log(ipStats[ip].total + 1) / 10; // Логарифмическая шкала для лучшей визуализации
            heatmapData.push([geoData.latitude, geoData.longitude, trafficWeight]);
        } catch (error) {
            console.error(`Ошибка при обработке IP ${ip}:`, error);
        }
    }
    
    // Обновляем тепловую карту
    heatmapLayer.setLatLngs(heatmapData);
    
    // Сохраняем статистику для использования в других функциях
    window.stats = {
        uniqueIPs: uniqueIPs.length,
        countryStats: countryStats,
        apiStats: apiStats,
        ipStats: ipStats
    };
    
    // Обновляем таблицу топ IP-адресов
    updateTopIPTable(ipStats);
    
    // Обновляем график API
    updateAPITrafficChart(apiStats);
}

// Обновление таблицы топ IP-адресов по трафику
function updateTopIPTable(ipStats) {
    const tableBody = document.querySelector('#top-ip-table tbody');
    tableBody.innerHTML = '';
    
    // Сортировка IP-адресов по общему трафику
    const sortedIPs = Object.keys(ipStats).sort((a, b) => {
        return ipStats[b].total - ipStats[a].total;
    });
    
    // Берем только топ-10 IP-адресов
    const topIPs = sortedIPs.slice(0, 10);
    
    // Заполнение таблицы
    for (const ip of topIPs) {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${ip} (${ipStats[ip].country})</td>
            <td>${formatBytes(ipStats[ip].upload)}</td>
            <td>${formatBytes(ipStats[ip].download)}</td>
            <td>${formatBytes(ipStats[ip].total)}</td>
        `;
        tableBody.appendChild(row);
    }
}

// Обновление графика распределения трафика по API
function updateAPITrafficChart(apiStats) {
    const ctx = document.getElementById('api-traffic-chart').getContext('2d');
    
    // Если график уже существует, уничтожаем его
    if (apiTrafficChart) {
        apiTrafficChart.destroy();
    }
    
    // Подготовка данных для графика
    const apis = Object.keys(apiStats);
    const uploadData = apis.map(api => apiStats[api].upload);
    const downloadData = apis.map(api => apiStats[api].download);
    
    // Создание графика
    apiTrafficChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: apis,
            datasets: [
                {
                    label: 'Upload (Bytes)',
                    data: uploadData,
                    backgroundColor: 'rgba(54, 162, 235, 0.5)',
                    borderColor: 'rgba(54, 162, 235, 1)',
                    borderWidth: 1
                },
                {
                    label: 'Download (Bytes)',
                    data: downloadData,
                    backgroundColor: 'rgba(255, 99, 132, 0.5)',
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
                    title: {
                        display: true,
                        text: 'Bytes'
                    }
                }
            },
            plugins: {
                title: {
                    display: true,
                    text: 'Распределение трафика по API'
                }
            }
        }
    });
}

// Обновление таблицы статистики по пользователям
function updateUserStatsTable(data) {
    const tableBody = document.querySelector('#user-stats-table tbody');
    tableBody.innerHTML = '';
    
    // Группировка данных по пользователям
    const userStats = {};
    
    for (const conn of data.connections) {
        const user = conn.user || 'anonymous';
        
        if (!userStats[user]) {
            userStats[user] = {
                upload: 0,
                download: 0
            };
        }
        
        userStats[user].upload += conn.upload;
        userStats[user].download += conn.download;
    }
    
    // Сортировка пользователей по общему трафику (upload + download)
    const sortedUsers = Object.keys(userStats).sort((a, b) => {
        const totalA = userStats[a].upload + userStats[a].download;
        const totalB = userStats[b].upload + userStats[b].download;
        return totalB - totalA;
    });
    
    // Заполнение таблицы
    for (const user of sortedUsers) {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${user}</td>
            <td>${formatBytes(userStats[user].upload)}</td>
            <td>${formatBytes(userStats[user].download)}</td>
        `;
        tableBody.appendChild(row);
    }
}

// Обновление графика трафика
function updateTrafficChart(data) {
    const ctx = document.getElementById('traffic-chart').getContext('2d');
    
    // Если график уже существует, уничтожаем его
    if (trafficChart) {
        trafficChart.destroy();
    }
    
    // Группировка данных по пользователям
    const userStats = {};
    
    for (const conn of data.connections) {
        const user = conn.user || 'anonymous';
        
        if (!userStats[user]) {
            userStats[user] = {
                upload: 0,
                download: 0
            };
        }
        
        userStats[user].upload += conn.upload;
        userStats[user].download += conn.download;
    }
    
    // Подготовка данных для графика
    const users = Object.keys(userStats);
    const uploadData = users.map(user => userStats[user].upload);
    const downloadData = users.map(user => userStats[user].download);
    
    // Создание графика
    trafficChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: users,
            datasets: [
                {
                    label: 'Upload (Bytes)',
                    data: uploadData,
                    backgroundColor: 'rgba(54, 162, 235, 0.5)',
                    borderColor: 'rgba(54, 162, 235, 1)',
                    borderWidth: 1
                },
                {
                    label: 'Download (Bytes)',
                    data: downloadData,
                    backgroundColor: 'rgba(255, 99, 132, 0.5)',
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
                    title: {
                        display: true,
                        text: 'Bytes'
                    }
                }
            },
            plugins: {
                title: {
                    display: true,
                    text: 'Трафик по пользователям'
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
            throw new Error(`Ошибка API: ${response.status}`);
        }
        
        const data = await response.json();
        
        // Добавляем случайные API для тестирования, если их нет
        if (data.connections && data.connections.length > 0 && !data.connections[0].api) {
            const apis = ['facebook', 'google', 'twitter', 'instagram', 'tiktok', 'youtube'];
            data.connections.forEach(conn => {
                conn.api = apis[Math.floor(Math.random() * apis.length)];
            });
        }
        
        // Обновление интерфейса
        updateSummaryCards(data);
        updateUserStatsTable(data);
        updateTrafficChart(data);
        await updateMapMarkers(data);
        
        // Повторный запрос через 10 секунд
        setTimeout(fetchData, 10000);
    } catch (error) {
        console.error('Ошибка при получении данных:', error);
        
        // В случае ошибки повторяем через 15 секунд
        setTimeout(fetchData, 15000);
    }
}

// Инициализация при загрузке страницы
document.addEventListener('DOMContentLoaded', () => {
    initMap();
    fetchData();
});