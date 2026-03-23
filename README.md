# Web Tunnel (mtunnel)

Tunneling aplikasi lokal kamu menjadi online seperti ngrok dan 1000% gratis dengan domain yang bisa kamu setup sendiri (domain.medandigital.dev).

## Cara Jalanin EXE (Windows)

1. Pastikan app lokal kamu sudah jalan dulu, misalnya di port `3000`.
2. Jalankan tunnel (dari folder tempat file EXE berada):

```powershell
.\medtunnel.exe 3000
```

Kalau mau request subdomain juga:

```powershell
.\medtunnel.exe 3000 namakamu
```

Kalau server pakai token auth, jalankan:

```powershell
.\medtunnel.exe 3000 namakamu TOKEN_RAHASIA
```

Atau via environment variable:

```powershell
$env:MTUNNEL_TOKEN="TOKEN_RAHASIA"
.\medtunnel.exe 3000 namakamu
```

## Arti Parameter

- Argumen pertama: port lokal (wajib), contoh `3000`
- Argumen kedua: subdomain (opsional), contoh `namakamu`
- Argumen ketiga: token (opsional), contoh `TOKEN_RAHASIA`

## Konfigurasi Server Tunnel

`server.js` mendukung environment variable berikut:

- `TUNNEL_DOMAIN` (default: `medandigital.dev`)
- `MTUNNEL_AUTH_TOKEN` (default: kosong / tanpa auth)
- `TUNNEL_REQUEST_TIMEOUT_MS` (default: `25000`)

Contoh run server dengan token:

```powershell
$env:TUNNEL_DOMAIN="medandigital.dev"
$env:MTUNNEL_AUTH_TOKEN="TOKEN_RAHASIA"
node server.js
```

## Kalau Berhasil

Nanti biasanya muncul log seperti ini:

```text
Connected to server
Tunnel Active!
Public URL: <url-dari-server>
```

Kalau URL public sudah keluar, tinggal akses URL itu dari browser.
