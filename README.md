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
