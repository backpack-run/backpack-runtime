package inference

import "testing"

func TestAgentOutputTokenBudgetPreservesInputWindow(t *testing.T) {
	for _, test := range []struct{ context, want int }{
		{0, 8192},
		{32768, 8192},
		{65536, 8192},
		{16384, 4096},
		{4096, 1024},
	} {
		if got := AgentOutputTokenBudget(test.context); got != test.want {
			t.Fatalf("AgentOutputTokenBudget(%d)=%d, want %d", test.context, got, test.want)
		}
	}
}
