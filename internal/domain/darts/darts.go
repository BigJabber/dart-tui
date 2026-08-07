// Package darts holds facts about a dartboard that are true regardless of which
// game is being played. It must not contain rules belonging to any single game
// mode — X01 and Cricket both depend on it, and neither may branch on the other.
package darts

// MaxDartsPerVisit is the number of darts in a normal visit to the board.
const MaxDartsPerVisit = 3

// SingleDartScores is the set of scores a single dart can produce, including 0
// for a miss (or a dart that bounces out / lands outside the scoring area).
var SingleDartScores = func() []int {
	seen := map[int]bool{0: true}
	for i := 1; i <= 20; i++ {
		seen[i] = true
		seen[2*i] = true
		seen[3*i] = true
	}
	seen[25] = true // outer bull
	seen[50] = true // inner bull
	out := make([]int, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	// Insertion sort keeps this dependency-free and the slice is tiny.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}()

// reachable[n] is the set of totals achievable with exactly n darts, for
// n in 1..MaxDartsPerVisit. Totals are inclusive of misses, so every total
// reachable with n darts is also reachable with n+1.
var reachable = func() [MaxDartsPerVisit + 1]map[int]bool {
	var tbl [MaxDartsPerVisit + 1]map[int]bool
	tbl[0] = map[int]bool{0: true}
	for n := 1; n <= MaxDartsPerVisit; n++ {
		cur := make(map[int]bool)
		for prev := range tbl[n-1] {
			for _, v := range SingleDartScores {
				cur[prev+v] = true
			}
		}
		tbl[n] = cur
	}
	return tbl
}()

// MaxScore is the highest total that can be thrown with n darts.
func MaxScore(darts int) int {
	if darts < 0 {
		return 0
	}
	if darts > MaxDartsPerVisit {
		darts = MaxDartsPerVisit
	}
	return darts * 60
}

// Reachable reports whether points can actually be scored with the given number
// of darts. It rejects both out-of-range totals and the "impossible" ones such
// as 179 with three darts, which are almost always typing mistakes.
func Reachable(points, dartCount int) bool {
	if dartCount < 1 || dartCount > MaxDartsPerVisit {
		return false
	}
	if points < 0 {
		return false
	}
	return reachable[dartCount][points]
}

// ValidDartCount reports whether n is a usable number of darts for one visit.
func ValidDartCount(n int) bool {
	return n >= 1 && n <= MaxDartsPerVisit
}

// ThreeDartAverage is the standard darts average: credited points per dart,
// scaled to a three-dart visit.
//
// It deliberately divides by darts thrown rather than visits×3, because a
// checkout visit may contain one or two darts and would otherwise inflate or
// deflate the figure.
func ThreeDartAverage(creditedPoints, dartsThrown int) float64 {
	if dartsThrown <= 0 {
		return 0
	}
	return float64(creditedPoints) / float64(dartsThrown) * 3
}
