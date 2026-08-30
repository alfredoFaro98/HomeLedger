# HomeLedger

Gestionale web per spese personali, pensato per self-hosting su server Linux domestico.

## Stack

- Backend Go con rendering server-side.
- Frontend HTML, HTMX, Alpine.js e CSS custom.
- Icone locali da IcoMoon Ultimate.
- PostgreSQL come database primario.
- Docker Compose per sviluppo e deploy iniziale.

## Avvio locale

```bash
go run ./cmd/homeledger
```

L'app risponde su `http://localhost:8080` quando avviata con `go run`.
Senza `DATABASE_URL` usa uno store in memoria, utile solo per provare l'interfaccia.

## Avvio con Docker Compose

```bash
docker compose up --build
```

Con Docker Compose l'app risponde su `http://localhost:18090`, cosi non collide con IIS/HTTP.sys su Windows.

PostgreSQL inizializza lo schema da `migrations/0001_init.sql` al primo avvio del volume.

## Protezione base

Per abilitare una password davanti all'app:

```bash
HOMELEDGER_USERNAME=admin HOMELEDGER_PASSWORD=una-password-forte docker compose up --build
```

Puoi anche partire da `.env.example` e creare un tuo `.env` locale.

## CSV

Da `Movimenti` puoi esportare e importare CSV. Le colonne supportate sono:

```text
date,kind,account,transfer_account,amount,category,counterparty,description
```

`kind` puo essere `income`, `expense` o `transfer`.

## Backup

Sul server Linux:

```bash
sh scripts/backup.sh
sh scripts/restore.sh backups/homeledger-YYYYMMDD-HHMMSS.sql
```
