package repository

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
)

type recordedQuery struct {
	sql  string
	args []any
}

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

type fakeProductTx struct {
	queries    []recordedQuery
	committed  bool
	rolledBack bool
}

func (t *fakeProductTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return t, nil
}

func (t *fakeProductTx) Commit(ctx context.Context) error {
	t.committed = true
	return nil
}

func (t *fakeProductTx) Rollback(ctx context.Context) error {
	if !t.committed {
		t.rolledBack = true
	}
	return nil
}

func (t *fakeProductTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (t *fakeProductTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}

func (t *fakeProductTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (t *fakeProductTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (t *fakeProductTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	t.queries = append(t.queries, recordedQuery{sql: sql, args: arguments})
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (t *fakeProductTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (t *fakeProductTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	t.queries = append(t.queries, recordedQuery{sql: sql, args: args})
	now := time.Now().UTC()

	switch {
	case strings.Contains(sql, "INSERT INTO products"):
		return &fakeRow{
			scanFn: func(dest ...any) error {
				*(dest[0].(*string)) = "prod-123"
				*(dest[1].(*time.Time)) = now
				*(dest[2].(*time.Time)) = now
				return nil
			},
		}
	case strings.Contains(sql, "FOR UPDATE"):
		return &fakeRow{
			scanFn: func(dest ...any) error {
				*(dest[0].(*model.ProductStatus)) = model.StatusDraft
				*(dest[1].(*string)) = "USD"
				*(dest[2].(*string)) = "SKU-123"
				return nil
			},
		}
	case strings.Contains(sql, "UPDATE products"):
		return &fakeRow{
			scanFn: func(dest ...any) error {
				*(dest[0].(*time.Time)) = now
				return nil
			},
		}
	default:
		return &fakeRow{}
	}
}

func (t *fakeProductTx) Conn() *pgx.Conn {
	return nil
}

var placeholderRegex = regexp.MustCompile(`\$(\d+)`)

func assertPlaceholdersMatchArgs(t *testing.T, query recordedQuery, expectedCount int) {
	t.Helper()
	if len(query.args) != expectedCount {
		t.Fatalf("expected %d query arguments, got %d", expectedCount, len(query.args))
	}
	matches := placeholderRegex.FindAllStringSubmatch(query.sql, -1)
	if len(matches) != expectedCount {
		t.Fatalf("expected %d SQL placeholders, got %d in query:\n%s", expectedCount, len(matches), query.sql)
	}
	seen := make(map[int]bool, expectedCount)
	for i, m := range matches {
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("invalid placeholder %q: %v", m[0], err)
		}
		if idx != i+1 {
			t.Fatalf("expected placeholder $%d at position %d, got $%d in query:\n%s", i+1, i+1, idx, query.sql)
		}
		if seen[idx] {
			t.Fatalf("duplicate placeholder $%d in query:\n%s", idx, query.sql)
		}
		seen[idx] = true
	}
}

func TestCreateProduct_SQLPlaceholdersAndWeightMapping(t *testing.T) {
	tx := &fakeProductTx{}
	repo := NewProductRepository(nil).(*pgProductRepository)
	repo.beginTx = func(ctx context.Context) (pgx.Tx, error) {
		return tx, nil
	}

	product := &model.Product{
		StoreID:             "store-1",
		CategoryID:          "cat-1",
		SKU:                 "SKU-001",
		Name:                "Wireless Mouse",
		Description:         "Ergonomic mouse",
		Price:               29.99,
		MRP:                 39.99,
		Tax:                 2.50,
		Currency:            "USD",
		Status:              model.StatusDraft,
		ImageURL:            "https://example.com/mouse.png",
		AvgRating:           4.5,
		Weight:              0.125,
		IsExchangeEligible:  true,
		MaxExchangeDiscount: 5.00,
	}

	if err := repo.CreateProduct(context.Background(), product); err != nil {
		t.Fatalf("CreateProduct returned unexpected error: %v", err)
	}
	if !tx.committed {
		t.Fatal("expected transaction to commit")
	}

	var insertQuery *recordedQuery
	for i := range tx.queries {
		if strings.Contains(tx.queries[i].sql, "INSERT INTO products") {
			insertQuery = &tx.queries[i]
			break
		}
	}
	if insertQuery == nil {
		t.Fatal("expected INSERT INTO products query to be executed")
	}

	assertPlaceholdersMatchArgs(t, *insertQuery, 15)

	if got, ok := insertQuery.args[12].(float64); !ok || got != 0.125 {
		t.Fatalf("expected $13 (weight) = 0.125, got %#v", insertQuery.args[12])
	}
	if got, ok := insertQuery.args[13].(bool); !ok || !got {
		t.Fatalf("expected $14 (is_exchange_eligible) = true, got %#v", insertQuery.args[13])
	}
	if got, ok := insertQuery.args[14].(float64); !ok || got != 5.00 {
		t.Fatalf("expected $15 (max_exchange_discount) = 5.00, got %#v", insertQuery.args[14])
	}
}

func TestUpdateProduct_SQLPlaceholdersAndWeightMapping(t *testing.T) {
	tx := &fakeProductTx{}
	repo := NewProductRepository(nil).(*pgProductRepository)
	repo.beginTx = func(ctx context.Context) (pgx.Tx, error) {
		return tx, nil
	}

	product := &model.Product{
		ID:                  "prod-123",
		StoreID:             "store-1",
		CategoryID:          "cat-2",
		SKU:                 "SKU-001",
		Name:                "Wireless Mouse Pro",
		Description:         "Updated ergonomic mouse",
		Price:               34.99,
		MRP:                 44.99,
		Tax:                 3.00,
		Currency:            "USD",
		Status:              model.StatusDraft,
		ImageURL:            "https://example.com/mouse-pro.png",
		Weight:              0.140,
		IsExchangeEligible:  true,
		MaxExchangeDiscount: 7.50,
	}

	if err := repo.UpdateProduct(context.Background(), product); err != nil {
		t.Fatalf("UpdateProduct returned unexpected error: %v", err)
	}
	if !tx.committed {
		t.Fatal("expected transaction to commit")
	}

	var updateQuery *recordedQuery
	for i := range tx.queries {
		if strings.Contains(tx.queries[i].sql, "UPDATE products") {
			updateQuery = &tx.queries[i]
			break
		}
	}
	if updateQuery == nil {
		t.Fatal("expected UPDATE products query to be executed")
	}

	assertPlaceholdersMatchArgs(t, *updateQuery, 12)

	expectedClauses := []string{
		"weight = $9",
		"is_exchange_eligible = $10",
		"max_exchange_discount = $11",
		"WHERE id = $12",
	}
	for _, clause := range expectedClauses {
		if !strings.Contains(updateQuery.sql, clause) {
			t.Fatalf("expected UPDATE query to contain %q, got:\n%s", clause, updateQuery.sql)
		}
	}

	if got, ok := updateQuery.args[8].(float64); !ok || got != 0.140 {
		t.Fatalf("expected $9 (weight) = 0.140, got %#v", updateQuery.args[8])
	}
	if got, ok := updateQuery.args[9].(bool); !ok || !got {
		t.Fatalf("expected $10 (is_exchange_eligible) = true, got %#v", updateQuery.args[9])
	}
	if got, ok := updateQuery.args[10].(float64); !ok || got != 7.50 {
		t.Fatalf("expected $11 (max_exchange_discount) = 7.50, got %#v", updateQuery.args[10])
	}
	if got, ok := updateQuery.args[11].(string); !ok || got != "prod-123" {
		t.Fatalf("expected $12 (id) = prod-123, got %#v", updateQuery.args[11])
	}
}

