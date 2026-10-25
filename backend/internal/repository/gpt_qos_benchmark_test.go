package repository

import (
	"context"
	"strconv"
	"testing"
)

// Local Redis emulation isolates scheduler/script overhead. This is not a
// production network-latency benchmark or an upstream model throughput claim.
func BenchmarkGPTQoSAdmission(b *testing.B) {
	for _, qos := range []bool{false, true} {
		name := "flags_off_account_slot"
		if qos {
			name = "qos_idle_fair_admission"
		}
		b.Run(name, func(b *testing.B) {
			c, _ := qosTestCache(b)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				id := strconv.Itoa(i)
				if qos {
					grant, err := c.GPTQoSPoll(ctx, qosEntry(id, 1, "premium"))
					if err != nil || grant.Status != "granted" {
						b.Fatalf("admission: %v %s", err, grant.Status)
					}
					if err = c.GPTQoSClose(ctx, id); err != nil {
						b.Fatal(err)
					}
				} else {
					ok, err := c.AcquireAccountSlot(ctx, 1, 1, id)
					if err != nil || !ok {
						b.Fatalf("acquire: %v", err)
					}
					if err = c.ReleaseAccountSlot(ctx, 1, id); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
