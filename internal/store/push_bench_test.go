package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Isolate the indexed active-receipt count used by every new push commit.
// Seed only this table; these figures are query costs, not ingestion capacity.
func BenchmarkPushReceiptCount(b *testing.B) {
	for _, size := range []int{0, 1000, 10000, 100000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			ctx := context.Background()
			s, err := Open(ctx, b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			now := time.Now()
			if err := s.Update(ctx, func(tx *Tx) error {
				for i := range size {
					_, err := tx.sql.ExecContext(ctx, "INSERT INTO push_receipts(lifetime,producer,input_key,digest,body,expires_at) VALUES('watch','producer',?,'digest','{}',?)", fmt.Sprint(i), timestamp(now.Add(24*time.Hour)))
					if err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := s.View(ctx, func(tx *Tx) error {
					n, err := tx.PushReceiptCount(now)
					if err == nil && n != size {
						return fmt.Errorf("count %d", n)
					}
					return err
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
