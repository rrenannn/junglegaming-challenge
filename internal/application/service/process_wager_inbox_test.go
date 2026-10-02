package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestProcessWagerService_InboxReplayWithSameHashSkipsReprocessing(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-inbox-1", "100.00")

	cmd := betCommand(walletID, "player-inbox-1", "30.00")
	cmd.Inbox = &InboxInfo{ConsumerName: "wager-transactions", MessageID: uuid.NewString()}

	first, err := svc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if first.AlreadyProcessed {
		t.Fatal("first delivery should not be marked as already processed")
	}

	second, err := svc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !second.AlreadyProcessed {
		t.Fatal("redelivery with the same message id and content should be marked as already processed")
	}

	if count := countWagerTransactions(t, pool, walletID); count != 1 {
		t.Fatalf("wager transactions = %d, want 1", count)
	}
	if count := countLedgerEntries(t, pool, walletID); count != 1 {
		t.Fatalf("ledger entries = %d, want 1", count)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "70.00" {
		t.Fatalf("final balance = %s, want 70.00 (redelivery must not debit twice)", balance)
	}
}

func TestProcessWagerService_InboxReplayWithDifferentHashConflicts(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-inbox-2", "100.00")

	messageID := uuid.NewString()

	first := betCommand(walletID, "player-inbox-2", "30.00")
	first.Inbox = &InboxInfo{ConsumerName: "wager-transactions", MessageID: messageID}
	if _, err := svc.Execute(context.Background(), first); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	second := betCommand(walletID, "player-inbox-2", "40.00")
	second.Inbox = &InboxInfo{ConsumerName: "wager-transactions", MessageID: messageID}

	_, err := svc.Execute(context.Background(), second)
	if !errors.Is(err, ErrInboxHashConflict) {
		t.Fatalf("err = %v, want ErrInboxHashConflict", err)
	}
}

func TestProcessWagerService_InboxDifferentMessageIDsProcessIndependently(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-inbox-3", "100.00")

	cmdA := betCommand(walletID, "player-inbox-3", "10.00")
	cmdA.Inbox = &InboxInfo{ConsumerName: "wager-transactions", MessageID: uuid.NewString()}
	if _, err := svc.Execute(context.Background(), cmdA); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	cmdB := betCommand(walletID, "player-inbox-3", "10.00")
	cmdB.Inbox = &InboxInfo{ConsumerName: "wager-transactions", MessageID: uuid.NewString()}
	if _, err := svc.Execute(context.Background(), cmdB); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	if count := countWagerTransactions(t, pool, walletID); count != 2 {
		t.Fatalf("wager transactions = %d, want 2", count)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "80.00" {
		t.Fatalf("final balance = %s, want 80.00", balance)
	}
}
