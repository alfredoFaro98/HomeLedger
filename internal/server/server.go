package server

import (
	"context"
	"crypto/subtle"
	"encoding/csv"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"homeledger.local/app/internal/store"
	"homeledger.local/app/internal/web"
)

type Server struct {
	templates *template.Template
	static    http.Handler
	store     store.Store
}

type NavItem struct {
	Label  string
	Href   string
	Icon   string
	Active bool
}

type PageData struct {
	Title       string
	CurrentYear int
	Nav         []NavItem
	Metrics     []Metric
	Accounts    []AccountSummary
	Categories  []CategoryView
	Recent      []Transaction
	Reports     []ReportRow
	Form        TransactionForm
	AccountForm AccountForm
	CategoryForm CategoryForm
	Filters      FilterView
	Error       string
	Notice      string
}

type Metric struct {
	Label string
	Value string
	Tone  string
}

type AccountSummary struct {
	ID      int64
	Name    string
	Type    string
	Balance string
}

type Transaction struct {
	ID                int64
	AccountID         int64
	TransferAccountID int64
	TransferLabel     string
	Date              string
	DateInput         string
	Payee             string
	Category          string
	Counterparty      string
	Description       string
	Amount            string
	AmountInput       string
	Kind              string
	IsIncome          bool
	IsExpense         bool
	IsTransfer        bool
	Tone              string
	EditAccounts      []AccountSummary
	EditCategories    []CategoryView
}

type TransactionForm struct {
	Today      string
	Accounts   []AccountSummary
	Categories []CategoryView
}

type AccountForm struct {
	Kinds      []Option
	Currencies []Option
}

type Option struct {
	Value string
	Label string
}

type CategoryView struct {
	ID          int64
	Name        string
	Color       string
	Icon        string
	SwatchStyle template.CSS
}

type CategoryForm struct {
	Icons []Option
}

type FilterView struct {
	From      string
	To        string
	AccountID int64
	Category  string
	Kind      string
	Query     string
}

type ReportRow struct {
	Label      string
	Amount     string
	Percentage string
	Tone       string
}

func New() http.Handler {
	appStore, err := store.OpenFromEnv(context.Background())
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	staticFS, err := fs.Sub(web.Files, "static")
	if err != nil {
		log.Fatalf("static filesystem: %v", err)
	}

	s := &Server{
		templates: template.Must(template.New("").Funcs(template.FuncMap{
			"selected":     selected,
			"isCustomIcon": func(icon string) bool { return strings.HasPrefix(icon, "data:image/") },
			"iconSrc": func(icon string) template.URL {
				if strings.HasPrefix(icon, "data:image/") {
					return template.URL(icon)
				}
				return ""
			},
		}).ParseFS(web.Files, "templates/*.html")),
		static:    http.FileServer(http.FS(staticFS)),
		store:     appStore,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /", s.dashboard)
	mux.HandleFunc("GET /transactions", s.transactions)
	mux.HandleFunc("POST /transactions", s.createTransaction)
	mux.HandleFunc("PUT /transactions/{id}", s.updateTransaction)
	mux.HandleFunc("DELETE /transactions/{id}", s.deleteTransaction)
	mux.HandleFunc("GET /transactions/export.csv", s.exportTransactions)
	mux.HandleFunc("POST /transactions/import.csv", s.importTransactions)
	mux.HandleFunc("GET /accounts", s.accounts)
	mux.HandleFunc("POST /accounts", s.createAccount)
	mux.HandleFunc("DELETE /accounts/{id}", s.archiveAccount)
	mux.HandleFunc("GET /categories", s.categories)
	mux.HandleFunc("POST /categories", s.createCategory)
	mux.HandleFunc("DELETE /categories/{id}", s.deleteCategory)
	mux.HandleFunc("GET /reports", s.reports)
	mux.HandleFunc("GET /settings", s.settings)
	mux.HandleFunc("GET /partials/recent-transactions", s.recentTransactions)

	return securityHeaders(basicAuth(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	summary, err := s.store.Dashboard(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("dashboard store: %v", err)
		return
	}

	data := PageData{
		Title:       "Dashboard",
		CurrentYear: time.Now().Year(),
		Nav:         navFor("/"),
		Metrics:     metricsFor(summary),
		Accounts:    accountsFor(summary.Accounts),
		Recent:      transactionsFor(summary.Transactions, 8, nil, nil),
	}

	s.render(w, "dashboard", data)
}

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.store.ListAccounts(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("accounts store: %v", err)
		return
	}

	data := PageData{
		Title:       "Conti",
		CurrentYear: time.Now().Year(),
		Nav:         navFor("/accounts"),
		Accounts:    accountsFor(accounts),
		AccountForm: accountForm(),
	}
	s.render(w, "accounts", data)
}

func (s *Server) categories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.store.ListCategories(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("categories store: %v", err)
		return
	}

	data := PageData{
		Title:        "Categorie",
		CurrentYear:  time.Now().Year(),
		Nav:          navFor("/categories"),
		Categories:   categoriesFor(categories),
		CategoryForm: categoryForm(),
	}
	s.render(w, "categories", data)
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderCategoryList(w, r, "Form non valido")
		return
	}

	input, err := store.ParseCategoryForm(r.PostForm)
	if err != nil {
		s.renderCategoryList(w, r, err.Error())
		return
	}

	if err := s.store.CreateCategory(r.Context(), input); err != nil {
		log.Printf("create category: %v", err)
		s.renderCategoryList(w, r, "Non sono riuscito a salvare la categoria")
		return
	}

	w.Header().Set("HX-Trigger", "category-created")
	s.renderCategoryList(w, r, "")
}

func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := s.store.DeleteCategory(r.Context(), store.DemoUserID, id); err != nil {
		log.Printf("delete category: %v", err)
		s.renderCategoryList(w, r, "Non sono riuscito a cancellare la categoria")
		return
	}

	w.Header().Set("HX-Trigger", "category-deleted")
	s.renderCategoryList(w, r, "")
}

func (s *Server) renderCategoryList(w http.ResponseWriter, r *http.Request, message string) {
	categories, err := s.store.ListCategories(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("category list store: %v", err)
		return
	}

	data := PageData{
		Categories: categoriesFor(categories),
		Error:      message,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "categories_result", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Printf("render category result: %v", err)
	}
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderAccountList(w, r, "Form non valido")
		return
	}

	input, err := store.ParseAccountForm(r.PostForm)
	if err != nil {
		s.renderAccountList(w, r, err.Error())
		return
	}

	if err := s.store.CreateAccount(r.Context(), input); err != nil {
		log.Printf("create account: %v", err)
		s.renderAccountList(w, r, "Non sono riuscito a salvare il conto")
		return
	}

	w.Header().Set("HX-Trigger", "account-created")
	s.renderAccountList(w, r, "")
}

func (s *Server) archiveAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := s.store.ArchiveAccount(r.Context(), store.DemoUserID, id); err != nil {
		log.Printf("archive account: %v", err)
		s.renderAccountList(w, r, "Non sono riuscito ad archiviare il conto")
		return
	}

	w.Header().Set("HX-Trigger", "account-archived")
	s.renderAccountList(w, r, "")
}

func (s *Server) renderAccountList(w http.ResponseWriter, r *http.Request, message string) {
	accounts, err := s.store.ListAccounts(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("account list store: %v", err)
		return
	}

	data := PageData{
		Accounts: accountsFor(accounts),
		Error:    message,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "accounts_result", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Printf("render account result: %v", err)
	}
}

func (s *Server) recentTransactions(w http.ResponseWriter, r *http.Request) {
	transactions, err := s.store.ListTransactions(r.Context(), store.DefaultTransactionFilter(store.DemoUserID))
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("recent transactions store: %v", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "recent_transactions", transactionsFor(transactions, 8, nil, nil)); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Printf("render partial: %v", err)
	}
}

func (s *Server) transactions(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.Dashboard(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("transactions store: %v", err)
		return
	}
	categories, err := s.store.ListCategories(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("transaction categories store: %v", err)
		return
	}
	filter := store.ParseTransactionFilter(r.URL.Query())
	transactions, err := s.store.ListTransactions(r.Context(), filter)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("filtered transactions store: %v", err)
		return
	}

	data := PageData{
		Title:       "Movimenti",
		CurrentYear: time.Now().Year(),
		Nav:         navFor("/transactions"),
		Accounts:    accountsFor(summary.Accounts),
		Categories:  categoriesFor(categories),
		Recent:      transactionsFor(transactions, 80, accountsFor(summary.Accounts), categoriesFor(categories)),
		Form: TransactionForm{
			Today:      store.Today(),
			Accounts:   accountsFor(summary.Accounts),
			Categories: categoriesFor(categories),
		},
		Filters: filtersFor(filter),
	}
	s.render(w, "transactions", data)
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.Dashboard(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("reports store: %v", err)
		return
	}

	data := PageData{
		Title:       "Report",
		CurrentYear: time.Now().Year(),
		Nav:         navFor("/reports"),
		Metrics:     metricsFor(summary),
		Reports:     reportRowsFor(summary.Transactions),
	}
	s.render(w, "reports", data)
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Title:       "Impostazioni",
		CurrentYear: time.Now().Year(),
		Nav:         navFor("/settings"),
	}
	s.render(w, "settings", data)
}

func (s *Server) createTransaction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderTransactionList(w, r, "Form non valido")
		return
	}

	input, err := store.ParseTransactionForm(r.PostForm)
	if err != nil {
		s.renderTransactionList(w, r, err.Error())
		return
	}

	if err := s.store.CreateTransaction(r.Context(), input); err != nil {
		log.Printf("create transaction: %v", err)
		s.renderTransactionList(w, r, "Non sono riuscito a salvare il movimento")
		return
	}

	w.Header().Set("HX-Trigger", "transaction-created")
	s.renderTransactionList(w, r, "")
}

func (s *Server) updateTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderTransactionList(w, r, "Form non valido")
		return
	}

	input, err := store.ParseTransactionForm(r.PostForm)
	if err != nil {
		s.renderTransactionList(w, r, err.Error())
		return
	}

	if err := s.store.UpdateTransaction(r.Context(), id, input); err != nil {
		log.Printf("update transaction: %v", err)
		s.renderTransactionList(w, r, "Non sono riuscito ad aggiornare il movimento")
		return
	}

	w.Header().Set("HX-Trigger", "transaction-updated")
	s.renderTransactionList(w, r, "")
}

func (s *Server) deleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := s.store.DeleteTransaction(r.Context(), store.DemoUserID, id); err != nil {
		log.Printf("delete transaction: %v", err)
		s.renderTransactionList(w, r, "Non sono riuscito a cancellare il movimento")
		return
	}

	w.Header().Set("HX-Trigger", "transaction-deleted")
	s.renderTransactionList(w, r, "")
}

func (s *Server) exportTransactions(w http.ResponseWriter, r *http.Request) {
	filter := store.ParseTransactionFilter(r.URL.Query())
	filter.Limit = -1
	transactions, err := s.store.ListTransactions(r.Context(), filter)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("export transactions store: %v", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="homeledger-transactions.csv"`)
	writer := csv.NewWriter(w)
	defer writer.Flush()

	_ = writer.Write([]string{"date", "kind", "account", "transfer_account", "amount", "category", "counterparty", "description"})
	for _, tx := range transactions {
		_ = writer.Write([]string{
			tx.Date.Format("2006-01-02"),
			tx.Kind,
			tx.AccountName,
			tx.TransferAccountName,
			fmt.Sprintf("%.2f", abs(tx.Amount)),
			tx.Category,
			tx.Counterparty,
			tx.Description,
		})
	}
}

func (s *Server) importTransactions(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		s.renderTransactionList(w, r, "File CSV non valido")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		s.renderTransactionList(w, r, "Seleziona un file CSV")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		s.renderTransactionList(w, r, "Il CSV deve avere intestazione e almeno una riga")
		return
	}

	accounts, err := s.store.ListAccounts(r.Context(), store.DemoUserID)
	if err != nil || len(accounts) == 0 {
		s.renderTransactionList(w, r, "Serve almeno un conto prima di importare")
		return
	}
	accountByName := map[string]int64{}
	for _, account := range accounts {
		accountByName[strings.ToLower(account.Name)] = account.ID
	}

	header := csvHeader(records[0])
	imported := 0
	for _, record := range records[1:] {
		input, err := transactionInputFromCSV(record, header, accountByName, accounts[0].ID)
		if err != nil {
			log.Printf("skip csv transaction: %v", err)
			continue
		}
		if err := s.store.CreateTransaction(r.Context(), input); err != nil {
			log.Printf("import csv transaction: %v", err)
			continue
		}
		imported++
	}

	s.renderTransactionList(w, r, fmt.Sprintf("Importati %d movimenti", imported))
}

func (s *Server) renderTransactionList(w http.ResponseWriter, r *http.Request, message string) {
	transactions, err := s.store.ListTransactions(r.Context(), store.DefaultTransactionFilter(store.DemoUserID))
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("transaction list store: %v", err)
		return
	}
	accounts, err := s.store.ListAccounts(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("transaction accounts store: %v", err)
		return
	}
	categories, err := s.store.ListCategories(r.Context(), store.DemoUserID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Printf("transaction categories store: %v", err)
		return
	}

	data := PageData{
		Recent: transactionsFor(transactions, 80, accountsFor(accounts), categoriesFor(categories)),
	}
	if strings.HasPrefix(message, "Importati") {
		data.Notice = message
	} else {
		data.Error = message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "transactions_result", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Printf("render transaction result: %v", err)
	}
}

func csvHeader(row []string) map[string]int {
	out := map[string]int{}
	for i, column := range row {
		out[strings.ToLower(strings.TrimSpace(column))] = i
	}
	return out
}

func csvValue(record []string, header map[string]int, names ...string) string {
	for _, name := range names {
		if index, ok := header[name]; ok && index >= 0 && index < len(record) {
			return strings.TrimSpace(record[index])
		}
	}
	return ""
}

func transactionInputFromCSV(record []string, header map[string]int, accountByName map[string]int64, defaultAccountID int64) (store.TransactionInput, error) {
	dateRaw := csvValue(record, header, "date", "occurred_on", "data")
	date, err := time.Parse("2006-01-02", dateRaw)
	if err != nil {
		return store.TransactionInput{}, fmt.Errorf("data non valida")
	}

	amount, err := strconv.ParseFloat(csvValue(record, header, "amount", "importo"), 64)
	if err != nil || amount == 0 {
		return store.TransactionInput{}, fmt.Errorf("importo non valido")
	}

	kind := strings.ToLower(csvValue(record, header, "kind", "tipo"))
	if kind == "" {
		if amount > 0 {
			kind = "income"
		} else {
			kind = "expense"
		}
	}
	if kind != "income" && kind != "expense" && kind != "transfer" {
		return store.TransactionInput{}, fmt.Errorf("tipo non valido")
	}

	accountID := defaultAccountID
	if accountName := strings.ToLower(csvValue(record, header, "account", "conto")); accountName != "" {
		if id, ok := accountByName[accountName]; ok {
			accountID = id
		}
	}

	transferAccountID := int64(0)
	if kind == "transfer" {
		transferName := strings.ToLower(csvValue(record, header, "transfer_account", "conto_destinazione"))
		id, ok := accountByName[transferName]
		if !ok || id == accountID {
			return store.TransactionInput{}, fmt.Errorf("conto destinazione non valido")
		}
		transferAccountID = id
	}

	if (kind == "expense" || kind == "transfer") && amount > 0 {
		amount = -amount
	}
	if kind == "income" && amount < 0 {
		amount = -amount
	}

	return store.TransactionInput{
		UserID:            store.DemoUserID,
		AccountID:         accountID,
		TransferAccountID: transferAccountID,
		OccurredOn:        date,
		Description:       csvValue(record, header, "description", "descrizione"),
		Amount:            amount,
		Kind:              kind,
		Category:          csvValue(record, header, "category", "categoria"),
		Counterparty:      csvValue(record, header, "counterparty", "controparte"),
	}, nil
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func filtersFor(filter store.TransactionFilter) FilterView {
	view := FilterView{
		AccountID: filter.AccountID,
		Category:  filter.Category,
		Kind:      filter.Kind,
		Query:     filter.Query,
	}
	if !filter.From.IsZero() {
		view.From = filter.From.Format("2006-01-02")
	}
	if !filter.To.IsZero() {
		view.To = filter.To.Format("2006-01-02")
	}
	return view
}

func (s *Server) placeholder(title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := PageData{
			Title:       title,
			CurrentYear: time.Now().Year(),
			Nav:         navFor(r.URL.Path),
		}
		s.render(w, "placeholder", data)
	}
}

func (s *Server) render(w http.ResponseWriter, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Printf("render %s: %v", name, err)
	}
}

func navFor(activePath string) []NavItem {
	items := []NavItem{
		{Label: "Dashboard", Href: "/", Icon: "layout-dashboard"},
		{Label: "Movimenti", Href: "/transactions", Icon: "list"},
		{Label: "Conti", Href: "/accounts", Icon: "wallet"},
		{Label: "Categorie", Href: "/categories", Icon: "tags"},
		{Label: "Report", Href: "/reports", Icon: "chart-column"},
		{Label: "Impostazioni", Href: "/settings", Icon: "settings"},
	}

	for i := range items {
		items[i].Active = cleanPath(items[i].Href) == cleanPath(activePath)
	}
	return items
}

func cleanPath(value string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(value))
	if cleaned == "/." {
		return "/"
	}
	return cleaned
}

func metricsFor(summary store.Dashboard) []Metric {
	var balance float64
	var income float64
	var expense float64
	now := time.Now()

	for _, account := range summary.Accounts {
		balance += account.Balance
	}
	for _, tx := range summary.Transactions {
		if tx.Date.Year() != now.Year() || tx.Date.Month() != now.Month() {
			continue
		}
		if tx.Kind == "transfer" {
			continue
		}
		if tx.Amount > 0 {
			income += tx.Amount
		} else {
			expense += -tx.Amount
		}
	}

	rate := 0.0
	if income > 0 {
		rate = ((income - expense) / income) * 100
	}

	return []Metric{
		{Label: "Saldo totale", Value: store.FormatMoney(balance), Tone: "neutral"},
		{Label: "Entrate mese", Value: store.FormatMoney(income), Tone: "income"},
		{Label: "Uscite mese", Value: store.FormatMoney(expense), Tone: "expense"},
		{Label: "Risparmio", Value: formatPercent(rate), Tone: "neutral"},
	}
}

func accountsFor(accounts []store.Account) []AccountSummary {
	out := make([]AccountSummary, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, AccountSummary{
			ID:      account.ID,
			Name:    account.Name,
			Type:    accountKindLabel(account.Kind),
			Balance: store.FormatMoney(account.Balance),
		})
	}
	return out
}

func accountKindLabel(kind string) string {
	switch kind {
	case "bank":
		return "Banca"
	case "cash":
		return "Contanti"
	case "card":
		return "Carta"
	case "savings":
		return "Risparmio"
	default:
		return kind
	}
}

func categoriesFor(categories []store.Category) []CategoryView {
	out := make([]CategoryView, 0, len(categories))
	for _, category := range categories {
		out = append(out, CategoryView{
			ID:          category.ID,
			Name:        category.Name,
			Color:       safeHexColor(category.Color),
			Icon:        category.Icon,
			SwatchStyle: template.CSS("background-color: " + safeHexColor(category.Color)),
		})
	}
	return out
}

func safeHexColor(color string) string {
	if len(color) != 7 || !strings.HasPrefix(color, "#") {
		return "#1f7a8c"
	}
	for _, char := range color[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return "#1f7a8c"
		}
	}
	return color
}

func accountForm() AccountForm {
	return AccountForm{
		Kinds: []Option{
			{Value: "bank", Label: "Banca"},
			{Value: "cash", Label: "Contanti"},
			{Value: "card", Label: "Carta"},
			{Value: "savings", Label: "Risparmio"},
		},
		Currencies: []Option{
			{Value: "EUR", Label: "EUR"},
			{Value: "USD", Label: "USD"},
			{Value: "GBP", Label: "GBP"},
		},
	}
}

func categoryForm() CategoryForm {
	return CategoryForm{
		Icons: []Option{
			{Value: "home", Label: "Casa"},
			{Value: "shopping-cart", Label: "Spesa"},
			{Value: "utensils", Label: "Cibo"},
			{Value: "car", Label: "Trasporti"},
			{Value: "briefcase", Label: "Lavoro"},
			{Value: "heart-pulse", Label: "Salute"},
			{Value: "film", Label: "Svago"},
			{Value: "circle", Label: "Altro"},
		},
	}
}

func reportRowsFor(transactions []store.Transaction) []ReportRow {
	now := time.Now()
	totals := map[string]float64{}
	totalExpense := 0.0

	for _, tx := range transactions {
		if tx.Date.Year() != now.Year() || tx.Date.Month() != now.Month() || tx.Amount >= 0 {
			continue
		}
		category := tx.Category
		if category == "" {
			category = "Senza categoria"
		}
		amount := -tx.Amount
		totals[category] += amount
		totalExpense += amount
	}

	if totalExpense == 0 {
		return []ReportRow{{Label: "Nessuna uscita nel mese", Amount: "0.00 EUR", Percentage: "0%", Tone: "neutral"}}
	}

	rows := make([]ReportRow, 0, len(totals))
	for category, amount := range totals {
		rows = append(rows, ReportRow{
			Label:      category,
			Amount:     store.FormatMoney(amount),
			Percentage: formatPercent((amount / totalExpense) * 100),
			Tone:       "expense",
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return totals[rows[i].Label] > totals[rows[j].Label]
	})
	return rows
}

func transactionsFor(transactions []store.Transaction, limit int, accounts []AccountSummary, categories []CategoryView) []Transaction {
	if len(transactions) == 0 {
		return []Transaction{
			{Date: "Oggi", Payee: "Nessun movimento reale", Category: "Demo", Amount: "0.00 EUR", Tone: "neutral"},
		}
	}

	if limit > 0 && len(transactions) > limit {
		transactions = transactions[:limit]
	}

	out := make([]Transaction, 0, len(transactions))
	for _, tx := range transactions {
		tone := "neutral"
		if tx.Amount > 0 {
			tone = "income"
		}
		if tx.Amount < 0 {
			tone = "expense"
		}
		if tx.Kind == "transfer" {
			tone = "transfer"
		}

		label := tx.Description
		transferLabel := ""
		if tx.Kind == "transfer" {
			transferLabel = tx.AccountName + " -> " + tx.TransferAccountName
			if label == "" {
				label = "Trasferimento"
			}
		}
		if label == "" {
			label = tx.Counterparty
		}
		if label == "" {
			label = tx.AccountName
		}
		if label == "" {
			label = "Movimento"
		}

		category := tx.Category
		if category == "" {
			category = tx.AccountName
		}

		out = append(out, Transaction{
			ID:                tx.ID,
			AccountID:         tx.AccountID,
			TransferAccountID: tx.TransferAccountID,
			TransferLabel:     transferLabel,
			Date:              tx.Date.Format("02/01"),
			DateInput:         tx.Date.Format("2006-01-02"),
			Payee:             label,
			Category:          category,
			Counterparty:      tx.Counterparty,
			Description:       tx.Description,
			Amount:            store.FormatMoney(tx.Amount),
			AmountInput:       fmt.Sprintf("%.2f", abs(tx.Amount)),
			Kind:              tx.Kind,
			IsIncome:          tx.Kind == "income",
			IsExpense:         tx.Kind == "expense",
			IsTransfer:        tx.Kind == "transfer",
			Tone:              tone,
			EditAccounts:      accounts,
			EditCategories:    categories,
		})
	}
	return out
}

func selected(current int64, candidate int64) template.HTMLAttr {
	if current == candidate {
		return "selected"
	}
	return ""
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func formatPercent(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", value), "0"), ".") + "%"
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func basicAuth(next http.Handler) http.Handler {
	expectedUser := os.Getenv("HOMELEDGER_USERNAME")
	expectedPassword := os.Getenv("HOMELEDGER_PASSWORD")
	if expectedUser == "" || expectedPassword == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		user, password, ok := r.BasicAuth()
		userMatches := subtle.ConstantTimeCompare([]byte(user), []byte(expectedUser)) == 1
		passwordMatches := subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) == 1
		if !ok || !userMatches || !passwordMatches {
			w.Header().Set("WWW-Authenticate", `Basic realm="HomeLedger"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
