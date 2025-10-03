document.addEventListener('DOMContentLoaded', function () {
    const API_URL = '/api/stats';

    const totalUploadElem = document.getElementById('total-upload');
    const totalDownloadElem = document.getElementById('total-download');
    const activeConnectionsElem = document.getElementById('active-connections');
    const lastUpdatedElem = document.getElementById('last-updated');

    const userStatsTable = document.getElementById('user-stats-table').getElementsByTagName('tbody')[0];
    const countryStatsTable = document.getElementById('country-stats-table').getElementsByTagName('tbody')[0];
    const ipStatsTable = document.getElementById('ip-stats-table').getElementsByTagName('tbody')[0];

    let map = L.map('map').setView([20, 0], 2);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
    }).addTo(map);

    let markers = {};

    function formatBytes(bytes, decimals = 2) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const dm = decimals < 0 ? 0 : decimals;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
    }

    function updateDashboard(data) {
        if (!data) return;

        totalUploadElem.textContent = formatBytes(data.totalUploadBytes);
        totalDownloadElem.textContent = formatBytes(data.totalDownloadBytes);
        activeConnectionsElem.textContent = data.activeConnections || 0;
        lastUpdatedElem.textContent = new Date(data.lastUpdateTime).toLocaleString();

        updateTable(userStatsTable, data.userStats, (key, value) => `
            <tr>
                <td>${key}</td>
                <td>${formatBytes(value.uploadBytes)}</td>
                <td>${formatBytes(value.downloadBytes)}</td>
            </tr>
        `);

        updateTable(countryStatsTable, data.countryStats, (key, value) => `
            <tr>
                <td>${countryCoordinates[key] ? countryCoordinates[key].name : key}</td>
                <td>${formatBytes(value.uploadBytes)}</td>
                <td>${formatBytes(value.downloadBytes)}</td>
                <td>${value.connections}</td>
            </tr>
        `);

        updateTable(ipStatsTable, data.ipStats, (key, value) => `
            <tr>
                <td>${key}</td>
                <td>${formatBytes(value.uploadBytes)}</td>
                <td>${formatBytes(value.downloadBytes)}</td>
            </tr>
        `);

        updateMap(data.countryStats);
    }

    function updateTable(table, stats, rowTemplate) {
        if (!stats) {
            table.innerHTML = '<tr><td colspan="100%">No data available</td></tr>';
            return;
        }

        const sortedKeys = Object.keys(stats).sort((a, b) => {
            const aTotal = (stats[a].uploadBytes || 0) + (stats[a].downloadBytes || 0);
            const bTotal = (stats[b].uploadBytes || 0) + (stats[b].downloadBytes || 0);
            return bTotal - aTotal;
        });

        table.innerHTML = '';
        sortedKeys.forEach(key => {
            table.innerHTML += rowTemplate(key, stats[key]);
        });
    }

    function updateMap(countryStats) {
        if (!countryStats) return;

        for (const code in countryStats) {
            if (countryCoordinates[code]) {
                const { name, lat, lon } = countryCoordinates[code];
                const stats = countryStats[code];
                const totalTraffic = stats.uploadBytes + stats.downloadBytes;
                const radius = Math.log(totalTraffic + 1) * 2;
                const popupContent = `
                    <b>${name}</b><br>
                    Upload: ${formatBytes(stats.uploadBytes)}<br>
                    Download: ${formatBytes(stats.downloadBytes)}<br>
                    Connections: ${stats.connections}
                `;

                if (markers[code]) {
                    markers[code].setLatLng([lat, lon]).setRadius(radius).setPopupContent(popupContent);
                } else {
                    markers[code] = L.circle([lat, lon], {
                        radius: radius,
                        fillColor: "#f03",
                        color: "#f03",
                        weight: 1,
                        opacity: 1,
                        fillOpacity: 0.5
                    }).addTo(map).bindPopup(popupContent);
                }
            }
        }
    }

    function fetchData() {
        fetch(API_URL)
            .then(response => response.json())
            .then(data => updateDashboard(data))
            .catch(error => console.error('Error fetching stats:', error));
    }

    fetchData();
    setInterval(fetchData, 5000); // Refresh every 5 seconds
});