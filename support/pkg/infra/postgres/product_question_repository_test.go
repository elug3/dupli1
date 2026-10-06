package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/infra/postgres"
	"github.com/elug3/dupli1/support/pkg/ports"
)

func TestProductQuestionRepositoryRoundTrip(t *testing.T) {
	db := requirePostgres(t)
	ctx := context.Background()
	product := "PQ-TEST-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM support_product_questions WHERE product_id = $1`, product) })
	repo := postgres.NewProductQuestionRepository(db)

	base := time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC)
	first := &domain.ProductQuestion{
		ID: product + "-1", ProductID: product, SkuID: "SKU-M", VariantLabel: "Black / M", ProductName: "숏 패딩 재킷",
		CustomerID: "u1", CustomerEmail: "s3jin@gmail.com", AuthorMask: "s3****", Type: domain.QuestionSize,
		Body: "M?", Fit: &domain.Fit{HeightCm: 178, UsualSize: "L"}, Secret: true,
		Status: domain.QuestionWaiting, CreatedAt: base, UpdatedAt: base,
	}
	second := &domain.ProductQuestion{
		ID: product + "-2", ProductID: product, CustomerID: "u2", AuthorMask: "mk****", Type: domain.QuestionStock,
		Body: "재입고?", Status: domain.QuestionWaiting, CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute),
	}
	for _, q := range []*domain.ProductQuestion{first, second} {
		if err := repo.Save(ctx, q); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	got, err := repo.FindByID(ctx, first.ID)
	if err != nil || got == nil {
		t.Fatalf("find: %v %v", got, err)
	}
	if got.Fit == nil || got.Fit.HeightCm != 178 || !got.Secret || got.VariantLabel != "Black / M" || got.AnsweredAt != nil {
		t.Fatalf("round trip = %+v", got)
	}

	waiting, _ := repo.ListForStaff(ctx, ports.ProductQuestionFilter{ProductID: product})
	if len(waiting) != 2 || waiting[0].ID != first.ID {
		t.Fatalf("waiting queue should be oldest first: %+v", waiting)
	}

	now := base.Add(time.Hour)
	got.Status, got.Answer, got.AnsweredBy, got.AnsweredAt = domain.QuestionAnswered, "M", "mgr", &now
	if err := repo.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	second.Hidden, second.HiddenBy, second.HiddenAt = true, "mgr", &now
	if err := repo.Save(ctx, second); err != nil {
		t.Fatal(err)
	}

	visible, _ := repo.ListByProduct(ctx, product, false)
	if len(visible) != 1 || visible[0].Answer != "M" || visible[0].AnsweredAt == nil {
		t.Fatalf("visible = %+v", visible)
	}
	all, _ := repo.ListByProduct(ctx, product, true)
	if len(all) != 2 || all[0].ID != second.ID {
		t.Fatalf("all should be newest first: %+v", all)
	}
	answered, _ := repo.ListForStaff(ctx, ports.ProductQuestionFilter{Queue: ports.QuestionQueueAnswered, ProductID: product})
	hidden, _ := repo.ListForStaff(ctx, ports.ProductQuestionFilter{Queue: ports.QuestionQueueHidden, ProductID: product, Type: domain.QuestionStock})
	if len(answered) != 1 || len(hidden) != 1 || hidden[0].HiddenBy != "mgr" {
		t.Fatalf("answered=%d hidden=%+v", len(answered), hidden)
	}
	mine, _ := repo.ListByCustomer(ctx, "u1")
	found := false
	for _, q := range mine {
		found = found || q.ID == first.ID
	}
	if !found {
		t.Fatal("ListByCustomer missed the question")
	}

	got.Fit = nil
	if err := repo.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.FindByID(ctx, first.ID); again.Fit != nil {
		t.Fatalf("fit not cleared: %+v", again.Fit)
	}
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if gone, _ := repo.FindByID(ctx, first.ID); gone != nil {
		t.Fatal("not deleted")
	}
}
