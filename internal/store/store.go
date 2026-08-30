package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const DemoUserID int64 = 1

type Store interface {
	Close() error
	EnsureBootstrap(ctx context.Context) error
	Dashboard(ctx context.Context, userID int64) (Dashboard, error)
	ListAccounts(ctx context.Context, userID int64) ([]Account, error)
	CreateAccount(ctx context.Context, input AccountInput) error
	ArchiveAccount(ctx context.Context, userID int64, id int64) error
	ListCategories(ctx context.Context, userID int64) ([]Category, error)
	CreateCategory(ctx context.Context, input CategoryInput) error
	DeleteCategory(ctx context.Context, userID int64, id int64) error
	ListTransactions(ctx context.Context, filter TransactionFilter) ([]Transaction, error)
	CreateTransaction(ctx context.Context, input TransactionInput) error
	UpdateTransaction(ctx context.Context, id int64, input TransactionInput) error
	DeleteTransaction(ctx context.Context, userID int64, id int64) error
}

type Dashboard struct {
	Accounts     []Account
	Transactions []Transaction
}

type Account struct {
	ID       int64
	Name     string
	Kind     string
	Currency string
	Balance  float64
}

type AccountInput struct {
	UserID         int64
	Name           string
	Kind           string
	Currency       string
	OpeningBalance float64
}

type Category struct {
	ID    int64
	Name  string
	Color string
	Icon  string
}

type CategoryInput struct {
	UserID int64
	Name   string
	Color  string
	Icon   string
}

type Transaction struct {
	ID                  int64
	AccountID           int64
	AccountName         string
	TransferAccountID   int64
	TransferAccountName string
	Date                time.Time
	Description         string
	Category            string
	Counterparty        string
	Amount              float64
	Kind                string
}

type TransactionInput struct {
	UserID            int64
	AccountID         int64
	TransferAccountID int64
	OccurredOn        time.Time
	Description       string
	Amount            float64
	Kind              string
	Category          string
	Counterparty      string
}

type TransactionFilter struct {
	UserID     int64
	From      time.Time
	To        time.Time
	AccountID int64
	Category  string
	Kind      string
	Query     string
	Limit     int
}

func DefaultTransactionFilter(userID int64) TransactionFilter {
	return TransactionFilter{UserID: userID, Limit: 80}
}

func ParseTransactionFilter(values map[string][]string) TransactionFilter {
	filter := DefaultTransactionFilter(DemoUserID)

	if parsed, err := time.Parse("2006-01-02", first(values, "from")); err == nil {
		filter.From = parsed
	}
	if parsed, err := time.Parse("2006-01-02", first(values, "to")); err == nil {
		filter.To = parsed
	}
	if parsed, err := strconv.ParseInt(first(values, "account_id"), 10, 64); err == nil && parsed > 0 {
		filter.AccountID = parsed
	}
	kind := first(values, "kind")
	if kind == "income" || kind == "expense" || kind == "transfer" {
		filter.Kind = kind
	}
	filter.Category = strings.TrimSpace(first(values, "category"))
	filter.Query = strings.TrimSpace(first(values, "q"))
	return filter
}

func OpenFromEnv(ctx context.Context) (Store, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		memory := NewMemoryStore()
		return memory, memory.EnsureBootstrap(ctx)
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	postgres := &PostgresStore{db: db}
	return postgres, postgres.EnsureBootstrap(ctx)
}

func ParseTransactionForm(values map[string][]string) (TransactionInput, error) {
	accountID, err := strconv.ParseInt(first(values, "account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		return TransactionInput{}, fmt.Errorf("seleziona un conto valido")
	}

	amount, err := strconv.ParseFloat(first(values, "amount"), 64)
	if err != nil || amount == 0 {
		return TransactionInput{}, fmt.Errorf("inserisci un importo diverso da zero")
	}

	occurredOn, err := time.Parse("2006-01-02", first(values, "occurred_on"))
	if err != nil {
		return TransactionInput{}, fmt.Errorf("inserisci una data valida")
	}

	kind := first(values, "kind")
	if kind != "income" && kind != "expense" && kind != "transfer" {
		return TransactionInput{}, fmt.Errorf("tipo movimento non valido")
	}

	transferAccountID := int64(0)
	if kind == "transfer" {
		parsed, err := strconv.ParseInt(first(values, "transfer_account_id"), 10, 64)
		if err != nil || parsed <= 0 {
			return TransactionInput{}, fmt.Errorf("seleziona il conto destinazione")
		}
		if parsed == accountID {
			return TransactionInput{}, fmt.Errorf("origine e destinazione devono essere conti diversi")
		}
		transferAccountID = parsed
	}

	if (kind == "expense" || kind == "transfer") && amount > 0 {
		amount = -amount
	}
	if kind == "income" && amount < 0 {
		amount = -amount
	}

	return TransactionInput{
		UserID:            DemoUserID,
		AccountID:         accountID,
		TransferAccountID: transferAccountID,
		OccurredOn:        occurredOn,
		Description:       strings.TrimSpace(first(values, "description")),
		Amount:            amount,
		Kind:              kind,
		Category:          strings.TrimSpace(first(values, "category")),
		Counterparty:      strings.TrimSpace(first(values, "counterparty")),
	}, nil
}

func ParseAccountForm(values map[string][]string) (AccountInput, error) {
	name := strings.TrimSpace(first(values, "name"))
	if name == "" {
		return AccountInput{}, fmt.Errorf("inserisci il nome del conto")
	}

	kind := first(values, "kind")
	switch kind {
	case "bank", "cash", "card", "savings":
	default:
		return AccountInput{}, fmt.Errorf("tipo conto non valido")
	}

	currency := strings.ToUpper(strings.TrimSpace(first(values, "currency")))
	if currency == "" {
		currency = "EUR"
	}
	if len(currency) != 3 {
		return AccountInput{}, fmt.Errorf("usa una valuta ISO a 3 lettere")
	}

	openingBalance := 0.0
	if raw := strings.TrimSpace(first(values, "opening_balance")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return AccountInput{}, fmt.Errorf("saldo iniziale non valido")
		}
		openingBalance = parsed
	}

	return AccountInput{
		UserID:         DemoUserID,
		Name:           name,
		Kind:           kind,
		Currency:       currency,
		OpeningBalance: openingBalance,
	}, nil
}

func ParseCategoryForm(values map[string][]string) (CategoryInput, error) {
	name := strings.TrimSpace(first(values, "name"))
	if name == "" {
		return CategoryInput{}, fmt.Errorf("inserisci il nome della categoria")
	}

	color := strings.TrimSpace(first(values, "color"))
	if color == "" {
		color = "#1f7a8c"
	}

	icon := strings.TrimSpace(first(values, "icon"))
	if icon == "" {
		icon = "circle"
	}

	return CategoryInput{
		UserID: DemoUserID,
		Name:   name,
		Color:  color,
		Icon:   icon,
	}, nil
}

func first(values map[string][]string, key string) string {
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func FormatMoney(amount float64) string {
	return fmt.Sprintf("%.2f EUR", amount)
}
