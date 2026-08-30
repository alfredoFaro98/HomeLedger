package store

import (
	"context"
	"fmt"
	"sync"
	"time"
	"strings"
)

type MemoryStore struct {
	mu           sync.Mutex
	nextID       int64
	accounts     []Account
	categories   []Category
	transactions []Transaction
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1}
}

func (s *MemoryStore) Close() error {
	return nil
}

func (s *MemoryStore) EnsureBootstrap(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.accounts) > 0 {
		return nil
	}

	s.accounts = []Account{
		{ID: 1, Name: "Conto principale", Kind: "bank", Currency: "EUR"},
		{ID: 2, Name: "Contanti", Kind: "cash", Currency: "EUR"},
	}
	s.categories = []Category{
		{ID: 3, Name: "Casa", Color: "#1f7a8c", Icon: "home"},
		{ID: 4, Name: "Spesa", Color: "#4f8a5b", Icon: "shopping-cart"},
		{ID: 5, Name: "Trasporti", Color: "#d9a441", Icon: "car"},
		{ID: 6, Name: "Stipendio", Color: "#2f855a", Icon: "briefcase"},
	}
	s.nextID = 7
	return nil
}

func (s *MemoryStore) Dashboard(ctx context.Context, userID int64) (Dashboard, error) {
	transactions, err := s.ListTransactions(ctx, DefaultTransactionFilter(userID))
	if err != nil {
		return Dashboard{}, err
	}
	accounts, err := s.ListAccounts(ctx, userID)
	if err != nil {
		return Dashboard{}, err
	}

	return Dashboard{Accounts: accounts, Transactions: transactions}, nil
}

func (s *MemoryStore) ListAccounts(ctx context.Context, userID int64) ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	accounts := make([]Account, len(s.accounts))
	copy(accounts, s.accounts)
	for i := range accounts {
		for _, tx := range s.transactions {
			if tx.AccountID == accounts[i].ID {
				accounts[i].Balance += tx.Amount
			}
			if tx.Kind == "transfer" && tx.TransferAccountID == accounts[i].ID {
				accounts[i].Balance += -tx.Amount
			}
		}
	}

	return accounts, nil
}

func (s *MemoryStore) CreateAccount(ctx context.Context, input AccountInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, account := range s.accounts {
		if account.Name == input.Name {
			return fmt.Errorf("esiste gia un conto con questo nome")
		}
	}

	s.accounts = append(s.accounts, Account{
		ID:       s.nextID,
		Name:     input.Name,
		Kind:     input.Kind,
		Currency: input.Currency,
		Balance:  input.OpeningBalance,
	})
	s.nextID++
	return nil
}

func (s *MemoryStore) ArchiveAccount(ctx context.Context, userID int64, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, account := range s.accounts {
		if account.ID == id {
			s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("conto non trovato")
}

func (s *MemoryStore) ListCategories(ctx context.Context, userID int64) ([]Category, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Category, len(s.categories))
	copy(out, s.categories)
	return out, nil
}

func (s *MemoryStore) CreateCategory(ctx context.Context, input CategoryInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, category := range s.categories {
		if category.Name == input.Name {
			return fmt.Errorf("esiste gia una categoria con questo nome")
		}
	}

	s.categories = append(s.categories, Category{
		ID:    s.nextID,
		Name:  input.Name,
		Color: input.Color,
		Icon:  input.Icon,
	})
	s.nextID++
	return nil
}

func (s *MemoryStore) DeleteCategory(ctx context.Context, userID int64, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, category := range s.categories {
		if category.ID == id {
			s.categories = append(s.categories[:i], s.categories[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("categoria non trovata")
}

func (s *MemoryStore) ListTransactions(ctx context.Context, filter TransactionFilter) ([]Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Transaction, 0, len(s.transactions))
	for _, tx := range s.transactions {
		if !matchesTransactionFilter(tx, filter) {
			continue
		}
		out = append(out, tx)
		if filter.Limit > 0 && len(out) >= filter.Limit {
			break
		}
	}
	return out, nil
}

func matchesTransactionFilter(tx Transaction, filter TransactionFilter) bool {
	if !filter.From.IsZero() && tx.Date.Before(filter.From) {
		return false
	}
	if !filter.To.IsZero() && tx.Date.After(filter.To) {
		return false
	}
	if filter.AccountID > 0 && tx.AccountID != filter.AccountID && tx.TransferAccountID != filter.AccountID {
		return false
	}
	if filter.Kind != "" && tx.Kind != filter.Kind {
		return false
	}
	if filter.Category != "" && !strings.EqualFold(tx.Category, filter.Category) {
		return false
	}
	if filter.Query != "" {
		haystack := strings.ToLower(tx.Description + " " + tx.Category + " " + tx.Counterparty + " " + tx.AccountName + " " + tx.TransferAccountName)
		if !strings.Contains(haystack, strings.ToLower(filter.Query)) {
			return false
		}
	}
	return true
}

func (s *MemoryStore) CreateTransaction(ctx context.Context, input TransactionInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var accountName string
	var transferAccountName string
	for _, account := range s.accounts {
		if account.ID == input.AccountID {
			accountName = account.Name
		}
		if account.ID == input.TransferAccountID {
			transferAccountName = account.Name
		}
	}
	if accountName == "" {
		return fmt.Errorf("conto non trovato")
	}
	if input.Kind == "transfer" && transferAccountName == "" {
		return fmt.Errorf("conto destinazione non trovato")
	}

	s.transactions = append([]Transaction{{
		ID:                  s.nextID,
		AccountID:           input.AccountID,
		AccountName:         accountName,
		TransferAccountID:   input.TransferAccountID,
		TransferAccountName: transferAccountName,
		Date:                input.OccurredOn,
		Description:         input.Description,
		Category:            input.Category,
		Counterparty:        input.Counterparty,
		Amount:              input.Amount,
		Kind:                input.Kind,
	}}, s.transactions...)
	s.nextID++
	return nil
}

func (s *MemoryStore) UpdateTransaction(ctx context.Context, id int64, input TransactionInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var accountName string
	var transferAccountName string
	for _, account := range s.accounts {
		if account.ID == input.AccountID {
			accountName = account.Name
		}
		if account.ID == input.TransferAccountID {
			transferAccountName = account.Name
		}
	}
	if accountName == "" {
		return fmt.Errorf("conto non trovato")
	}
	if input.Kind == "transfer" && transferAccountName == "" {
		return fmt.Errorf("conto destinazione non trovato")
	}

	for i := range s.transactions {
		if s.transactions[i].ID == id {
			s.transactions[i].AccountID = input.AccountID
			s.transactions[i].AccountName = accountName
			s.transactions[i].TransferAccountID = input.TransferAccountID
			s.transactions[i].TransferAccountName = transferAccountName
			s.transactions[i].Date = input.OccurredOn
			s.transactions[i].Description = input.Description
			s.transactions[i].Category = input.Category
			s.transactions[i].Counterparty = input.Counterparty
			s.transactions[i].Amount = input.Amount
			s.transactions[i].Kind = input.Kind
			return nil
		}
	}
	return fmt.Errorf("movimento non trovato")
}

func (s *MemoryStore) DeleteTransaction(ctx context.Context, userID int64, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, tx := range s.transactions {
		if tx.ID == id {
			s.transactions = append(s.transactions[:i], s.transactions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("movimento non trovato")
}

func Today() string {
	return time.Now().Format("2006-01-02")
}
