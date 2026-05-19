package timing

import (
	"testing"
)

func tv(s string) TimeValue { return MustParseTimeValue(s) }

func TestParseTimeValue(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"0.000", 0},
		{"1.000", 1},
		{"1.234", 1.234},
		{"60.000", 60},
		{"3600.000", 3600},
	}
	for _, tt := range tests {
		v, err := ParseTimeValue(tt.in)
		if err != nil {
			t.Errorf("ParseTimeValue(%q): %v", tt.in, err)
			continue
		}
		got := v.Float64()
		if got != tt.want {
			t.Errorf("ParseTimeValue(%q).Float64() = %f, want %f", tt.in, got, tt.want)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := ParseTimeValue("abc"); err == nil {
		t.Error("expected error for 'abc'")
	}
	if _, err := ParseTimeValue("-1.0"); err == nil {
		t.Error("expected error for '-1.0'")
	}
}

func TestAddSub(t *testing.T) {
	a := tv("1.500")
	b := tv("0.500")
	if !a.Add(b).Equal(tv("2.000")) {
		t.Errorf("Add: got %s", a.Add(b))
	}
	if !a.Sub(b).Equal(tv("1.000")) {
		t.Errorf("Sub: got %s", a.Sub(b))
	}
}

func TestMulDiv(t *testing.T) {
	a := tv("1.500")
	if !a.MulInt(2).Equal(tv("3.000")) {
		t.Errorf("MulInt: got %s", a.MulInt(2))
	}
	if !a.DivInt(3).Equal(tv("0.500")) {
		t.Errorf("DivInt: got %s", a.DivInt(3))
	}
}

func TestCmp(t *testing.T) {
	a, b := tv("1.000"), tv("2.000")
	if !a.Less(b) {
		t.Error("1.000 < 2.000 should be true")
	}
	if a.Greater(b) {
		t.Error("1.000 > 2.000 should be false")
	}
	if !a.Equal(tv("1.000")) {
		t.Error("1.000 == 1.000 should be true")
	}
}

func TestMaxMin(t *testing.T) {
	a, b := tv("1.000"), tv("2.000")
	if !Max(a, b).Equal(b) {
		t.Errorf("Max: got %s", Max(a, b))
	}
	if !Min(a, b).Equal(a) {
		t.Errorf("Min: got %s", Min(a, b))
	}
}

func TestFormatHMSm(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0.000", "00:00:00.000"},
		{"1.500", "00:00:01.500"},
		{"61.123", "00:01:01.123"},
		{"3661.001", "01:01:01.001"},
	}
	for _, tt := range tests {
		got := tv(tt.in).FormatHMSm()
		if got != tt.want {
			t.Errorf("FormatHMSm(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTimeIntervalContains(t *testing.T) {
	ti := NewTimeInterval(tv("1.000"), tv("3.000"))
	if !ti.Contains(tv("2.000")) {
		t.Error("should contain 2.000")
	}
	if !ti.Contains(tv("1.000")) {
		t.Error("should contain begin")
	}
	if !ti.Contains(tv("3.000")) {
		t.Error("should contain end")
	}
	if ti.Contains(tv("0.999")) {
		t.Error("should not contain 0.999")
	}
	if ti.Contains(tv("3.001")) {
		t.Error("should not contain 3.001")
	}
}

func TestTimeIntervalLength(t *testing.T) {
	ti := NewTimeInterval(tv("1.000"), tv("3.500"))
	if !ti.Length().Equal(tv("2.500")) {
		t.Errorf("Length: got %s, want 2.500", ti.Length())
	}
}

func TestAdjacentIntervals(t *testing.T) {
	a := NewTimeInterval(tv("0.000"), tv("1.000"))
	b := NewTimeInterval(tv("1.000"), tv("2.000"))
	if !a.IsAdjacentBefore(b) {
		t.Error("a should be adjacent before b")
	}
	if !b.IsAdjacentAfter(a) {
		t.Error("b should be adjacent after a")
	}
}

func TestZeroLengthInterval(t *testing.T) {
	ti := NewTimeInterval(tv("1.000"), tv("1.000"))
	if !ti.HasZeroLength() {
		t.Error("should have zero length")
	}
}
