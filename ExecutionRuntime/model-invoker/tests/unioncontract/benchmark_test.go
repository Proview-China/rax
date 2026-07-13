package unioncontract_test

import "testing"

var benchmarkDigest string

func BenchmarkUnionDigest(b *testing.B) {
	request := validRequest()
	plan := validPlan()
	event := validEffectEvent()
	command := validApprovalCommand()
	result := validResult()

	benchmarks := []struct {
		name   string
		digest func() (string, error)
	}{
		{name: "request", digest: request.Digest},
		{name: "plan", digest: plan.ComputeDigest},
		{name: "event", digest: event.Digest},
		{name: "command", digest: command.Digest},
		{name: "result", digest: result.ComputeDigest},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				digest, err := benchmark.digest()
				if err != nil {
					b.Fatalf("digest: %v", err)
				}
				benchmarkDigest = digest
			}
		})
	}
}
