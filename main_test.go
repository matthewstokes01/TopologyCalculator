package main

import (
	"testing"
)

func TestCalculateTopologySkew(t *testing.T) {
	t.Parallel()
	testCase := map[string]int{
		"eu-west-2a": 10,
		"eu-west-2b": 5,
		"eu-west-2c": 6,
	}

	got := calculateTopologySkew(testCase)
	want := 5
	if want != got {
		t.Errorf("Skew should equal %v, got %v", want, got)
	}
}
