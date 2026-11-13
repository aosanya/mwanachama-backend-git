package mwanachamagit

import "strings"

type lineHunk struct {
	start int
	end   int
	lines []string
}

func mergeLines(base, ours, theirs string) (merged string, conflict bool) {
	if ours == theirs {
		return ours, false
	}
	if ours == base {
		return theirs, false
	}
	if theirs == base {
		return ours, false
	}
	if strings.ContainsRune(base+ours+theirs, 0) {
		return "", true
	}
	baseLines, baseNewline := splitLines(base)
	oursLines, oursNewline := splitLines(ours)
	theirsLines, theirsNewline := splitLines(theirs)
	mergedLines, conflict := mergeHunks(baseLines, diffHunks(baseLines, oursLines), diffHunks(baseLines, theirsLines))
	if conflict {
		return "", true
	}
	return joinLines(mergedLines, mergedNewline(baseNewline, oursNewline, theirsNewline)), false
}

func mergedNewline(base, ours, theirs bool) bool {
	if ours == base {
		return theirs
	}
	return ours
}

func splitLines(text string) ([]string, bool) {
	if text == "" {
		return nil, false
	}
	newline := strings.HasSuffix(text, "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n"), newline
}

func joinLines(lines []string, newline bool) string {
	if len(lines) == 0 {
		return ""
	}
	joined := strings.Join(lines, "\n")
	if newline {
		joined += "\n"
	}
	return joined
}

func lcsTable(a, b []string) [][]int {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	return table
}

func diffHunks(base, side []string) []lineHunk {
	table := lcsTable(base, side)
	var hunks []lineHunk
	var open *lineHunk
	closeHunk := func() {
		if open != nil {
			hunks = append(hunks, *open)
			open = nil
		}
	}
	openAt := func(i int) {
		if open == nil {
			open = &lineHunk{start: i, end: i}
		}
	}
	i, j := 0, 0
	for i < len(base) || j < len(side) {
		switch {
		case i < len(base) && j < len(side) && base[i] == side[j]:
			closeHunk()
			i++
			j++
		case i < len(base) && (j == len(side) || table[i+1][j] >= table[i][j+1]):
			openAt(i)
			i++
			open.end = i
		default:
			openAt(i)
			open.lines = append(open.lines, side[j])
			j++
		}
	}
	closeHunk()
	return hunks
}

func hunksTouch(a, b lineHunk) bool {
	return a.start <= b.end && b.start <= a.end
}

func sameHunk(a, b lineHunk) bool {
	if a.start != b.start || a.end != b.end || len(a.lines) != len(b.lines) {
		return false
	}
	for i := range a.lines {
		if a.lines[i] != b.lines[i] {
			return false
		}
	}
	return true
}

func mergeHunks(base []string, ours, theirs []lineHunk) ([]string, bool) {
	var out []string
	position := 0
	apply := func(h lineHunk) {
		out = append(out, base[position:h.start]...)
		out = append(out, h.lines...)
		position = h.end
	}
	i, j := 0, 0
	for i < len(ours) || j < len(theirs) {
		switch {
		case i == len(ours):
			apply(theirs[j])
			j++
		case j == len(theirs):
			apply(ours[i])
			i++
		case hunksTouch(ours[i], theirs[j]):
			if !sameHunk(ours[i], theirs[j]) {
				return nil, true
			}
			apply(ours[i])
			i++
			j++
		case ours[i].start < theirs[j].start:
			apply(ours[i])
			i++
		default:
			apply(theirs[j])
			j++
		}
	}
	out = append(out, base[position:]...)
	return out, false
}
