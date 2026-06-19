# Analisis Codebase & Rencana Refactor Bertahap

> Dokumen analisis untuk monorepo **Rive** (settlement layer pembayaran agent-to-agent, 0G APAC Hackathon Track 3).
> Disusun: 2026-06-19. Berbasis pembacaan kode penuh + `go vet ./...` (bersih) + pemetaan hotspot.
> **Catatan:** dokumen ini analisis & rencana — belum ada kode yang diubah.

---

## Ringkasan kondisi

Secara fundamental codebase ini **sehat dan di atas rata-rata proyek hackathon**: Clean Architecture ditegakkan konsisten di backend, `go vet ./...` bersih, 16 file test (termasuk fakes pgx + test SQL), disiplin `big.Int` / canonical-JSON (RFC 8785) / idempotency terjaga, dan kontrak punya test suite.

Jadi ini bukan "perbaiki yang rusak", melainkan **mengurangi utang teknis pada beberapa hotspot dan menutup celah robustness** sebelum skala bertambah.

---

## Temuan masalah (diurutkan per dampak)

### 🔴 Robustness / produksi

**M1 — Upload 0G Storage terjadi DI DALAM transaksi DB.**
`recordBookkeeping` memanggil `uploadBookkeepingJournal` (panggilan jaringan ke 0G) di dalam `withTx` — `backend/internal/repository/postgres/work_order_repository.go:742-787`. Satu upload yang lambat/hang menahan transaksi Postgres + koneksi pool terbuka selama upload berlangsung. Di bawah beban ini bisa menghabiskan connection pool. Ini juga menabrak prinsip di CLAUDE.md (orkestrasi infra mestinya di usecase, bukan terkubur dalam tx repository).

**M2 — Tidak ada graceful shutdown & resource leak.**
`backend/cmd/api/main.go` memakai `ListenAndServe()` tanpa signal handling, tanpa `ReadTimeout/WriteTimeout/IdleTimeout` (rawan Slowloris untuk API publik). `zgClient.Close()`, `nettingGateway.Close()`, dan pool DB **tidak pernah ditutup** (`app.go` mengembalikan `App` tanpa hook cleanup). Goroutine netting menerima `context.Background()` (`backend/internal/app/app.go:64`) sehingga **tidak bisa dibatalkan**. Baris `_ = application` (`main.go:16`) juga mati.

**M3 — Celah recovery pada netting settlement.**
Di `FlushPending` (`backend/internal/usecase/netting_usecase.go:132-181`) urutannya: claim intents di DB → upload manifest → `settleBatch` on-chain → `MarkBatchSettled`. Jika proses crash / RPC hang di tengah, intents tersangkut di batch "claimed" **tanpa loop rekonsiliasi saat startup**. `SettleBatch` juga `bind.WaitMined` (`backend/internal/infrastructure/settlement/netting.go:115`) yang bisa memblokir seluruh tick netting jika tx tak kunjung mined.

**M4 — I/O jaringan tanpa timeout di jalur boot.**
`NewNettingGateway` memanggil `client.NetworkID(context.Background())` tanpa timeout (`backend/internal/infrastructure/settlement/netting.go:64`); `backend/internal/infrastructure/storage/zerog.go:27` memakai `blockchain.MustNewWeb3` yang **panic** (tidak konsisten dengan gaya `Initialize()` yang return error) → RPC bermasalah bisa menjatuhkan/menggantung startup.

### 🟡 Maintainability

**M5 — `work_order_repository.go` (1054 baris) memikul terlalu banyak & duplikasi SQL masif.**
- Daftar 17 kolom `SELECT work_orders` ditulis-tangan **3×** (baris 69-90, 102-123, 136-159). Konsekuensinya sudah terlihat: `scanWorkOrder` (baris 958+) **tidak membaca** `release_tx_hash`/`refund_tx_hash` padahal kolom itu ada di migrasi (`backend/migrations/00005_work_orders.sql:19-20`) dan ditulis oleh `RecordOrderReleased`/`RecordOrderRefunded` → inkonsistensi laten.
- Enam metode `Record*`/`Rollback*` nyaris identik strukturnya (UPDATE…RETURNING + `recordBookkeeping` dengan posting double-entry yang saling cermin). Banyak copy-paste yang rawan menyimpang.
- Repository mencampur 3 tanggung jawab: persistence SQL, **logika akuntansi double-entry** (sebenarnya domain), dan orkestrasi upload 0G.

**M6 — Logging campur aduk** (`log.Printf` vs `logrus`), tanpa level/struktur. Error di loop netting hanya `log.Printf` (`backend/internal/usecase/netting_usecase.go:125`). Sudah diakui di CLAUDE.md.

**M7 — Paket placeholder/mati.**
`backend/internal/repository/redis/` hanya `doc.go` (tanpa implementasi); `backend/internal/delivery/http/dto/{,request,response}/` hanya `doc.go` — scaffolding kosong yang menyiratkan struktur yang belum ada.

**M8 — Catatan konfigurasi yang basi.** *(dikoreksi 2026-06-19)*
`backend/.env.example:21` sebenarnya **sudah** memakai `NETTING_WINDOW_SECONDS` (jamak, sesuai kode) — jadi tidak ada mismatch runtime. Masalah sebenarnya: catatan di `backend/CLAUDE.md` masih mengklaim `.env.example` mengirim ejaan tunggal dan menyarankan "set both", sehingga menyesatkan. Perbaikannya cukup menyelaraskan dokumentasi. **(Sudah dibereskan di Fase 1.)**

### 🟢 Frontend (web)

**M9 — Dead code signifikan di dashboard.**
Komponen `web/app/dashboard/_components/WorkOrders.tsx` dan `MOCK_WORK_ORDERS` **tidak pernah di-import/dirender** oleh `dashboard/page.tsx`. Objek mock besar `SAMPLE_RESPONSE`/`PROFITABLE_RESPONSE` (~85 baris di `data.ts`) hanya dipakai agar `DEMO_AGENTS` punya `.address` — `SearchSection` cuma membaca `address/label/status`, tidak pernah `.response`. `data.ts` juga mencampur formatter murni dengan fixture mock.

### Lintas-komponen

- **CI** hanya ada untuk `contracts/` (`contracts/.github/workflows/test.yml`); tidak ada CI untuk backend/web.

---

## Rencana refactor bertahap

Tiap fase dirancang **independen, bisa di-merge sendiri, dan menjaga test hijau**. Diurutkan agar yang berisiko rendah/bernilai tinggi lebih dulu.

### Fase 0 — Jaring pengaman (prasyarat, ~½ hari)
- Tambah workflow CI backend (`go build` / `go vet` / `go test -race`) dan web (`pnpm lint` / `pnpm build`) agar refactor berikutnya terlindungi.
- Tetapkan baseline: tidak ada perubahan perilaku tanpa test yang menjaganya.

### Fase 1 — Pembersihan berisiko-nol (~½ hari)
- Hapus dead code web: `WorkOrders.tsx`, `MOCK_WORK_ORDERS`, dan mock besar `SAMPLE_RESPONSE`/`PROFITABLE_RESPONSE` (rampingkan jadi daftar `DEMO_AGENTS` minimal: address/label/status). Pisahkan formatter murni dari fixture di `data.ts` → `format.ts`.
- Hapus paket placeholder `repository/redis` & `delivery/http/dto/*` (atau isi jika memang akan dipakai — **putuskan dulu**).
- Selaraskan `NETTING_WINDOW_SECOND(S)` di `.env.example` ↔ kode (M8). Bersihkan `_ = application` di main.go.

### Fase 2 — Robustness server & shutdown (~1 hari, M2/M4)
- `cmd/api/main.go`: tambahkan `Read/Write/IdleTimeout` pada `http.Server`, signal handling (`SIGINT/SIGTERM`) + `server.Shutdown(ctx)`.
- `app.Initialize` mengembalikan juga fungsi cleanup (atau `App.Close()`) yang menutup pool DB, `zgClient`, `nettingGateway`, dan membatalkan context loop netting (ganti `context.Background()` jadi context yang dibatalkan saat shutdown).
- Beri timeout pada `client.NetworkID` saat boot gateway; pertimbangkan ganti `MustNewWeb3` agar return error, bukan panic.

### Fase 3 — Keluarkan upload 0G dari transaksi DB (~1–2 hari, M1) — **fase paling bernilai**
- Pindahkan orkestrasi: upload manifest/journal ke 0G **sebelum** membuka tx, lalu transaksi DB hanya menulis baris + CID hasil upload. Repository jadi murni persistence; logika bookkeeping/double-entry naik ke usecase atau service domain khusus.
- Test fakes yang ada (`fakeNettingTx`, fake pgx) tetap dipakai; tambah test yang memastikan upload tidak terjadi di dalam tx.

### Fase 4 — Pecah `work_order_repository.go` & hapus duplikasi SQL (~2 hari, M5)
- Ekstrak satu konstanta/builder untuk daftar kolom `work_orders` (hilangkan duplikasi 3×), sekalian masukkan `release_tx_hash`/`refund_tx_hash` ke `scanWorkOrder` + domain (perbaiki inkonsistensi M5).
- Satukan enam `Record*`/`Rollback*` lewat satu helper berparameter (event kind + arah posting), karena posting forward/rollback adalah cermin. Pisahkan file: persistence vs bookkeeping.
- Karena test SQL mengandalkan string query, lakukan dengan hati-hati dan perbarui test berbarengan.

### Fase 5 — Rekonsiliasi netting & logging (~1–2 hari, M3/M6)
- Tambah loop/rutin startup yang mendeteksi batch "claimed/open" yang tertinggal dan menyelesaikan/menandai-gagal secara idempoten. Pertimbangkan memisahkan submit-tx dari wait-mined agar tick tidak terblokir.
- Standarkan logging ke satu logger berstruktur (pilih `logrus` yang sudah ada atau `slog`), termasuk error di loop netting.

---

## Matriks masalah → fase

| Kode | Masalah | Severity | Fase |
|------|---------|----------|------|
| M1 | Upload 0G di dalam tx DB | 🔴 | 3 |
| M2 | Tidak ada graceful shutdown & resource leak | 🔴 | 2 |
| M3 | Celah recovery netting settlement | 🔴 | 5 |
| M4 | I/O jaringan tanpa timeout saat boot | 🔴 | 2 |
| M5 | `work_order_repository.go` terlalu besar + duplikasi SQL | 🟡 | 4 |
| M6 | Logging campur aduk | 🟡 | 5 |
| M7 | Paket placeholder/mati (`redis`, `dto`) | 🟡 | 1 |
| M8 | Footgun config `NETTING_WINDOW_SECOND(S)` | 🟡 | 1 |
| M9 | Dead code dashboard web | 🟢 | 1 |
| — | CI hanya untuk contracts | 🟡 | 0 |

**Saran urutan:** mulai **Fase 1** (cepat, nol risiko, langsung mengurangi noise) lalu **Fase 3** (dampak robustness terbesar).

## Keputusan yang menunggu masukan

1. Paket `redis`/`dto` — dihapus atau memang akan diisi?
2. Urutan prioritas fase mengikuti timeline hackathon.
