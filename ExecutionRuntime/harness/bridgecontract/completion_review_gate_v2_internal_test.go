package bridgecontract

import "testing"

func TestCompletionReviewGateV2EachSourceCanBeTheUniqueMinimumTTL(t *testing.T) {
	const long = int64(10_000)
	tests := []struct {
		name   string
		values [4]int64
		want   int64
	}{
		{"request", [4]int64{1_001, long + 2, long + 3, long + 4}, 1_001},
		{"input", [4]int64{long + 1, 1_002, long + 3, long + 4}, 1_002},
		{"review", [4]int64{long + 1, long + 2, 1_003, long + 4}, 1_003},
		{"receipt", [4]int64{long + 1, long + 2, long + 3, 1_004}, 1_004},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := minimumCompletionReviewGateExpiryV2(testCase.values[:]...)
			if got != testCase.want {
				t.Fatalf("minimum TTL = %d, want %d", got, testCase.want)
			}
		})
	}
}
