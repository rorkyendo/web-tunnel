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

## Arti Parameter

- Argumen pertama: port lokal (wajib), contoh `3000`
- Argumen kedua: subdomain (opsional), contoh `namakamu`

## Kalau Berhasil

Nanti biasanya muncul log seperti ini:

```text
Connected to server
Tunnel Active!
Public URL: <url-dari-server>
```

Kalau URL public sudah keluar, tinggal akses URL itu dari browser.
