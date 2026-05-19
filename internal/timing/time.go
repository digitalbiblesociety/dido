// Package timing provides exact rational time arithmetic.
// This is a port of aeneas/exacttiming.py.
// TimeValue is backed by math/big.Rat for exact arithmetic.
package timing

import (
	"fmt"
	"math/big"
	"strings"
)

// TimeValue is an exact rational time value (seconds).
// Backed by *big.Rat so all arithmetic is exact.
type TimeValue struct {
	r *big.Rat
}

// Zero is the zero TimeValue.
var Zero = TimeValue{r: new(big.Rat)}

// ParseTimeValue parses a decimal string like "1.234" or "0.000" into a TimeValue.
func ParseTimeValue(s string) (TimeValue, error) {
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		return Zero, fmt.Errorf("timing: cannot parse %q as TimeValue", s)
	}
	if r.Sign() < 0 {
		return Zero, fmt.Errorf("timing: TimeValue %q is negative", s)
	}
	return TimeValue{r: r}, nil
}

// MustParseTimeValue panics if s cannot be parsed.
func MustParseTimeValue(s string) TimeValue {
	tv, err := ParseTimeValue(s)
	if err != nil {
		panic(err)
	}
	return tv
}

// FromRat wraps a *big.Rat as a TimeValue (takes ownership).
func FromRat(r *big.Rat) TimeValue { return TimeValue{r: new(big.Rat).Set(r)} }

// FromFloat64 converts a float64 to a TimeValue via rational approximation.
func FromFloat64(f float64) TimeValue {
	return TimeValue{r: new(big.Rat).SetFloat64(f)}
}

// FromInt64 returns num/denom as a TimeValue.
func FromInt64(num, denom int64) TimeValue {
	return TimeValue{r: new(big.Rat).SetFrac(big.NewInt(num), big.NewInt(denom))}
}

// Float64 returns the float64 approximation.
func (t TimeValue) Float64() float64 {
	f, _ := t.r.Float64()
	return f
}

// Rat returns a copy of the underlying *big.Rat.
func (t TimeValue) Rat() *big.Rat { return new(big.Rat).Set(t.r) }

// IsZero returns true if the value is exactly 0.
func (t TimeValue) IsZero() bool { return t.r.Sign() == 0 }

// Cmp compares t and other: -1, 0, or +1.
func (t TimeValue) Cmp(other TimeValue) int { return t.r.Cmp(other.r) }

// Equal returns true if t == other.
func (t TimeValue) Equal(other TimeValue) bool { return t.r.Cmp(other.r) == 0 }

// Less returns true if t < other.
func (t TimeValue) Less(other TimeValue) bool { return t.r.Cmp(other.r) < 0 }

// Greater returns true if t > other.
func (t TimeValue) Greater(other TimeValue) bool { return t.r.Cmp(other.r) > 0 }

// Add returns t + other.
func (t TimeValue) Add(other TimeValue) TimeValue {
	return TimeValue{r: new(big.Rat).Add(t.r, other.r)}
}

// Sub returns t - other. The result must be non-negative.
func (t TimeValue) Sub(other TimeValue) TimeValue {
	r := new(big.Rat).Sub(t.r, other.r)
	if r.Sign() < 0 {
		panic(fmt.Sprintf("timing: Sub result negative: %s - %s", t, other))
	}
	return TimeValue{r: r}
}

// SubAllowNeg returns t - other (may be negative; for internal use).
func (t TimeValue) SubAllowNeg(other TimeValue) TimeValue {
	return TimeValue{r: new(big.Rat).Sub(t.r, other.r)}
}

// Mul returns t * other.
func (t TimeValue) Mul(other TimeValue) TimeValue {
	return TimeValue{r: new(big.Rat).Mul(t.r, other.r)}
}

// MulInt returns t * n.
func (t TimeValue) MulInt(n int64) TimeValue {
	return TimeValue{r: new(big.Rat).Mul(t.r, new(big.Rat).SetInt64(n))}
}

// Div returns t / other. Panics on division by zero.
func (t TimeValue) Div(other TimeValue) TimeValue {
	if other.r.Sign() == 0 {
		panic("timing: division by zero")
	}
	return TimeValue{r: new(big.Rat).Quo(t.r, other.r)}
}

// DivInt returns t / n.
func (t TimeValue) DivInt(n int64) TimeValue {
	return TimeValue{r: new(big.Rat).Quo(t.r, new(big.Rat).SetInt64(n))}
}

// Max returns the larger of a and b.
func Max(a, b TimeValue) TimeValue {
	if a.r.Cmp(b.r) >= 0 {
		return a
	}
	return b
}

// Min returns the smaller of a and b.
func Min(a, b TimeValue) TimeValue {
	if a.r.Cmp(b.r) <= 0 {
		return a
	}
	return b
}

// String formats as decimal seconds with 3 decimal places.
func (t TimeValue) String() string {
	f, _ := t.r.Float64()
	return fmt.Sprintf("%.3f", f)
}

// FormatHMSm formats as "HH:MM:SS.mmm".
func (t TimeValue) FormatHMSm() string {
	f, _ := t.r.Float64()
	total := int64(f*1000 + 0.5) // total milliseconds, rounded
	ms := total % 1000
	total /= 1000
	s := total % 60
	total /= 60
	min := total % 60
	h := total / 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, min, s, ms)
}

// FormatHMSc formats as "HH:MM:SS,mmm" (SRT comma-style).
func (t TimeValue) FormatHMSc() string {
	return strings.Replace(t.FormatHMSm(), ".", ",", 1)
}

// -----------------------------------------------------------------------
// TimeInterval
// -----------------------------------------------------------------------

// RelativePosition enumerates the 26 relative positions from exacttiming.py.
type RelativePosition int

const (
	PosPP_L  RelativePosition = 0
	PosPP_C  RelativePosition = 1
	PosPP_G  RelativePosition = 2
	PosPI_LL RelativePosition = 3
	PosPI_LC RelativePosition = 4
	PosPI_LG RelativePosition = 5
	PosPI_CG RelativePosition = 6
	PosPI_GG RelativePosition = 7
	PosIP_L  RelativePosition = 8
	PosIP_B  RelativePosition = 9
	PosIP_I  RelativePosition = 10
	PosIP_E  RelativePosition = 11
	PosIP_G  RelativePosition = 12
	PosII_LL RelativePosition = 13
	PosII_LB RelativePosition = 14
	PosII_LI RelativePosition = 15
	PosII_LE RelativePosition = 16
	PosII_LG RelativePosition = 17
	PosII_BI RelativePosition = 18
	PosII_BE RelativePosition = 19
	PosII_BG RelativePosition = 20
	PosII_II RelativePosition = 21
	PosII_IE RelativePosition = 22
	PosII_IG RelativePosition = 23
	PosII_EG RelativePosition = 24
	PosII_GG RelativePosition = 25
)

// inversePosition is the direct transcription of INVERSE_RELATIVE_POSITION from exacttiming.py.
var inversePosition = [26]RelativePosition{
	PosPP_L:  PosPP_G,
	PosPP_C:  PosPP_C,
	PosPP_G:  PosPP_L,
	PosPI_LL: PosIP_G,
	PosPI_LC: PosIP_E,
	PosPI_LG: PosIP_I,
	PosPI_CG: PosIP_B,
	PosPI_GG: PosIP_L,
	PosIP_L:  PosPI_GG,
	PosIP_B:  PosPI_CG,
	PosIP_I:  PosPI_LG,
	PosIP_E:  PosPI_LC,
	PosIP_G:  PosPI_LL,
	PosII_LL: PosII_GG,
	PosII_LB: PosII_EG,
	PosII_LI: PosII_IG,
	PosII_LE: PosII_IE,
	PosII_LG: PosII_II,
	PosII_BI: PosII_BG,
	PosII_BE: PosII_BE,
	PosII_BG: PosII_BI,
	PosII_II: PosII_LG,
	PosII_IE: PosII_LE,
	PosII_IG: PosII_LI,
	PosII_EG: PosII_LB,
	PosII_GG: PosII_LL,
}

// InversePosition returns the position of self relative to other,
// given that other is at position pos relative to self.
func InversePosition(pos RelativePosition) RelativePosition {
	return inversePosition[pos]
}

// TimeInterval is a [begin, end] pair of TimeValues with begin ≤ end.
type TimeInterval struct {
	Begin TimeValue
	End   TimeValue
}

// NewTimeInterval constructs a TimeInterval, panicking on invalid args.
func NewTimeInterval(begin, end TimeValue) TimeInterval {
	if begin.r.Sign() < 0 {
		panic(fmt.Sprintf("timing: TimeInterval begin is negative: %s", begin))
	}
	if begin.r.Cmp(end.r) > 0 {
		panic(fmt.Sprintf("timing: TimeInterval begin %s > end %s", begin, end))
	}
	return TimeInterval{Begin: begin, End: end}
}

// Length returns end - begin.
func (ti TimeInterval) Length() TimeValue {
	return TimeValue{r: new(big.Rat).Sub(ti.End.r, ti.Begin.r)}
}

// HasZeroLength returns true if begin == end.
func (ti TimeInterval) HasZeroLength() bool { return ti.Begin.Equal(ti.End) }

// Contains returns true if begin <= t <= end.
func (ti TimeInterval) Contains(t TimeValue) bool {
	return ti.Begin.r.Cmp(t.r) <= 0 && t.r.Cmp(ti.End.r) <= 0
}

// InnerContains returns true if begin < t < end.
func (ti TimeInterval) InnerContains(t TimeValue) bool {
	return ti.Begin.r.Cmp(t.r) < 0 && t.r.Cmp(ti.End.r) < 0
}

// StartsAt returns true if begin == t.
func (ti TimeInterval) StartsAt(t TimeValue) bool { return ti.Begin.Equal(t) }

// EndsAt returns true if end == t.
func (ti TimeInterval) EndsAt(t TimeValue) bool { return ti.End.Equal(t) }

// IsAdjacentBefore returns true if ti.End == other.Begin.
func (ti TimeInterval) IsAdjacentBefore(other TimeInterval) bool {
	return ti.End.Equal(other.Begin)
}

// IsAdjacentAfter returns true if ti.Begin == other.End.
func (ti TimeInterval) IsAdjacentAfter(other TimeInterval) bool {
	return other.IsAdjacentBefore(ti)
}

// IsNonZeroBeforeNonZero returns true if ti ends where other begins, both non-zero.
func (ti TimeInterval) IsNonZeroBeforeNonZero(other TimeInterval) bool {
	return ti.IsAdjacentBefore(other) && !ti.HasZeroLength() && !other.HasZeroLength()
}

// Offset shifts both endpoints by delta (clamped to non-negative unless allowNeg).
func (ti *TimeInterval) Offset(delta TimeValue, allowNeg bool) {
	ti.Begin = TimeValue{r: new(big.Rat).Add(ti.Begin.r, delta.r)}
	ti.End = TimeValue{r: new(big.Rat).Add(ti.End.r, delta.r)}
	if !allowNeg {
		if ti.Begin.r.Sign() < 0 {
			ti.Begin = Zero
		}
		if ti.End.r.Sign() < 0 {
			ti.End = Zero
		}
	}
}

// String formats as "[begin, end]".
func (ti TimeInterval) String() string {
	return fmt.Sprintf("[%s, %s]", ti.Begin, ti.End)
}

// RelativePositionOf returns the position of other relative to ti.
// Direct port of TimeInterval.relative_position_of in exacttiming.py.
func (ti TimeInterval) RelativePositionOf(other TimeInterval) RelativePosition {
	selfIsPoint := ti.HasZeroLength()
	otherIsPoint := other.HasZeroLength()

	switch {
	case selfIsPoint && otherIsPoint:
		// TABLE 1
		c := ti.Begin.r.Cmp(other.Begin.r)
		switch {
		case c > 0:
			return PosPP_L // other is less than self
		case c == 0:
			return PosPP_C
		default:
			return PosPP_G
		}

	case selfIsPoint:
		// TABLE 2: self is a point, other is an interval
		if other.End.r.Cmp(ti.Begin.r) < 0 {
			return PosPI_LL
		} else if other.End.r.Cmp(ti.Begin.r) == 0 {
			return PosPI_LC
		} else if other.Begin.r.Cmp(ti.Begin.r) < 0 {
			return PosPI_LG
		} else if other.Begin.r.Cmp(ti.Begin.r) == 0 {
			return PosPI_CG
		}
		return PosPI_GG

	case otherIsPoint:
		// TABLE 3: self is an interval, other is a point
		c := other.Begin.r.Cmp(ti.Begin.r)
		if c < 0 {
			return PosIP_L
		} else if c == 0 {
			return PosIP_B
		} else if other.Begin.r.Cmp(ti.End.r) < 0 {
			return PosIP_I
		} else if other.Begin.r.Cmp(ti.End.r) == 0 {
			return PosIP_E
		}
		return PosIP_G

	default:
		// Both non-zero intervals: TABLES 4-8
		cBegin := other.Begin.r.Cmp(ti.Begin.r)
		switch {
		case cBegin < 0: // other.begin < ti.begin (TABLE 4)
			cEnd := other.End.r.Cmp(ti.Begin.r)
			switch {
			case cEnd < 0:
				return PosII_LL
			case cEnd == 0:
				return PosII_LB
			default:
				cEnd2 := other.End.r.Cmp(ti.End.r)
				if cEnd2 < 0 {
					return PosII_LI
				} else if cEnd2 == 0 {
					return PosII_LE
				}
				return PosII_LG
			}

		case cBegin == 0: // other.begin == ti.begin (TABLE 5)
			cEnd := other.End.r.Cmp(ti.End.r)
			if cEnd < 0 {
				return PosII_BI
			} else if cEnd == 0 {
				return PosII_BE
			}
			return PosII_BG

		default: // other.begin > ti.begin (TABLES 6-8)
			cBegin2 := other.Begin.r.Cmp(ti.End.r)
			if cBegin2 < 0 {
				// TABLE 6: other.begin inside ti
				cEnd := other.End.r.Cmp(ti.End.r)
				if cEnd < 0 {
					return PosII_II
				} else if cEnd == 0 {
					return PosII_IE
				}
				return PosII_IG
			} else if cBegin2 == 0 {
				// TABLE 7
				return PosII_EG
			}
			// TABLE 8
			return PosII_GG
		}
	}
}

// RelativePositionWRT returns the position of ti relative to other
// (the inverse of RelativePositionOf).
func (ti TimeInterval) RelativePositionWRT(other TimeInterval) RelativePosition {
	return InversePosition(ti.RelativePositionOf(other))
}

// Intersection returns the overlap interval.
// ok is false when there is no intersection at all (not even a shared point).
func (ti TimeInterval) Intersection(other TimeInterval) (TimeInterval, bool) {
	pos := ti.RelativePositionOf(other)
	switch pos {
	case PosPP_C, PosPI_LC, PosPI_LG, PosPI_CG, PosIP_B, PosII_LB:
		return TimeInterval{Begin: ti.Begin, End: ti.Begin}, true
	case PosIP_E, PosII_EG:
		return TimeInterval{Begin: ti.End, End: ti.End}, true
	case PosII_BI, PosII_BE, PosII_II, PosII_IE:
		return TimeInterval{Begin: other.Begin, End: other.End}, true
	case PosIP_I, PosII_LI, PosII_LE, PosII_LG, PosII_BG, PosII_IG:
		b := Max(ti.Begin, other.Begin)
		e := Min(ti.End, other.End)
		return TimeInterval{Begin: b, End: e}, true
	}
	return TimeInterval{}, false
}

// Overlaps returns true if the two intervals have any intersection.
func (ti TimeInterval) Overlaps(other TimeInterval) bool {
	_, ok := ti.Intersection(other)
	return ok
}
