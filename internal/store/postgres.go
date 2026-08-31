package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type PostgresStore struct {
	db *sql.DB
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) EnsureBootstrap(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name)
		VALUES (1, 'local@homeledger', 'bootstrap-disabled', 'Locale')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO accounts (id, user_id, name, kind, currency)
		VALUES
			(1, 1, 'Conto principale', 'bank', 'EUR'),
			(2, 1, 'Contanti', 'cash', 'EUR')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO categories (id, user_id, name, color, icon)
		VALUES
			(1, 1, 'Casa', '#1f7a8c', 'home'),
			(2, 1, 'Spesa', '#4f8a5b', 'shopping-cart'),
			(3, 1, 'Trasporti', '#d9a441', 'car'),
			(4, 1, 'Stipendio', '#2f855a', 'briefcase')
		ON CONFLICT (id) DO NOTHING;

		SELECT setval(pg_get_serial_sequence('users', 'id'), COALESCE((SELECT MAX(id) FROM users), 1), true);
		SELECT setval(pg_get_serial_sequence('accounts', 'id'), COALESCE((SELECT MAX(id) FROM accounts), 1), true);
		SELECT setval(pg_get_serial_sequence('categories', 'id'), COALESCE((SELECT MAX(id) FROM categories), 1), true);
	`)
	if err != nil {
		return fmt.Errorf("bootstrap postgres: %w", err)
	}
	return nil
}

func (s *PostgresStore) Dashboard(ctx context.Context, userID int64) (Dashboard, error) {
	accounts, err := s.ListAccounts(ctx, userID)
	if err != nil {
		return Dashboard{}, err
	}
	transactions, err := s.ListTransactions(ctx, DefaultTransactionFilter(userID))
	if err != nil {
		return Dashboard{}, err
	}
	return Dashboard{Accounts: accounts, Transactions: transactions}, nil
}

func (s *PostgresStore) ListAccounts(ctx context.Context, userID int64) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			a.id,
			a.name,
			a.kind,
			a.currency,
			a.opening_balance
				+ COALESCE(SUM(CASE WHEN t.account_id = a.id THEN t.amount ELSE 0 END), 0)
				+ COALESCE(SUM(CASE WHEN t.transfer_account_id = a.id THEN -t.amount ELSE 0 END), 0) AS balance
		FROM accounts a
		LEFT JOIN transactions t ON t.account_id = a.id OR t.transfer_account_id = a.id
		WHERE a.user_id = $1 AND a.archived_at IS NULL
		GROUP BY a.id
		ORDER BY a.name;
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.ID, &account.Name, &account.Kind, &account.Currency, &account.Balance); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *PostgresStore) CreateAccount(ctx context.Context, input AccountInput) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO accounts (user_id, name, kind, currency, opening_balance)
		VALUES ($1, $2, $3, $4, $5);
	`, input.UserID, input.Name, input.Kind, input.Currency, input.OpeningBalance)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func (s *PostgresStore) ArchiveAccount(ctx context.Context, userID int64, id int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE accounts
		SET archived_at = now(), updated_at = now()
		WHERE user_id = $1 AND id = $2 AND archived_at IS NULL;
	`, userID, id)
	if err != nil {
		return fmt.Errorf("archive account: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("archive account rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("conto non trovato")
	}
	return nil
}

func (s *PostgresStore) ListCategories(ctx context.Context, userID int64) ([]Category, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(color, ''), COALESCE(icon, '')
		FROM categories
		WHERE user_id = $1
		ORDER BY name;
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.ID, &category.Name, &category.Color, &category.Icon); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (s *PostgresStore) CreateCategory(ctx context.Context, input CategoryInput) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO categories (user_id, name, color, icon)
		VALUES ($1, $2, $3, $4);
	`, input.UserID, input.Name, input.Color, input.Icon)
	if err != nil {
		return fmt.Errorf("create category: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteCategory(ctx context.Context, userID int64, id int64) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM categories
		WHERE user_id = $1 AND id = $2;
	`, userID, id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete category rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("categoria non trovata")
	}
	return nil
}

func (s *PostgresStore) ListTransactions(ctx context.Context, filter TransactionFilter) ([]Transaction, error) {
	query := `
		SELECT
			t.id,
			COALESCE(t.account_id, 0),
			COALESCE(a.name, ''),
			COALESCE(ta.id, 0),
			COALESCE(ta.name, ''),
			t.occurred_on,
			t.description,
			COALESCE(c.name, ''),
			COALESCE(cp.name, ''),
			t.amount,
			t.kind
		FROM transactions t
		LEFT JOIN accounts a ON a.id = t.account_id
		LEFT JOIN accounts ta ON ta.id = t.transfer_account_id
		LEFT JOIN categories c ON c.id = t.category_id
		LEFT JOIN counterparties cp ON cp.id = t.counterparty_id
		WHERE t.user_id = $1`
	args := []any{filter.UserID}

	if !filter.From.IsZero() {
		args = append(args, filter.From)
		query += fmt.Sprintf(" AND t.occurred_on >= $%d", len(args))
	}
	if !filter.To.IsZero() {
		args = append(args, filter.To)
		query += fmt.Sprintf(" AND t.occurred_on <= $%d", len(args))
	}
	if filter.AccountID > 0 {
		args = append(args, filter.AccountID)
		query += fmt.Sprintf(" AND (t.account_id = $%d OR t.transfer_account_id = $%d)", len(args), len(args))
	}
	if filter.Kind != "" {
		args = append(args, filter.Kind)
		query += fmt.Sprintf(" AND t.kind = $%d", len(args))
	}
	if filter.Category != "" {
		args = append(args, filter.Category)
		query += fmt.Sprintf(" AND LOWER(COALESCE(c.name, '')) = LOWER($%d)", len(args))
	}
	if filter.Query != "" {
		args = append(args, "%"+strings.ToLower(filter.Query)+"%")
		query += fmt.Sprintf(" AND (LOWER(t.description) LIKE $%d OR LOWER(COALESCE(c.name, '')) LIKE $%d OR LOWER(COALESCE(cp.name, '')) LIKE $%d OR LOWER(a.name) LIKE $%d OR LOWER(COALESCE(ta.name, '')) LIKE $%d)", len(args), len(args), len(args), len(args), len(args))
	}

	limit := filter.Limit
	if limit == 0 {
		limit = 80
	}
	query += " ORDER BY t.occurred_on DESC, t.id DESC"
	if limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var tx Transaction
		if err := rows.Scan(&tx.ID, &tx.AccountID, &tx.AccountName, &tx.TransferAccountID, &tx.TransferAccountName, &tx.Date, &tx.Description, &tx.Category, &tx.Counterparty, &tx.Amount, &tx.Kind); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		transactions = append(transactions, tx)
	}
	return transactions, rows.Err()
}

func (s *PostgresStore) CreateTransaction(ctx context.Context, input TransactionInput) error {
	categoryID, err := s.upsertCategory(ctx, input.UserID, input.Category)
	if err != nil {
		return err
	}
	counterpartyID, err := s.upsertCounterparty(ctx, input.UserID, input.Counterparty)
	if err != nil {
		return err
	}
	accountID := nullableInt64(input.AccountID)
	transferAccountID := nullableInt64(input.TransferAccountID)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO transactions (
			user_id, account_id, transfer_account_id, category_id, counterparty_id,
			occurred_on, description, amount, currency, kind
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'EUR', $9);
	`, input.UserID, accountID, transferAccountID, categoryID, counterpartyID, input.OccurredOn, input.Description, input.Amount, input.Kind)
	if err != nil {
		return fmt.Errorf("create transaction: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateTransaction(ctx context.Context, id int64, input TransactionInput) error {
	categoryID, err := s.upsertCategory(ctx, input.UserID, input.Category)
	if err != nil {
		return err
	}
	counterpartyID, err := s.upsertCounterparty(ctx, input.UserID, input.Counterparty)
	if err != nil {
		return err
	}
	accountID := nullableInt64(input.AccountID)
	transferAccountID := nullableInt64(input.TransferAccountID)

	result, err := s.db.ExecContext(ctx, `
		UPDATE transactions
		SET account_id = $1,
			transfer_account_id = $2,
			category_id = $3,
			counterparty_id = $4,
			occurred_on = $5,
			description = $6,
			amount = $7,
			kind = $8,
			updated_at = now()
		WHERE user_id = $9 AND id = $10;
	`, accountID, transferAccountID, categoryID, counterpartyID, input.OccurredOn, input.Description, input.Amount, input.Kind, input.UserID, id)
	if err != nil {
		return fmt.Errorf("update transaction: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update transaction rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("movimento non trovato")
	}
	return nil
}

func (s *PostgresStore) DeleteTransaction(ctx context.Context, userID int64, id int64) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM transactions
		WHERE user_id = $1 AND id = $2;
	`, userID, id)
	if err != nil {
		return fmt.Errorf("delete transaction: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete transaction rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("movimento non trovato")
	}
	return nil
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func (s *PostgresStore) upsertCategory(ctx context.Context, userID int64, name string) (*int64, error) {
	if name == "" {
		return nil, nil
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO categories (user_id, name)
		VALUES ($1, $2)
		ON CONFLICT (user_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id;
	`, userID, name).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("upsert category: %w", err)
	}
	return &id, nil
}

func (s *PostgresStore) upsertCounterparty(ctx context.Context, userID int64, name string) (*int64, error) {
	if name == "" {
		return nil, nil
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO counterparties (user_id, name)
		VALUES ($1, $2)
		ON CONFLICT (user_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id;
	`, userID, name).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("upsert counterparty: %w", err)
	}
	return &id, nil
}
