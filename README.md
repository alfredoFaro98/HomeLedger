# HomeLedger

Gestionale web per spese personali, pensato per il self-hosting su un server Linux domestico. Un binario Go singolo, un database Postgres, nessuna dipendenza da servizi esterni.

## Cosa fa

- **Dashboard**: saldo totale, entrate/uscite del mese corrente, tasso di risparmio, elenco conti e ultimi movimenti.
- **Movimenti**: entrate, uscite e trasferimenti tra conti, con filtri per data, conto, categoria, tipo e ricerca testuale.
- **Conti**: banca, contanti, carta o risparmio, in EUR/USD/GBP, con archiviazione (non cancellazione distruttiva).
- **Categorie**: nome, colore e icona personalizzabili, usate per raggruppare i movimenti.
- **Report**: riepilogo mensile delle uscite per categoria, con percentuale sul totale.
- **Impostazioni**: tema chiaro/scuro/sistema e accent color (petrolio, verde, blu, indigo), salvati lato client.
- **Import / Export CSV**: per portare dati dentro o fuori dall'app senza vincoli.
- **Autenticazione HTTP Basic** opzionale, da attivare via variabili d'ambiente.

L'interfaccia è renderizzata lato server (Go `html/template`), con HTMX per gli aggiornamenti parziali e Alpine.js per lo stato lato client. Nessun framework JS pesante, nessuna build frontend.

## Stack

- Backend: Go, rendering server-side, routing con `net/http` (Go 1.22+ mux).
- Frontend: HTML, [HTMX](https://htmx.org), [Alpine.js](https://alpinejs.dev), CSS custom con variabili per tema/accent.
- Icone locali da IcoMoon Ultimate (nessuna dipendenza da CDN).
- Database: PostgreSQL.
- Deploy: Docker / Docker Compose.

## Avvio locale

```bash
go run ./cmd/homeledger
```

L'app risponde su `http://localhost:8080`. Senza `DATABASE_URL` impostata usa uno store in memoria: comodo per provare l'interfaccia, ma i dati non sopravvivono al riavvio.

## Avvio con Docker Compose

```bash
docker compose up --build
```

Con Docker Compose l'app risponde su `http://localhost:18090` (per non collidere con IIS/HTTP.sys su Windows) e i dati persistono su un volume Postgres. Lo schema si inizializza da [migrations/0001_init.sql](migrations/0001_init.sql) al primo avvio del volume.

## Configurazione

Variabili d'ambiente supportate:

| Variabile | Descrizione | Default |
|---|---|---|
| `HTTP_ADDR` | Indirizzo e porta di ascolto | `:8080` |
| `DATABASE_URL` | Connection string Postgres | store in memoria se assente |
| `HOMELEDGER_USERNAME` | Utente per l'HTTP Basic Auth | disattivata se assente |
| `HOMELEDGER_PASSWORD` | Password per l'HTTP Basic Auth | disattivata se assente |

L'autenticazione si attiva solo se **entrambe** `HOMELEDGER_USERNAME` e `HOMELEDGER_PASSWORD` sono impostate; l'endpoint `/healthz` resta sempre accessibile senza credenziali, per i probe di orchestratori/reverse proxy.

Puoi partire da [.env.example](.env.example) e creare un tuo `.env` locale:

```bash
HOMELEDGER_USERNAME=admin HOMELEDGER_PASSWORD=una-password-forte docker compose up --build
```

## Import / Export CSV

Dalla pagina **Movimenti** puoi esportare ed importare CSV. Colonne supportate:

```text
date,kind,account,transfer_account,amount,category,counterparty,description
```

- `kind` può essere `income`, `expense` o `transfer`.
- `transfer_account` è obbligatorio solo per i trasferimenti e deve corrispondere al nome di un conto esistente.
- L'importo va sempre positivo in export; in import il segno viene normalizzato automaticamente in base a `kind`.

## Backup e restore

Sul server Linux, con lo stack Docker Compose attivo:

```bash
sh scripts/backup.sh
sh scripts/restore.sh backups/homeledger-YYYYMMDD-HHMMSS.sql
```

`backup.sh` esegue un `pg_dump` del database `homeledger` e lo salva in `./backups/` (override con `BACKUP_DIR`); `restore.sh` applica un dump con `psql`.

## Roadmap

- [ ] Screenshot di dashboard, movimenti e report nel README.
- [ ] Grafici di andamento spese nella pagina Report.
- [ ] Multi-utente reale (oggi tutti i dati sono legati a un unico utente demo).
