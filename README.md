# Web Tunnel (mtunnel)

Tunneling aplikasi lokal kamu menjadi online seperti ngrok dan 1000% gratis dengan domain yang bisa kamu setup sendiri (domain.medandigital.dev).

## Cara Jalanin EXE (Windows)

1. Pastikan aplikasi lokal kamu sudah jalan dulu.
2. Jalankan `medtunnel.exe` dari folder file EXE.

Contoh paling sederhana (tanpa subdomain):

```powershell
.\medtunnel.exe 3000
```

Contoh dengan subdomain:

```powershell
.\medtunnel.exe 3000 namakamu
```

Contoh dengan token:

```powershell
.\medtunnel.exe 3000 namakamu TOKEN_RAHASIA
```

Contoh dengan token via environment variable:

```powershell
$env:MTUNNEL_TOKEN="TOKEN_RAHASIA"
.\medtunnel.exe 3000 namakamu
```

Contoh untuk aplikasi lokal di Apache/Laragon port 80:

```powershell
.\medtunnel.exe 80 familyhill
```

## Arti Parameter

- Argumen pertama: port lokal (wajib), contoh `3000`
- Argumen kedua: subdomain (opsional), contoh `namakamu`
- Argumen ketiga: token (opsional), contoh `TOKEN_RAHASIA`
- Argumen keempat: upstream host lokal (opsional), default `localhost`

## Opsi Environment Variable (Client EXE)

- `MTUNNEL_SERVER`: URL websocket server tunnel (default `ws://163.223.104.100:3000`)
- `MTUNNEL_TOKEN`: token auth (opsional)
- `MTUNNEL_UPSTREAM_HOST`: host upstream lokal (default `localhost`)
- `MTUNNEL_UPSTREAM_HOST_HEADER`: paksa header Host ke nilai tertentu
- `MTUNNEL_READ_TIMEOUT_SEC`: timeout baca koneksi websocket (default `300`)
- `MTUNNEL_RECONNECT_BASE_SEC`: jeda reconnect awal (default `2`)
- `MTUNNEL_RECONNECT_MAX_SEC`: jeda reconnect maksimum (default `30`)
- `MTUNNEL_DEBUG`: set `1` untuk log debug request proxy

Contoh untuk virtual host lokal:

```powershell
$env:MTUNNEL_UPSTREAM_HOST="127.0.0.1"
$env:MTUNNEL_UPSTREAM_HOST_HEADER="familyhill.medandigital.dev"
.\medtunnel.exe 80 familyhill TOKEN_RAHASIA
```

## Kalau Berhasil

Nanti biasanya muncul log seperti ini:

```text
Connected
🚀 URL: https://subdomain.medandigital.dev
```

Kalau URL public sudah keluar, tunnel sudah aktif dan bisa langsung diakses dari browser.

## Mode TCP (Database, Redis, dsb)

Untuk tunneling port non-HTTP (MySQL/MariaDB `3306`, PostgreSQL `5432`, MongoDB `27017`, Redis `6379`, RDP, dsb) pakai mode `tcp`:

```powershell
.\mtunnel.exe tcp 3306
```

Output:

```text
Connected (mtunnel v0.6 tcp)
🚀 TCP URL: tcp://medandigital.dev:20000
   Forwarding to localhost:3306
```

Lalu koneksi dari mana saja ke `medandigital.dev:20000` akan diteruskan ke `localhost:3306`. Contoh MySQL:

```powershell
mysql -h medandigital.dev -P 20000 -u root -p
```

### Parameter mode TCP

```powershell
.\mtunnel.exe tcp <local-port> [remote-port] [token] [upstream-host]
```

- Argumen 1 (`tcp`): mode TCP
- Argumen 2: port lokal (wajib), contoh `3306`
- Argumen 3: remote port yang diminta (opsional, `0`/kosong = auto-assign)
- Argumen 4: token (opsional)
- Argumen 5: upstream host lokal (opsional, default `localhost`)

### Catatan mode TCP

- Remote port di-assign otomatis dari range `20000–20010` (diatur via env server `TUNNEL_TCP_PORT_MIN`/`TUNNEL_TCP_PORT_MAX`). Minta port spesifik lewat argumen ke-3 atau env `MTUNNEL_REMOTE_PORT`.
- Tunnel TCP **tidak lewat nginx/subdomain** — langsung ke port VPS, jadi aksesnya pakai `medandigital.dev:<remote-port>` atau `163.223.104.100:<remote-port>`.
- Bisa jalan bareng tunnel HTTP — jalankan instance `mtunnel.exe` kedua untuk port DB-nya.
- Koneksi TCP bersifat long-lived (tidak ada timeout request seperti HTTP).
- Tidak ada enkripsi end-to-end di layer tunnel — untuk DB produksi, aktifkan TLS di sisi database-nya sendiri.
