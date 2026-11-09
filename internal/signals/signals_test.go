package signals

import "testing"

func TestRepeatedQuestionsGrouped(t *testing.T) {
	qs := []string{
		"what is the difference between readiness and liveness probe",
		"ok but readiness vs liveness probe difference again?",
		"how do I change the font size",
		"explain readiness and liveness probe difference once more",
	}
	g := Repeated(qs)
	if len(g) != 1 || len(g[0]) != 3 {
		t.Fatalf("%v", g)
	}
}
