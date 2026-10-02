package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/idgen"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
)

type stepClock struct {
	mu   sync.Mutex
	next time.Time
}

func newStepClock() *stepClock {
	return &stepClock{next: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.next
	c.next = c.next.Add(time.Second)
	return t
}

func seedLedgerFixture(t *testing.T, pool *pgxpool.Pool) (walletID string, listSvc *ListLedgerService) {
	t.Helper()

	uow := postgres.NewUnitOfWork(pool)
	clk := newStepClock()
	ids := idgen.NewUUIDGenerator()

	openSvc := NewOpenWalletService(uow, clk, ids)
	wagerSvc := NewProcessWagerService(uow, clk, ids, port.NoopMetrics{})

	wallet, err := openSvc.Execute(context.Background(), OpenWalletCommand{
		PlayerID:       "player-ledger",
		Currency:       "BRL",
		OpeningBalance: "100.00",
	})
	if err != nil {
		t.Fatalf("open wallet: %v", err)
	}

	if _, err := wagerSvc.Execute(context.Background(), betCommand(wallet.ID(), "player-ledger", "30.00")); err != nil {
		t.Fatalf("process bet: %v", err)
	}

	winCmd := ProcessWagerCommand{
		TransactionID:         uuid.NewString(),
		ProviderID:            "provider-a",
		PlayerID:              "player-ledger",
		WalletID:              wallet.ID(),
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  domain.KindWin,
		Amount:                mustParseMoney(t, "10.00"),
		ExternalTransactionID: uuid.NewString(),
	}
	if _, err := wagerSvc.Execute(context.Background(), winCmd); err != nil {
		t.Fatalf("process win: %v", err)
	}

	return wallet.ID(), NewListLedgerService(uow)
}

func mustParseMoney(t *testing.T, decimal string) domain.Money {
	t.Helper()
	money, err := domain.ParseMoney(decimal, domain.BRL)
	if err != nil {
		t.Fatalf("ParseMoney(%q): %v", decimal, err)
	}
	return money
}

func TestListLedgerService_PaginatesInStableOrder(t *testing.T) {
	pool := newTestPool(t)
	walletID, svc := seedLedgerFixture(t, pool)

	var (
		cursor      string
		directions  []domain.MovementDirection
		nextCursors []string
	)
	for i := 0; i < 3; i++ {
		page, err := svc.Execute(context.Background(), ListLedgerQuery{WalletID: walletID, Cursor: cursor, Limit: 1})
		if err != nil {
			t.Fatalf("Execute (page %d): %v", i, err)
		}
		if len(page.Entries) != 1 {
			t.Fatalf("page %d entries = %d, want 1", i, len(page.Entries))
		}
		directions = append(directions, page.Entries[0].Direction())
		nextCursors = append(nextCursors, page.NextCursor)
		cursor = page.NextCursor
	}

	wantDirections := []domain.MovementDirection{domain.DirectionCredit, domain.DirectionDebit, domain.DirectionCredit}
	for i, want := range wantDirections {
		if directions[i] != want {
			t.Fatalf("page %d direction = %s, want %s", i, directions[i], want)
		}
	}
	if nextCursors[0] == "" || nextCursors[1] == "" {
		t.Fatalf("expected non-empty cursors for pages 0 and 1, got %q, %q", nextCursors[0], nextCursors[1])
	}
	if nextCursors[2] != "" {
		t.Fatalf("expected empty cursor on the last page, got %q", nextCursors[2])
	}
}

func TestListLedgerService_WalletNotFound(t *testing.T) {
	pool := newTestPool(t)
	svc := NewListLedgerService(postgres.NewUnitOfWork(pool))

	_, err := svc.Execute(context.Background(), ListLedgerQuery{WalletID: uuid.NewString()})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want repository.ErrNotFound", err)
	}
}

func TestListLedgerService_InvalidCursorIsRejected(t *testing.T) {
	pool := newTestPool(t)
	walletID := seedWallet(t, pool, "player-bad-cursor", "10.00")
	svc := NewListLedgerService(postgres.NewUnitOfWork(pool))

	_, err := svc.Execute(context.Background(), ListLedgerQuery{WalletID: walletID, Cursor: "not-a-valid-cursor!!"})
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}
