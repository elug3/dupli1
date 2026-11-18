package domain

import "testing"

func entriesOf(a *PointAccount, kind string) []PointEntry {
	var out []PointEntry
	for _, e := range a.NewEntries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func TestReviewWrittenTextThenPhoto(t *testing.T) {
	a := newAccount()
	if got := a.ReviewWritten("ord-1", "sku-1", "rev-1", 19, 0, t0); got != 0 {
		t.Fatalf("19 characters earned %d", got)
	}
	if got := a.ReviewWritten("ord-1", "sku-1", "rev-1", 20, 0, t0); got != ReviewTextPoints {
		t.Fatalf("text review earned %d", got)
	}
	// Replayed or edited without a photo: nothing more.
	if got := a.ReviewWritten("ord-1", "sku-1", "rev-1", 40, 0, t0); got != 0 {
		t.Fatalf("replay earned %d", got)
	}
	// A photo added later tops it up to 5,000 in all.
	if got := a.ReviewWritten("ord-1", "sku-1", "rev-1", 40, 2, t0); got != ReviewPhotoPoints-ReviewTextPoints {
		t.Fatalf("photo top-up earned %d", got)
	}
	if a.Balance(t0) != ReviewPhotoPoints {
		t.Fatalf("balance = %d, want 5000", a.Balance(t0))
	}
	es := entriesOf(a, EntryReview)
	if len(es) != 2 || es[0].Note != ReviewNoteText || es[1].Note != ReviewNotePhoto || es[1].OrderID != "ord-1" || es[1].Ref != "rev-1" {
		t.Fatalf("entries = %+v", es)
	}
}

func TestReviewWithPhotoEarnsFiveThousandAtOnce(t *testing.T) {
	a := newAccount()
	if got := a.ReviewWritten("ord-1", "sku-1", "rev-1", 25, 1, t0); got != ReviewPhotoPoints {
		t.Fatalf("photo review earned %d", got)
	}
	if es := entriesOf(a, EntryReview); len(es) != 1 || es[0].Points != ReviewPhotoPoints {
		t.Fatalf("entries = %+v", es)
	}
}

// Deleting a review and writing another for the same line earns nothing.
func TestReviewEarnsOncePerLine(t *testing.T) {
	a := newAccount()
	a.ReviewWritten("ord-1", "sku-1", "rev-1", 30, 0, t0)
	if got := a.ReviewWritten("ord-1", "sku-1", "rev-2", 30, 0, t0); got != 0 {
		t.Fatalf("second review of the line earned %d", got)
	}
	if got := a.ReviewWritten("ord-1", "sku-2", "rev-3", 30, 0, t0); got != ReviewTextPoints {
		t.Fatalf("another line earned %d", got)
	}
}

func TestReviewHiddenTakesBackForGood(t *testing.T) {
	a := newAccount()
	a.ReviewWritten("ord-1", "sku-1", "rev-1", 30, 1, t0)
	if short := a.ReviewHidden("ord-1", "sku-1", t0); short != 0 {
		t.Fatalf("shortfall = %d", short)
	}
	if a.Balance(t0) != 0 {
		t.Fatalf("balance after hide = %d", a.Balance(t0))
	}
	// Unhidden (or edited) later: still nothing.
	if got := a.ReviewWritten("ord-1", "sku-1", "rev-1", 30, 1, t0); got != 0 {
		t.Fatalf("after hide earned %d", got)
	}
}

func TestReviewHiddenAfterSpendingIsForgiven(t *testing.T) {
	a := newAccount()
	a.ReviewWritten("ord-1", "sku-1", "rev-1", 30, 0, t0)
	if _, err := a.Reserve("ord-9", 1_500, t0); err != nil {
		t.Fatal(err)
	}
	if short := a.ReviewHidden("ord-1", "sku-1", t0); short != 1_500 {
		t.Fatalf("shortfall = %d, want 1500", short)
	}
	if a.Balance(t0) != 0 {
		t.Fatalf("balance = %d", a.Balance(t0))
	}
}

func TestCanceledOrderTakesBackReviewRewards(t *testing.T) {
	a := newAccount()
	a.OrderPaid("ord-1", 100_000, t0)
	a.OrderFulfilled("ord-1", 100_000, t0)
	a.ReviewWritten("ord-1", "sku-1", "rev-1", 30, 1, t0)
	if a.Balance(t0) != 1_000+ReviewPhotoPoints {
		t.Fatalf("balance = %d", a.Balance(t0))
	}
	a.OrderCanceled("ord-1", t0)
	if a.Balance(t0) != 0 {
		t.Fatalf("balance after cancel = %d", a.Balance(t0))
	}
	if got := a.ReviewWritten("ord-1", "sku-2", "rev-2", 30, 0, t0); got != 0 {
		t.Fatalf("review of a canceled order earned %d", got)
	}
}
